package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	// AgentModeCanonical launches Siemens' NX2512 managed_core/.NET 8 runner
	// against the compiled NXGO.Agent.NXHost DLL.
	AgentModeCanonical AgentMode = "canonical"
)

type WorkerConfig struct {
	NXHome         string
	PipeName       string
	ArtifactDir    string
	JournalPath    string
	AgentMode      AgentMode
	AgentBin       string
	RunnerPath     string
	TargetPath     string
	StartupTimeout time.Duration
	WorkerNonce    string
}

type WorkerProcess struct {
	Config        WorkerConfig
	Manifest      *WorkerManifest
	Client        *pipe.Client
	cmd           *exec.Cmd
	waitDone      <-chan error
	output        *synchronizedBuffer
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

	requestedJournal := cfg.JournalPath
	targetPath, agentBin, err := resolveWorkerAgentPaths(cfg, findRepoRoot())
	if err != nil {
		return nil, err
	}
	cfg.JournalPath = ""
	cfg.AgentBin = agentBin
	cfg.TargetPath = targetPath

	var command string
	var commandArgs []string
	if requestedJournal != "" {
		// Explicit custom journals remain available for fixtures and are not the
		// canonical Agent path. They continue to use Siemens run_journal.
		absJournal, absErr := filepath.Abs(requestedJournal)
		if absErr != nil {
			return nil, absErr
		}
		cfg.JournalPath = absJournal
		command = inst.RunJournal
		commandArgs = []string{absJournal}
	} else {
		command, commandArgs, err = resolveCanonicalWorkerLaunch(inst, targetPath)
		if err != nil {
			return nil, err
		}
		cfg.RunnerPath = command
	}

	if cfg.WorkerNonce == "" {
		nonce, nonceErr := newWorkerNonce()
		if nonceErr != nil {
			return nil, fmt.Errorf("generate worker handshake nonce: %w", nonceErr)
		}
		cfg.WorkerNonce = nonce
	}

	cmd := exec.Command(command, commandArgs...)
	// Siemens' managed_core runner resolves NX native libraries relative to
	// NXBIN. Starting it from the repository/agent output directory can load
	// managed assemblies successfully but leaves libuginit unresolved.
	cmd.Dir = filepath.Join(inst.Home, "NXBIN")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("NXGO_PIPE_NAME=%s", cfg.PipeName),
		fmt.Sprintf("NXGO_WORKER_NONCE=%s", cfg.WorkerNonce),
		fmt.Sprintf("UGII_BASE_DIR=%s", inst.Home),
		fmt.Sprintf("UGII_NXBIN=%s", filepath.Join(inst.Home, "NXBIN")),
		fmt.Sprintf("UGII_ROOT_DIR=%s", inst.Home),
		"UGII_PLATFORM=x64wnt",
		fmt.Sprintf("UGII_LOCALIZATION_FILES=%s", filepath.Join(inst.Home, "localization")),
		fmt.Sprintf("HOME=%s", os.TempDir()),
		// The direct managed_core runner does not inherit run_journal's native
		// search path, but NXOpen P/Invokes libuginit and related NX binaries.
		fmt.Sprintf("PATH=%s;%s;%s;%s", filepath.Join(inst.Home, "NXBIN"), filepath.Join(inst.Home, "UGII"), filepath.Join(inst.Home, "NXBIN", "managed_core"), os.Getenv("PATH")),
	)
	if cfg.ArtifactDir != "" {
		journalState := filepath.Join(cfg.ArtifactDir, "request-journal.bin")
		cmd.Env = append(cmd.Env, fmt.Sprintf("NXGO_JOURNAL_STATE=%s", journalState))
	}
	if cfg.AgentBin != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("NXGO_AGENT_BIN=%s", cfg.AgentBin))
	}
	if cfg.ArtifactDir != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("NXGO_AGENT_DIAGNOSTICS=%s", filepath.Join(cfg.ArtifactDir, "agent-bootstrap.log")))
		_ = writeRunnerLaunchDiagnostic(cfg.ArtifactDir, command, commandArgs, inst)
	}

	var outBuf synchronizedBuffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start NX Agent runner %s: %w", filepath.Base(command), err)
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
		output:   &outBuf,
	}

	pipePath := fmt.Sprintf(`\\.\pipe\%s`, cfg.PipeName)
	client, err := waitForPipe(ctx, pipePath, cfg.StartupTimeout, waitDone, &outBuf)
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
		Nonce:           cfg.WorkerNonce,
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

	if cfg.ArtifactDir != "" {
		if manifestBytes, err := json.MarshalIndent(wp.Manifest, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(cfg.ArtifactDir, "worker-manifest.json"), manifestBytes, 0644)
		}
	}

	return wp, nil
}

