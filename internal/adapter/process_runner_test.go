package adapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/source/localfile"
)

var (
	testAdapterBinaryPath  string
	realGoIngestBinaryPath string
	testBuildDir           string
)

// TestMain builds the test-only protocol helper binary and the real go-ingest-adapter
// binary once for all process runner tests, and cleans them up on exit.
func TestMain(m *testing.M) {
	Init()

	var err error
	testBuildDir, err = os.MkdirTemp("", "siw-adapter-testbin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create test build dir: %v\n", err)
		os.Exit(1)
	}

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}

	// 1. Build test-only adapter helper (exists only in test code).
	helperSrc := filepath.Join(testBuildDir, "helper.go")
	if err := os.WriteFile(helperSrc, []byte(testAdapterSource), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write test adapter source: %v\n", err)
		_ = os.RemoveAll(testBuildDir)
		os.Exit(1)
	}
	testAdapterBinaryPath = filepath.Join(testBuildDir, "test-adapter"+ext)
	buildHelper := exec.Command("go", "build", "-o", testAdapterBinaryPath, helperSrc)
	if out, err := buildHelper.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build test adapter: %v\n%s\n", err, out)
		_ = os.RemoveAll(testBuildDir)
		os.Exit(1)
	}

	// 2. Build real go-ingest-adapter binary for integration verification.
	realGoIngestBinaryPath = filepath.Join(testBuildDir, "go-ingest-adapter"+ext)
	buildReal := exec.Command("go", "build", "-o", realGoIngestBinaryPath, "../../cmd/go-ingest-adapter")
	if out, err := buildReal.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build go-ingest-adapter: %v\n%s\n", err, out)
		_ = os.RemoveAll(testBuildDir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(testBuildDir)
	os.Exit(code)
}

