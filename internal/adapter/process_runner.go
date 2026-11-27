package adapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/source/localfile"
)

const (
	// DefaultTimeoutMs is the default hard execution timeout for an adapter (30 seconds).
	DefaultTimeoutMs int64 = 30000

	// DefaultMaxOutputBytes is the default stdout capture limit (10 MiB).
	DefaultMaxOutputBytes int64 = 10 * 1024 * 1024

	// DefaultMaxStderrBytes is the default stderr diagnostic capture limit (1 MiB).
	DefaultMaxStderrBytes int64 = 1 * 1024 * 1024

	// stagedInputFilename is the fixed filename created inside the per-run temp dir.
	stagedInputFilename = "input.xdr"
)

// RunParams defines trusted execution parameters for invoking an allowlisted adapter.
// Callers cannot pass arbitrary executable paths, shell commands, environment variables,
// or arbitrary input file paths.
type RunParams struct {
	RunID       string
	CaseID      string
	Operation   string
	InputFormat string
	Target      RequestTarget
	Limits      RequestLimits
}

// RunOutput holds the validated adapter result alongside captured bounded diagnostics.
type RunOutput struct {
	Result          *Result
	Stderr          []byte
	StderrTruncated bool
	StderrBytesSeen int64
	ActualExitCode  int
	ActualDuration  time.Duration
	rawStdout       []byte
}

// StdoutCopy returns a defensive copy of the exact validated JSON bytes
// captured from adapter stdout. Mutating the returned slice has no effect on
// RunOutput's internal buffer.
func (o *RunOutput) StdoutCopy() []byte {
	if o == nil || len(o.rawStdout) == 0 {
		return nil
	}
	cp := make([]byte, len(o.rawStdout))
	copy(cp, o.rawStdout)
	return cp
}

// ProcessRunner executes allowlisted adapters in isolated per-run temporary
// workspaces with bounded input staging, bounded output capture, minimal
// environment, and strict timeout/cancellation enforcement.
//
// # OS Isolation Limitations
//
// ProcessRunner uses os/exec.CommandContext with a minimal environment, a unique
// per-run temporary working directory (0700), and read-only staged input (0400).
// It does NOT provide a complete OS sandbox or container isolation boundary:
//   - Memory limits (e.g., cgroups/rlimit) are not enforced portably across OSes.
//   - Network namespaces are not created by this Go process runner; production
//     deployments must enforce network egress controls at the container/host level.
//   - On timeout or cancellation, CommandContext terminates the direct child process
//     via SIGKILL (on Unix) or TerminateProcess (on Windows); grandchildren spawned
//     by an adapter are not guaranteed to be reaped without OS-specific process groups
//     or cgroup supervision.
type ProcessRunner struct {
	registry       *Registry
	baseTempDir    string
	maxStderrBytes int64
}

// ProcessRunnerConfig configures a ProcessRunner.
type ProcessRunnerConfig struct {
	// Registry is the allowlisted adapter registry (required).
	Registry *Registry

	// BaseTempDir is an optional parent directory for per-run temporary directories.
	// If empty, os.TempDir() is used.
	BaseTempDir string

	// MaxStderrBytes bounds captured stderr diagnostics. Defaults to DefaultMaxStderrBytes (1 MiB).
	MaxStderrBytes int64
}