func writeRunnerLaunchDiagnostic(artifactDir, command string, args []string, inst *Installation) error {
	if inst == nil || artifactDir == "" {
		return nil
	}
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	payload := map[string]any{
		"command": command, "args": args, "nx_home": inst.Home,
		"managed_dir": inst.ManagedDir, "runner": inst.RunDotnetCoreNXOpen,
		"working_dir": filepath.Join(inst.Home, "NXBIN"),
		"environment_contract": map[string]string{
			"UGII_BASE_DIR": inst.Home,
			"UGII_ROOT_DIR": inst.Home,
			"UGII_NXBIN":    filepath.Join(inst.Home, "NXBIN"),
			"UGII_PLATFORM": "x64wnt",
			"HOME":          os.TempDir(),
		},
		"mode": "managed_core", "created_at_utc": time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(artifactDir, "runner-launch.json"), data, 0o644)
}

// resolveCanonicalWorkerLaunch is intentionally NX-independent so the
// managed_core command/argument contract is covered by ordinary CI.
func resolveCanonicalWorkerLaunch(inst *Installation, targetPath string) (string, []string, error) {
	if inst == nil {
		return "", nil, errors.New("canonical NX Agent installation is required")
	}
	if inst.RunDotnetCoreNXOpen == "" {
		return "", nil, fmt.Errorf("canonical NX Agent requires NX2512 managed_core runner run_dotnet_core_nxopen.exe under %s", inst.Home)
	}
	if filepath.Base(inst.ManagedDir) != "managed_core" {
		return "", nil, fmt.Errorf("canonical NX Agent requires managed_core NXOpen assemblies; discovered %s", inst.ManagedDir)
	}
	if strings.ToLower(filepath.Ext(targetPath)) != ".dll" {
		return "", nil, fmt.Errorf("canonical NX Agent target must be a .NET DLL: %s", targetPath)
	}
	return inst.RunDotnetCoreNXOpen, []string{strings.TrimSuffix(targetPath, filepath.Ext(targetPath))}, nil
}

// resolveWorkerAgentPaths is intentionally NX-independent so the target
// contract is covered by ordinary CI. For canonical mode the first result is
// the .NET target DLL, not a run_journal source file. Explicit JournalPath
// remains an escape hatch for test fixtures/custom journals, but it cannot be
// combined with an AgentMode because that would make the selected runtime
// ambiguous.
func resolveWorkerAgentPaths(cfg WorkerConfig, repoRoot string) (journalPath string, agentBin string, err error) {
	if cfg.JournalPath != "" {
		if cfg.AgentMode != "" {
			return "", "", errors.New("worker config cannot set both JournalPath and AgentMode")
		}
		return cfg.JournalPath, cfg.AgentBin, nil
	}

	mode := cfg.AgentMode
	if mode == "" {
		mode = AgentModeCanonical
	}

	switch mode {
	case AgentModeCanonical:
		agentBin = cfg.AgentBin
		if agentBin == "" {
			agentBin = filepath.Join(repoRoot, "agent", "bin")
		}
		absBin, absErr := filepath.Abs(agentBin)
		if absErr != nil {
			return "", "", fmt.Errorf("resolve canonical AgentBin: %w", absErr)
		}
		agentBin = absBin
		for _, dll := range []string{"Newtonsoft.Json.dll", "NXGO.Protocol.dll", "NXGO.Agent.Core.dll", "NXGO.Agent.NXHost.dll", "NXGO.Agent.NXHost.runtimeconfig.json"} {
			path := filepath.Join(agentBin, dll)
			if _, statErr := os.Stat(path); statErr != nil {
				return "", "", fmt.Errorf("canonical NX Agent artifact unavailable %s: %w", path, statErr)
			}
		}
		return filepath.Join(agentBin, "NXGO.Agent.NXHost.dll"), agentBin, nil

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

// Output returns the combined stdout and stderr captured from the worker runner process.
func (wp *WorkerProcess) Output() string {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	if wp.output == nil {
		return ""
	}
	return wp.output.String()
}

func (wp *WorkerProcess) flushArtifacts() {
	if wp.Config.ArtifactDir == "" {
		return
	}
	wp.mu.Lock()
	out := ""
	if wp.output != nil {
		out = wp.output.String()
	}
	wp.mu.Unlock()
	if out != "" {
		_ = os.WriteFile(filepath.Join(wp.Config.ArtifactDir, "runner-output.log"), []byte(out), 0644)
	}
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
	wp.flushArtifacts()
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
		wp.flushArtifacts()
		return nil
	}

	err := process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		err = nil
	}
	_ = waitForWorkerExit(waitDone, 2*time.Second)
	wp.flushArtifacts()
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