// testAdapterSource is a standalone test-only process implementation that simulates
// well-defined protocol responses for testing ProcessRunner. It is NOT a Stellar SDK
// or XDR conformance adapter and is NEVER registered in production.
const testAdapterSource = `package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type reqTarget struct {
	AdapterID      string ` + "`json:\"adapter_id\"`" + `
	AdapterVersion string ` + "`json:\"adapter_version\"`" + `
	SDKVersion     string ` + "`json:\"sdk_version\"`" + `
}

type req struct {
	SchemaVersion string    ` + "`json:\"schema_version\"`" + `
	RunID         string    ` + "`json:\"run_id\"`" + `
	CaseID        string    ` + "`json:\"case_id\"`" + `
	Operation     string    ` + "`json:\"operation\"`" + `
	InputPath     string    ` + "`json:\"input_path\"`" + `
	InputFormat   string    ` + "`json:\"input_format\"`" + `
	InputSHA256   string    ` + "`json:\"input_sha256\"`" + `
	Target        reqTarget ` + "`json:\"target\"`" + `
}

type resExec struct {
	Status     string ` + "`json:\"status\"`" + `
	ExitCode   int    ` + "`json:\"exit_code\"`" + `
	DurationMs int64  ` + "`json:\"duration_ms\"`" + `
}

type resInput struct {
	SHA256 string ` + "`json:\"sha256\"`" + `
}

type resDecode struct {
	Status       string  ` + "`json:\"status\"`" + `
	DetectedType *string ` + "`json:\"detected_type\"`" + `
}

type resDiag struct {
	Stage   string ` + "`json:\"stage\"`" + `
	Code    string ` + "`json:\"code\"`" + `
	Message string ` + "`json:\"message\"`" + `
}

type result struct {
	SchemaVersion      string      ` + "`json:\"schema_version\"`" + `
	RunID              string      ` + "`json:\"run_id\"`" + `
	CaseID             string      ` + "`json:\"case_id\"`" + `
	AdapterID          string      ` + "`json:\"adapter_id\"`" + `
	AdapterVersion     string      ` + "`json:\"adapter_version\"`" + `
	SDKVersion         string      ` + "`json:\"sdk_version\"`" + `
	Execution          resExec     ` + "`json:\"execution\"`" + `
	Input              resInput    ` + "`json:\"input\"`" + `
	Decode             resDecode   ` + "`json:\"decode\"`" + `
	SemanticProjection any         ` + "`json:\"semantic_projection\"`" + `
	Diagnostics        []resDiag   ` + "`json:\"diagnostics\"`" + `
	CapabilitiesUsed   []string    ` + "`json:\"capabilities_used\"`" + `
}

func main() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdin read error: %v\n", err)
		os.Exit(1)
	}

	var r req
	if err := json.Unmarshal(raw, &r); err != nil {
		fmt.Fprintf(os.Stderr, "json unmarshal error: %v\n", err)
		os.Exit(1)
	}

	// Read staged input file and compute SHA-256.
	inputBytes, err := os.ReadFile(r.InputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read input_path %s: %v\n", r.InputPath, err)
		os.Exit(2)
	}
	sum := sha256.Sum256(inputBytes)
	actualHash := hex.EncodeToString(sum[:])

	baseRes := func(status string, exitCode int, decodeStatus string) result {
		return result{
			SchemaVersion:  "1",
			RunID:          r.RunID,
			CaseID:         r.CaseID,
			AdapterID:      r.Target.AdapterID,
			AdapterVersion: r.Target.AdapterVersion,
			SDKVersion:     r.Target.SDKVersion,
			Execution: resExec{
				Status:     status,
				ExitCode:   exitCode,
				DurationMs: 1,
			},
			Input: resInput{SHA256: actualHash},
			Decode: resDecode{
				Status:       decodeStatus,
				DetectedType: nil,
			},
			SemanticProjection: nil,
			Diagnostics:        []resDiag{},
			CapabilitiesUsed:   []string{},
		}
	}

	switch r.CaseID {
	case "case_success":
		fmt.Fprintln(os.Stderr, "test-adapter: diagnostic message on stderr")
		out, _ := json.Marshal(baseRes("completed", 0, "not_attempted"))
		fmt.Println(string(out))
		os.Exit(0)

	case "case_check_env":
		// Verify stripped environment: secret env var must not be present.
		if os.Getenv("SIW_TEST_SECRET_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "" {
			fmt.Fprintln(os.Stderr, "secret environment variable leaked")
			os.Exit(9)
		}
		out, _ := json.Marshal(baseRes("completed", 0, "not_attempted"))
		fmt.Println(string(out))
		os.Exit(0)

	case "case_check_readonly":
		info, err := os.Stat(r.InputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat failed: %v\n", err)
			os.Exit(2)
		}
		if info.Mode().Perm()&0o222 != 0 {
			fmt.Fprintf(os.Stderr, "input file is writable: mode %v\n", info.Mode().Perm())
			os.Exit(2)
		}
		out, _ := json.Marshal(baseRes("completed", 0, "not_attempted"))
		fmt.Println(string(out))
		os.Exit(0)

	case "case_failure":
		fmt.Fprintln(os.Stderr, "test-adapter: simulated failure")
		res := baseRes("failed", 3, "invalid_input")
		res.Diagnostics = []resDiag{{Stage: "decode", Code: "bad_xdr", Message: "simulated invalid input"}}
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
		os.Exit(3)

	case "case_malformed_json":
		fmt.Print("{not valid json")
		os.Exit(0)

	case "case_extra_json":
		out, _ := json.Marshal(baseRes("completed", 0, "not_attempted"))
		fmt.Print(string(out))
		fmt.Print(` + "`" + `{"extra":true}` + "`" + `)
		os.Exit(0)

	case "case_trailing_brace":
		out, _ := json.Marshal(baseRes("completed", 0, "not_attempted"))
		fmt.Print(string(out))
		fmt.Print("}")
		os.Exit(0)

	case "case_empty_stdout":
		fmt.Fprintln(os.Stderr, "no stdout produced")
		os.Exit(1)

	case "case_mismatched_run_id":
		res := baseRes("completed", 0, "not_attempted")
		res.RunID = "run_wrong"
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
		os.Exit(0)

	case "case_mismatched_case_id":
		res := baseRes("completed", 0, "not_attempted")
		res.CaseID = "case_wrong"
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
		os.Exit(0)

	case "case_mismatched_adapter_id":
		res := baseRes("completed", 0, "not_attempted")
		res.AdapterID = "other-adapter"
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
		os.Exit(0)

	case "case_mismatched_sdk_version":
		res := baseRes("completed", 0, "not_attempted")
		res.SDKVersion = "v9.9.9"
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
		os.Exit(0)

	case "case_mismatched_input_hash":
		res := baseRes("completed", 0, "not_attempted")
		res.Input.SHA256 = strings.Repeat("f", 64)
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
		os.Exit(0)

	case "case_exit_code_mismatch":
		res := baseRes("completed", 0, "not_attempted")
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
		os.Exit(2)

	case "case_timeout":
		time.Sleep(10 * time.Second)
		os.Exit(0)

	case "case_oversized_stdout":
		chunk := strings.Repeat("A", 4096)
		for i := 0; i < 512; i++ {
			fmt.Print(chunk)
		}
		os.Exit(0)

	case "case_oversized_stderr":
		chunk := strings.Repeat("E", 4096)
		for i := 0; i < 512; i++ {
			fmt.Fprint(os.Stderr, chunk)
		}
		out, _ := json.Marshal(baseRes("completed", 0, "not_attempted"))
		fmt.Println(string(out))
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "unknown test case_id: %s\n", r.CaseID)
		os.Exit(1)
	}
}
`