// NewProcessRunner creates a new ProcessRunner with the given configuration.
func NewProcessRunner(cfg ProcessRunnerConfig) (*ProcessRunner, error) {
	if cfg.Registry == nil {
		return nil, fmt.Errorf("%w: adapter registry must not be nil", ErrInvalidRequest)
	}

	maxStderr := cfg.MaxStderrBytes
	if maxStderr <= 0 {
		maxStderr = DefaultMaxStderrBytes
	}

	if cfg.BaseTempDir != "" {
		info, err := os.Stat(cfg.BaseTempDir)
		if err != nil {
			return nil, fmt.Errorf("%w: base temp dir %q stat failed: %v", ErrInvalidRequest, cfg.BaseTempDir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%w: base temp dir %q is not a directory", ErrInvalidRequest, cfg.BaseTempDir)
		}
	}

	return &ProcessRunner{
		registry:       cfg.Registry,
		baseTempDir:    cfg.BaseTempDir,
		maxStderrBytes: maxStderr,
	}, nil
}

// Run executes the target adapter against an already-verified corpus input.
//
// It stages the verified bytes into a unique per-run temporary directory as a
// read-only file, invokes the allowlisted executable directly (never via a shell),
// enforces timeout, cancellation, and output bounds, and validates the response
// against both the JSON Schema and the original request.
func (r *ProcessRunner) Run(ctx context.Context, params RunParams, verified *localfile.VerifiedInput) (*RunOutput, error) {
	if verified == nil {
		return nil, fmt.Errorf("%w: verified input must not be nil", ErrInvalidRequest)
	}

	// Verify limits before launching anything.
	if err := validateRequestLimits(&params.Limits); err != nil {
		return nil, err
	}

	// Reject input exceeding max_input_bytes before staging or launching.
	if verified.ByteLength() > params.Limits.MaxInputBytes {
		return nil, fmt.Errorf("%w: input byte length %d exceeds max_input_bytes %d",
			ErrInputTooLarge, verified.ByteLength(), params.Limits.MaxInputBytes)
	}

	// Resolve allowlisted adapter from registry.
	spec, err := r.registry.Resolve(params.Target, params.Operation)
	if err != nil {
		return nil, err
	}

	// Create unique per-run temporary directory with restrictive permissions (0700).
	runDir, err := os.MkdirTemp(r.baseTempDir, "siw-run-*")
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create per-run temp directory: %v", ErrAdapterProcessFailure, err)
	}
	defer func() {
		// Ensure read-only staged files can be removed cleanly on all platforms.
		stagedPath := filepath.Join(runDir, stagedInputFilename)
		_ = os.Chmod(stagedPath, 0o600)
		_ = os.RemoveAll(runDir)
	}()

	if err := os.Chmod(runDir, 0o700); err != nil {
		return nil, fmt.Errorf("%w: failed to set permissions on run directory: %v", ErrAdapterProcessFailure, err)
	}

	// Stage verified bytes into worker-created read-only file.
	stagedPath := filepath.Join(runDir, stagedInputFilename)
	if err := stageReadOnlyInput(stagedPath, verified); err != nil {
		return nil, fmt.Errorf("%w: failed to stage input: %v", ErrAdapterProcessFailure, err)
	}

	// Construct and validate the protocol request.
	req := &Request{
		SchemaVersion: ProtocolVersion,
		RunID:         params.RunID,
		CaseID:        params.CaseID,
		Operation:     params.Operation,
		InputPath:     stagedPath,
		InputFormat:   params.InputFormat,
		InputSHA256:   verified.SHA256(),
		Target:        params.Target,
		Limits:        params.Limits,
	}

	if err := ValidateRequest(req); err != nil {
		return nil, err
	}

	reqBytes, err := EncodeRequest(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to encode request: %v", ErrInvalidRequest, err)
	}

	// Execute with hard timeout bound to parent context.
	timeout := time.Duration(params.Limits.TimeoutMs) * time.Millisecond
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command directly from registry binary path (never via shell, no args).
	cmd := exec.CommandContext(runCtx, spec.BinaryPath)
	cmd.Dir = runDir
	cmd.Env = minimalEnvironment()
	cmd.Stdin = bytes.NewReader(reqBytes)

	stdoutCapture := newBoundedStdoutWriter(params.Limits.MaxOutputBytes, cancel)
	stderrCapture := newBoundedStderrWriter(r.maxStderrBytes)
	cmd.Stdout = stdoutCapture
	cmd.Stderr = stderrCapture

	startTime := time.Now()
	runErr := cmd.Run()
	duration := time.Since(startTime)

	// Determine exit code.
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	// Check output overflow FIRST so it is never misclassified as timeout or ordinary exit failure.
	if stdoutCapture.Overflowed() {
		return nil, fmt.Errorf("%w: adapter stdout exceeded limit of %d bytes",
			ErrAdapterOutputTooLarge, params.Limits.MaxOutputBytes)
	}

	// Check parent context cancellation vs timeout.
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%w: %v", ErrAdapterCancelled, ctx.Err())
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("%w: exceeded %dms timeout", ErrAdapterTimeout, params.Limits.TimeoutMs)
	}

	// If the process failed to launch (exitCode == -1), return process failure immediately.
	if runErr != nil && exitCode == -1 {
		return nil, fmt.Errorf("%w: failed to execute adapter: %v", ErrAdapterProcessFailure, runErr)
	}

	stdoutBytes := stdoutCapture.Bytes()
	stderrBytes, stderrTruncated, stderrSeen := stderrCapture.Snapshot()

	// Parse and validate JSON result from stdout.
	if len(bytes.TrimSpace(stdoutBytes)) == 0 {
		return nil, fmt.Errorf("%w: adapter produced empty stdout (exit code %d): %v",
			ErrAdapterProcessFailure, exitCode, runErr)
	}

	// Strictly decode (rejects malformed JSON, unknown fields, trailing JSON).
	res, err := DecodeResult(stdoutBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAdapterMalformedOutput, err)
	}

	// Validate against embedded JSON Schema (Draft 2020-12).
	if err := SchemaValidateResult(stdoutBytes); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResult, err)
	}

	// Validate typed result rules and protocol version.
	if err := ValidateResult(res); err != nil {
		return nil, err
	}

	// Validate identity and integrity consistency against request.
	if err := ValidateResultAgainstRequest(res, req); err != nil {
		return nil, err
	}

	// Verify execution status and exit code consistency against actual process exit.
	if err := validateProcessExitConsistency(res, exitCode, runErr); err != nil {
		return nil, err
	}

	rawStdoutCopy := make([]byte, len(stdoutBytes))
	copy(rawStdoutCopy, stdoutBytes)

	return &RunOutput{
		Result:          res,
		Stderr:          stderrBytes,
		StderrTruncated: stderrTruncated,
		StderrBytesSeen: stderrSeen,
		ActualExitCode:  exitCode,
		ActualDuration:  duration,
		rawStdout:       rawStdoutCopy,
	}, nil
}

