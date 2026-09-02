package supervisor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

var (
	ErrWorkerStartTimeout = errors.New("timeout waiting for NX Agent named pipe")
	ErrWorkerDied         = errors.New("NX worker process exited prematurely")
)

type WorkerConfig struct {
	NXHome         string
	PipeName       string
	ArtifactDir    string
	JournalPath    string
	StartupTimeout time.Duration
}

type WorkerProcess struct {
	Config   WorkerConfig
	Manifest *WorkerManifest
	Client   *pipe.Client
	cmd      *exec.Cmd
	mu       sync.Mutex
	stopped  bool
	quarantineErr error
}

func StartWorker(ctx context.Context, cfg WorkerConfig) (*WorkerProcess, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("real NX worker requires Windows")
	}

	if cfg.NXHome == "" {
		installs, err := Discover()
		if err != nil {
			return nil, fmt.Errorf("discover NX: %w", err)
		}
		cfg.NXHome = installs[0].Home
	}

	inst, err := InspectInstallation(cfg.NXHome, "worker_config")
	if err != nil {
		return nil, fmt.Errorf("inspect NX home: %w", err)
	}

	if cfg.PipeName == "" {
		cfg.PipeName = fmt.Sprintf("nxgo-worker-%d-%d", os.Getpid(), time.Now().UnixNano()%1000000)
	}
	if cfg.StartupTimeout <= 0 {
		cfg.StartupTimeout = 30 * time.Second
	}
	if cfg.JournalPath == "" {
		repoRoot := findRepoRoot()
		cfg.JournalPath = filepath.Join(repoRoot, "agent", "bundle", "AgentWorker.cs")
	}

	absJournal, err := filepath.Abs(cfg.JournalPath)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(inst.RunJournal, absJournal)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("NXGO_PIPE_NAME=%s", cfg.PipeName),
		fmt.Sprintf("UGII_BASE_DIR=%s", inst.Home),
	)

	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start run_journal: %w", err)
	}

	wp := &WorkerProcess{
		Config: cfg,
		cmd:    cmd,
	}

	pipePath := fmt.Sprintf(`\\.\pipe\%s`, cfg.PipeName)
	client, err := waitForPipe(ctx, pipePath, cfg.StartupTimeout, cmd, &outBuf)
	if err != nil {
		_ = cmd.Process.Kill()
		if outBuf.Len() > 0 {
			return nil, fmt.Errorf("%w (output: %s)", err, outBuf.String())
		}
		return nil, err
	}
	wp.Client = client

	// Any ambiguous transport outcome or protocol-corruption event makes the
	// NX process unsafe to reuse. Killing the owning worker is intentionally
	// aggressive: a request that timed out after send may still be queued or
	// running inside NX, so keeping the process alive could allow a late CAD
	// mutation to commit after the caller already observed failure.
	client.SetQuarantineHook(func(cause error) {
		wp.quarantine(cause)
	})

	// Perform handshake.
	hsResp, err := client.Handshake(ctx, &protocol.HandshakeRequest{
		ProtocolVersion: protocol.Version{Major: protocol.CurrentProtocolMajor, Minor: protocol.CurrentProtocolMinor},
		SDKVersion:      "v0.1.0",
		ClientPID:       os.Getpid(),
		Nonce:           fmt.Sprintf("nonce-%d", time.Now().UnixNano()),
	})
	if err != nil {
		_ = wp.Kill()
		return nil, fmt.Errorf("worker handshake failed: %w", err)
	}

	wp.Manifest = &WorkerManifest{
		ID:          hsResp.SessionID,
		PID:         cmd.Process.Pid,
		NXHome:      inst.Home,
		NXRelease:   hsResp.NXRelease,
		Endpoint:    pipePath,
		StartedAt:   time.Now().UTC(),
		Owner:       os.Getenv("USERNAME"),
		Mode:        "dedicated-worker",
		ArtifactDir: cfg.ArtifactDir,
	}

	return wp, nil
}

func waitForPipe(ctx context.Context, pipePath string, timeout time.Duration, cmd *exec.Cmd, outBuf *bytes.Buffer) (*pipe.Client, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			if outBuf != nil && outBuf.Len() > 0 {
				return nil, fmt.Errorf("%w: %s", ErrWorkerDied, outBuf.String())
			}
			return nil, ErrWorkerDied
		}

		dialCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		conn, err := pipe.DialPipe(dialCtx, pipePath)
		cancel()

		if err == nil {
			return pipe.NewClient(conn), nil
		}

		time.Sleep(100 * time.Millisecond)
	}
	return nil, ErrWorkerStartTimeout
}

// quarantine records the first terminal transport cause and destroys the
// process that owns the corresponding NX session. It is called asynchronously
// by pipe.Client so it must be safe to race with Stop/Kill.
func (wp *WorkerProcess) quarantine(cause error) {
	wp.mu.Lock()
	if wp.quarantineErr == nil {
		wp.quarantineErr = cause
	}
	alreadyStopped := wp.stopped
	wp.stopped = true
	client := wp.Client
	var process *os.Process
	if wp.cmd != nil {
		process = wp.cmd.Process
	}
	wp.mu.Unlock()

	if client != nil {
		_ = client.Close()
	}
	if !alreadyStopped && process != nil {
		_ = process.Kill()
	}
}

// QuarantineReason reports why this worker was made permanently unusable. A
// nil result means no transport quarantine has been recorded.
func (wp *WorkerProcess) QuarantineReason() error {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	return wp.quarantineErr
}

func (wp *WorkerProcess) Stop(ctx context.Context) error {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	if wp.stopped {
		return nil
	}
	wp.stopped = true

	if wp.Client != nil {
		callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_, _ = wp.Client.Call(callCtx, &protocol.RequestEnvelope{
			RequestID: "shutdown-req",
			Op:        "shutdown",
		})
		cancel()
		_ = wp.Client.Close()
	}

	if wp.cmd != nil && wp.cmd.Process != nil {
		_ = wp.cmd.Process.Kill()
		_ = wp.cmd.Wait()
	}
	return nil
}

func (wp *WorkerProcess) Kill() error {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.stopped = true
	if wp.Client != nil {
		_ = wp.Client.Close()
	}
	if wp.cmd != nil && wp.cmd.Process != nil {
		return wp.cmd.Process.Kill()
	}
	return nil
}

func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}