// makeVerifiedInput creates a real *localfile.VerifiedInput through localfile.Store
// so that tests exercise the authentic verified-input boundary.
func makeVerifiedInput(t *testing.T, payload []byte) (*localfile.VerifiedInput, string, string) {
	t.Helper()
	corpusDir := t.TempDir()
	relPath := "corpus/sample.xdr"
	fullPath := filepath.Join(corpusDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(payload)
	hexHash := hex.EncodeToString(sum[:])

	store, err := localfile.NewStore(localfile.DefaultConfig(corpusDir))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	verified, err := store.LoadAndVerify(relPath, int64(len(payload)), hexHash)
	if err != nil {
		t.Fatal(err)
	}
	return verified, fullPath, hexHash
}

func newTestRunner(t *testing.T, binPath string, baseTempDir string, maxStderr int64) *ProcessRunner {
	t.Helper()
	reg, err := NewProductionRegistry(ProductionRegistryConfig{
		GoIngestBinaryPath: binPath,
		GoIngestVersion:    DefaultGoAdapterVersion,
		GoSDKVersion:       PinnedGoSDKVersion,
	})
	if err != nil {
		t.Fatalf("NewProductionRegistry failed: %v", err)
	}

	runner, err := NewProcessRunner(ProcessRunnerConfig{
		Registry:       reg,
		BaseTempDir:    baseTempDir,
		MaxStderrBytes: maxStderr,
	})
	if err != nil {
		t.Fatalf("NewProcessRunner failed: %v", err)
	}
	return runner
}

func baseRunParams(caseID string) RunParams {
	return RunParams{
		RunID:       "run_test_001",
		CaseID:      caseID,
		Operation:   OperationDecodeLedgerMeta,
		InputFormat: InputFormatLedgerCloseMeta,
		Target: RequestTarget{
			AdapterID:      AdapterIDGoIngest,
			AdapterVersion: DefaultGoAdapterVersion,
			SDKVersion:     PinnedGoSDKVersion,
		},
		Limits: RequestLimits{
			TimeoutMs:      5000,
			MaxInputBytes:  64 * 1024 * 1024,
			MaxOutputBytes: 1024 * 1024,
		},
	}
}

func TestProcessRunner_SuccessAndStagingAndCleanup(t *testing.T) {
	payload := []byte("exact-binary-ledger-close-meta-bytes-\x00\x01\x02\xff")
	verified, corpusFile, expectedHash := makeVerifiedInput(t, payload)
	baseTemp := t.TempDir()

	runner := newTestRunner(t, testAdapterBinaryPath, baseTemp, 0)
	out, err := runner.Run(context.Background(), baseRunParams("case_success"), verified)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if out.Result.RunID != "run_test_001" {
		t.Fatalf("unexpected run_id: %q", out.Result.RunID)
	}
	if out.Result.Input.SHA256 != expectedHash {
		t.Fatalf("staged input hash mismatch: got %q, want %q", out.Result.Input.SHA256, expectedHash)
	}
	if !bytes.Contains(out.Stderr, []byte("diagnostic message on stderr")) {
		t.Fatalf("expected stderr diagnostic, got %q", string(out.Stderr))
	}
	if out.ActualExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", out.ActualExitCode)
	}

	// Verify temporary directory was removed.
	entries, err := os.ReadDir(baseTemp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected per-run temp dir to be cleaned up, found %v", entries)
	}

	// Verify original corpus file was untouched.
	afterCorpus, err := os.ReadFile(corpusFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterCorpus, payload) {
		t.Fatal("original corpus file was modified")
	}
}

