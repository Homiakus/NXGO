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

type AgentMode string

const (
	// AgentModeLegacy keeps the current production default until the compiled
	// Core->NXHost path has retained E3 evidence on the pinned real-NX runner.
	AgentModeLegacy AgentMode = "legacy"
	// AgentModeCanonical launches the minimal run_journal bootstrap that loads
	// NXGO.Agent.Core.dll + NXGO.Agent.NXHost.dll from AgentBin.
	AgentModeCanonical AgentMode = "canonical"
)

type WorkerConfig struct {
	NXHome         string
	PipeName       string
	ArtifactDir    string
	JournalPath    string
	AgentMode      AgentMode
	AgentBin       string
	StartupTimeout time.Duration
}

type WorkerProcess struct {
	Config        WorkerConfig
	Manifest      *WorkerManifest
	Client        *pipe.Client
	cmd           *exec.Cmd
	waitDone      <-chan error
	mu            sync.Mutex
	stopped       bool
	quarantineErr error
}

// synchronizedBuffer is used by cmd.Stdout/Stderr and diagnostic readers on
// different goroutines. bytes.Buffer alone is not safe for concurrent access.
type synchronizedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *synchronizedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
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

	journalPath, agentBin, err := resolveWorkerAgentPaths(cfg, findRepoRoot())
	if err != nil {
		return nil, err
	}
	cfg.JournalPath = journalPath
	cfg.AgentBin = agentBin

	absJournal, err := filepath.Abs(cfg.JournalPath)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(inst.RunJournal, absJournal)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("NXGO_PIPE_NAME=%s", cfg.PipeName),
		fmt.Sprintf("UGII_BASE_DIR=%s", inst.Home),
	)
	if cfg.AgentBin != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("NXGO_AGENT_BIN=%s", cfg.AgentBin))
	}

	var outBuf synchronizedBuffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start run_journal: %w", err)
	}

	// Exactly one goroutine owns Cmd.Wait. This both reaps the child and gives
	// startup/Stop/Kill a reliable process-exit signal; ProcessState polling
	// before Wait cannot detect an early run_journal crash reliably.
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
		close(waitDone)
	}()

	wp := &WorkerProcess{
		Config:   cfg,
		cmd:      cmd,
		waitDone: waitDone,
	}

	pipePath := fmt.Sprintf(`\\.\pipe\%s`, cfg.PipeName)
	client, err := waitForPipe(ctx, pipePath, cfg.StartupTimeout, waitDone)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = waitForWorkerExit(waitDone, 2*time.Second)
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

// resolveWorkerAgentPaths is intentionally NX-independent so the launch-mode
// contract is covered by ordinary CI. Explicit JournalPath remains an escape
// hatch for test fixtures/custom journals, but it cannot be combined with an
// AgentMode because that would make the selected runtime ambiguous.
func resolveWorkerAgentPaths(cfg WorkerConfig, repoRoot string) (journalPath string, agentBin string, err error) {
	if cfg.JournalPath != "" {
		if cfg.AgentMode != "" {
			return "", "", errors.New("worker config cannot set both JournalPath and AgentMode")
		}
		return cfg.JournalPath, cfg.AgentBin, nil
	}

	mode := cfg.AgentMode
	if mode == "" {
		mode = AgentModeLegacy
	}

	switch mode {
	case AgentModeLegacy:
		journalPath = filepath.Join(repoRoot, "agent", "bundle", "AgentWorker.cs")
		if _, statErr := os.Stat(journalPath); statErr != nil {
			return "", "", fmt.Errorf("legacy NX Agent journal unavailable: %w", statErr)
		}
		return journalPath, "", nil

	case AgentModeCanonical:
		journalPath = filepath.Join(repoRoot, "agent", "bundle", "CompiledHostBootstrap.cs")
		if _, statErr := os.Stat(journalPath); statErr != nil {
			return "", "", fmt.Errorf("canonical NX Agent bootstrap unavailable: %w", statErr)
		}

		agentBin = cfg.AgentBin
		if agentBin == "" {
			agentBin = filepath.Join(repoRoot, "agent", "bin")
		}
		absBin, absErr := filepath.Abs(agentBin)
		if absErr != nil {
			return "", "", fmt.Errorf("resolve canonical AgentBin: %w", absErr)
		}
		agentBin = absBin
		for _, dll := range []string{"Newtonsoft.Json.dll", "NXGO.Protocol.dll", "NXGO.Agent.Core.dll", "NXGO.Agent.NXHost.dll"} {
			path := filepath.Join(agentBin, dll)
			if _, statErr := os.Stat(path); statErr != nil {
				return "", "", fmt.Errorf("canonical NX Agent artifact unavailable %s: %w", path, statErr)
			}
		}
		return journalPath, agentBin, nil

	default:
		return "", "", fmt.Errorf("unsupported NX Agent mode %q", mode)
	}
}

func waitForPipe(ctx context.Context, pipePath string, timeout time.Duration, waitDone <-chan error, output *synchronizedBuffer) (*pipe.Client, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	probe := time.NewTicker(100 * time.Millisecond)
	defer probe.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case waitErr := <-waitDone:
			if waitErr != nil {
				return nil, fmt.Errorf("%w: %v", ErrWorkerDied, waitErr)
			}
			return nil, ErrWorkerDied
		case <-deadline.C:
			return nil, ErrWorkerStartTimeout
		case <-probe.C:
			if output != nil && startupOutputIndicatesFailure(output.String()) {
				return nil, ErrWorkerDied
			}
			dialCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			conn, err := pipe.DialPipe(dialCtx, pipePath)
			cancel()
			if err == nil {
				return pipe.NewClient(conn), nil
			}
		}
	}
}

func waitForWorkerExit(waitDone <-chan error, timeout time.Duration) error {
	if waitDone == nil {
		return nil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-waitDone:
		return err
	case <-timer.C:
		return context.DeadlineExceeded
	}
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
	if wp.stopped {
		wp.mu.Unlock()
		return nil
	}
	wp.stopped = true
	client := wp.Client
	var process *os.Process
	if wp.cmd != nil {
		process = wp.cmd.Process
	}
	waitDone := wp.waitDone
	wp.mu.Unlock()

	if client != nil {
		callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_, _ = client.Call(callCtx, &protocol.RequestEnvelope{
			RequestID: "shutdown-req",
			Op:        "shutdown",
		})
		cancel()
		_ = client.Close()
	}

	var killErr error
	if process != nil {
		killErr = process.Kill()
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
	}
	_ = waitForWorkerExit(waitDone, 2*time.Second)
	return killErr
}

func (wp *WorkerProcess) Kill() error {
	wp.mu.Lock()
	alreadyStopped := wp.stopped
	wp.stopped = true
	client := wp.Client
	var process *os.Process
	if wp.cmd != nil {
		process = wp.cmd.Process
	}
	waitDone := wp.waitDone
	wp.mu.Unlock()

	if client != nil {
		_ = client.Close()
	}
	if alreadyStopped || process == nil {
		return nil
	}

	err := process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		err = nil
	}
	_ = waitForWorkerExit(waitDone, 2*time.Second)
	return err
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