// stageReadOnlyInput writes the verified input bytes to path and sets it read-only (0400).
func stageReadOnlyInput(path string, verified *localfile.VerifiedInput) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(f, verified.NewReader())
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}

	return os.Chmod(path, 0o400)
}

// minimalEnvironment returns a stripped environment with no inherited secrets,
// cloud credentials, or tokens.
func minimalEnvironment() []string {
	env := []string{
		"LC_ALL=C",
		"LANG=C",
	}
	// Preserve SYSTEMROOT on Windows if present, as Go's runtime/winsock requires it.
	if sysRoot := os.Getenv("SYSTEMROOT"); sysRoot != "" {
		env = append(env, "SYSTEMROOT="+sysRoot)
	}
	return env
}

// validateProcessExitConsistency ensures the adapter's reported execution status
// and exit code match the actual OS process outcome.
func validateProcessExitConsistency(res *Result, actualExitCode int, runErr error) error {
	if res.Execution.ExitCode != actualExitCode {
		return fmt.Errorf("%w: reported exit_code %d does not match actual process exit code %d",
			ErrAdapterResponseMismatch, res.Execution.ExitCode, actualExitCode)
	}

	if runErr == nil && res.Execution.Status != ExecutionStatusCompleted {
		return fmt.Errorf("%w: process exited 0 but reported execution.status %q",
			ErrAdapterResponseMismatch, res.Execution.Status)
	}

	if runErr != nil && res.Execution.Status == ExecutionStatusCompleted {
		return fmt.Errorf("%w: process exited %d but reported execution.status %q",
			ErrAdapterResponseMismatch, actualExitCode, res.Execution.Status)
	}

	return nil
}

// boundedStdoutWriter enforces a strict byte limit on stdout without unbounded buffering.
// When the limit is exceeded, it records overflow and invokes cancel() to terminate
// the child process immediately.
type boundedStdoutWriter struct {
	mu         sync.Mutex
	maxBytes   int64
	written    int64
	overflowed bool
	buf        bytes.Buffer
	cancel     context.CancelFunc
}

func newBoundedStdoutWriter(maxBytes int64, cancel context.CancelFunc) *boundedStdoutWriter {
	return &boundedStdoutWriter{
		maxBytes: maxBytes,
		cancel:   cancel,
	}
}

func (w *boundedStdoutWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.overflowed {
		// Discard excess bytes without storing them in memory, and return len(p)
		// so os/exec does not wait or error before process termination completes.
		w.written += int64(len(p))
		return len(p), nil
	}

	remaining := w.maxBytes - w.written
	if int64(len(p)) > remaining {
		if remaining > 0 {
			w.buf.Write(p[:remaining])
		}
		w.written += int64(len(p))
		w.overflowed = true
		if w.cancel != nil {
			w.cancel()
		}
		return len(p), nil
	}

	w.buf.Write(p)
	w.written += int64(len(p))
	return len(p), nil
}

func (w *boundedStdoutWriter) Overflowed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.overflowed
}

func (w *boundedStdoutWriter) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make([]byte, w.buf.Len())
	copy(cp, w.buf.Bytes())
	return cp
}

// boundedStderrWriter caps captured stderr bytes in memory while draining the pipe
// so the child process never blocks on a full stderr pipe.
type boundedStderrWriter struct {
	mu        sync.Mutex
	maxBytes  int64
	seen      int64
	truncated bool
	buf       bytes.Buffer
}

func newBoundedStderrWriter(maxBytes int64) *boundedStderrWriter {
	return &boundedStderrWriter{
		maxBytes: maxBytes,
	}
}

func (w *boundedStderrWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.seen += int64(len(p))
	if w.truncated {
		return len(p), nil
	}

	remaining := w.maxBytes - int64(w.buf.Len())
	if int64(len(p)) > remaining {
		if remaining > 0 {
			w.buf.Write(p[:remaining])
		}
		w.truncated = true
		return len(p), nil
	}

	w.buf.Write(p)
	return len(p), nil
}

func (w *boundedStderrWriter) Snapshot() (data []byte, truncated bool, seen int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make([]byte, w.buf.Len())
	copy(cp, w.buf.Bytes())
	return cp, w.truncated, w.seen
}

// DefaultGoIngestBinaryName returns the platform-appropriate binary name for go-ingest-adapter.
func DefaultGoIngestBinaryName() string {
	if runtime.GOOS == "windows" {
		return "go-ingest-adapter.exe"
	}
	return "go-ingest-adapter"
}