func TestProcessRunner_StagedInputIsReadOnly(t *testing.T) {
	verified, _, _ := makeVerifiedInput(t, []byte("readonly-check"))
	runner := newTestRunner(t, testAdapterBinaryPath, t.TempDir(), 0)

	_, err := runner.Run(context.Background(), baseRunParams("case_check_readonly"), verified)
	if err != nil {
		t.Fatalf("readonly check failed: %v", err)
	}
}

func TestProcessRunner_MinimalEnvironment(t *testing.T) {
	t.Setenv("SIW_TEST_SECRET_TOKEN", "super-secret-value")
	t.Setenv("GITHUB_TOKEN", "ghp_secret")

	verified, _, _ := makeVerifiedInput(t, []byte("env-check"))
	runner := newTestRunner(t, testAdapterBinaryPath, t.TempDir(), 0)

	_, err := runner.Run(context.Background(), baseRunParams("case_check_env"), verified)
	if err != nil {
		t.Fatalf("minimal environment check failed: %v", err)
	}
}

func TestProcessRunner_ValidFailureResultAccepted(t *testing.T) {
	verified, _, _ := makeVerifiedInput(t, []byte("failure-check"))
	runner := newTestRunner(t, testAdapterBinaryPath, t.TempDir(), 0)

	out, err := runner.Run(context.Background(), baseRunParams("case_failure"), verified)
	if err != nil {
		t.Fatalf("expected valid failed protocol result to be accepted, got: %v", err)
	}
	if out.Result.Execution.Status != ExecutionStatusFailed {
		t.Fatalf("expected execution status %q, got %q", ExecutionStatusFailed, out.Result.Execution.Status)
	}
	if out.ActualExitCode != 3 {
		t.Fatalf("expected actual exit code 3, got %d", out.ActualExitCode)
	}
}

func TestProcessRunner_MalformedAndInvalidResponsesRejected(t *testing.T) {
	verified, _, _ := makeVerifiedInput(t, []byte("response-validation"))
	baseTemp := t.TempDir()
	runner := newTestRunner(t, testAdapterBinaryPath, baseTemp, 0)

	tests := []struct {
		caseID  string
		wantErr error
	}{
		{"case_malformed_json", ErrAdapterMalformedOutput},
		{"case_extra_json", ErrAdapterMalformedOutput},
		{"case_trailing_brace", ErrAdapterMalformedOutput},
		{"case_empty_stdout", ErrAdapterProcessFailure},
		{"case_mismatched_run_id", ErrAdapterResponseMismatch},
		{"case_mismatched_case_id", ErrAdapterResponseMismatch},
		{"case_mismatched_adapter_id", ErrAdapterIdentityMismatch},
		{"case_mismatched_sdk_version", ErrAdapterIdentityMismatch},
		{"case_mismatched_input_hash", ErrInputHashMismatch},
		{"case_exit_code_mismatch", ErrAdapterResponseMismatch},
	}

	for _, tt := range tests {
		t.Run(tt.caseID, func(t *testing.T) {
			_, err := runner.Run(context.Background(), baseRunParams(tt.caseID), verified)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("case %q: expected error %v, got %v", tt.caseID, tt.wantErr, err)
			}
		})
	}

	// Verify cleanup even after failures.
	entries, err := os.ReadDir(baseTemp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected temp dir to be cleaned up after failures, found %v", entries)
	}
}

func TestProcessRunner_TimeoutKillsProcess(t *testing.T) {
	verified, _, _ := makeVerifiedInput(t, []byte("timeout-check"))
	baseTemp := t.TempDir()
	runner := newTestRunner(t, testAdapterBinaryPath, baseTemp, 0)

	params := baseRunParams("case_timeout")
	params.Limits.TimeoutMs = 150

	start := time.Now()
	_, err := runner.Run(context.Background(), params, verified)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrAdapterTimeout) {
		t.Fatalf("expected ErrAdapterTimeout, got %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("process took too long to be killed on timeout: %v", elapsed)
	}

	entries, _ := os.ReadDir(baseTemp)
	if len(entries) != 0 {
		t.Fatalf("expected temp dir cleanup after timeout, found %v", entries)
	}
}

func TestProcessRunner_ContextCancellationStopsProcess(t *testing.T) {
	verified, _, _ := makeVerifiedInput(t, []byte("cancel-check"))
	baseTemp := t.TempDir()
	runner := newTestRunner(t, testAdapterBinaryPath, baseTemp, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_, err := runner.Run(ctx, baseRunParams("case_timeout"), verified)
	if !errors.Is(err, ErrAdapterCancelled) {
		t.Fatalf("expected ErrAdapterCancelled, got %v", err)
	}

	entries, _ := os.ReadDir(baseTemp)
	if len(entries) != 0 {
		t.Fatalf("expected temp dir cleanup after cancellation, found %v", entries)
	}
}

func TestProcessRunner_OversizedStdoutDetectedAndBounded(t *testing.T) {
	verified, _, _ := makeVerifiedInput(t, []byte("stdout-limit-check"))
	runner := newTestRunner(t, testAdapterBinaryPath, t.TempDir(), 0)

	params := baseRunParams("case_oversized_stdout")
	params.Limits.MaxOutputBytes = 8192

	_, err := runner.Run(context.Background(), params, verified)
	if !errors.Is(err, ErrAdapterOutputTooLarge) {
		t.Fatalf("expected ErrAdapterOutputTooLarge, got %v", err)
	}
}

func TestProcessRunner_OversizedStderrBounded(t *testing.T) {
	verified, _, _ := makeVerifiedInput(t, []byte("stderr-limit-check"))
	const stderrLimit int64 = 4096
	runner := newTestRunner(t, testAdapterBinaryPath, t.TempDir(), stderrLimit)

	out, err := runner.Run(context.Background(), baseRunParams("case_oversized_stderr"), verified)
	if err != nil {
		t.Fatalf("expected success with bounded stderr, got: %v", err)
	}
	if int64(len(out.Stderr)) > stderrLimit {
		t.Fatalf("stderr exceeded bound %d: got %d", stderrLimit, len(out.Stderr))
	}
	if !out.StderrTruncated {
		t.Fatal("expected StderrTruncated to be true")
	}
	if out.StderrBytesSeen <= stderrLimit {
		t.Fatalf("expected StderrBytesSeen > %d, got %d", stderrLimit, out.StderrBytesSeen)
	}
}

func TestProcessRunner_RejectsOversizedInputBeforeLaunch(t *testing.T) {
	verified, _, _ := makeVerifiedInput(t, bytes.Repeat([]byte("x"), 1024))
	runner := newTestRunner(t, testAdapterBinaryPath, t.TempDir(), 0)

	params := baseRunParams("case_success")
	params.Limits.MaxInputBytes = 512

	_, err := runner.Run(context.Background(), params, verified)
	if !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("expected ErrInputTooLarge, got %v", err)
	}
}

func TestProcessRunner_RejectsNilVerifiedInput(t *testing.T) {
	runner := newTestRunner(t, testAdapterBinaryPath, t.TempDir(), 0)
	_, err := runner.Run(context.Background(), baseRunParams("case_success"), nil)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestProcessRunner_RealGoIngestAdapter_ProtocolIntegration(t *testing.T) {
	verified, _, expectedHash := makeVerifiedInput(t, []byte("dummy-xdr-for-protocol-test"))
	runner := newTestRunner(t, realGoIngestBinaryPath, t.TempDir(), 0)

	out, err := runner.Run(context.Background(), baseRunParams("case_unit4_protocol"), verified)
	if err != nil {
		t.Fatalf("expected valid protocol response from go-ingest-adapter, got error: %v", err)
	}

	if out.Result.AdapterID != AdapterIDGoIngest {
		t.Fatalf("expected adapter_id %q, got %q", AdapterIDGoIngest, out.Result.AdapterID)
	}
	if out.Result.Input.SHA256 != expectedHash {
		t.Fatalf("input hash mismatch: got %q, want %q", out.Result.Input.SHA256, expectedHash)
	}
	if out.Result.Execution.Status != ExecutionStatusFailed {
		t.Fatalf("expected execution.status %q for non-XDR input, got %q",
			ExecutionStatusFailed, out.Result.Execution.Status)
	}
	if out.Result.Decode.Status != DecodeStatusInvalidInput {
		t.Fatalf("expected decode.status %q for non-XDR input, got %q",
			DecodeStatusInvalidInput, out.Result.Decode.Status)
	}
	if out.Result.SemanticProjection != nil {
		t.Fatalf("expected nil semantic_projection for non-XDR input")
	}
	if len(out.Result.Diagnostics) == 0 || out.Result.Diagnostics[0].Code != "xdr_unmarshal_failed" {
		t.Fatalf("expected xdr_unmarshal_failed diagnostic, got %+v", out.Result.Diagnostics)
	}
}
