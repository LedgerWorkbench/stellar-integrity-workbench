package evidence_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/assertion"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/evidence"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/source/localfile"
)

func makeSyntheticVerifiedInput(t *testing.T, payload []byte) (*localfile.VerifiedInput, string, int64) {
	t.Helper()
	rootDir := t.TempDir()
	relPath := "synthetic/input.xdr"
	fullPath := filepath.Join(rootDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(fullPath, payload, 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	sum := sha256.Sum256(payload)
	hashHex := hex.EncodeToString(sum[:])

	store, err := localfile.NewStore(localfile.DefaultConfig(rootDir))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	vi, err := store.LoadAndVerify(relPath, int64(len(payload)), hashHex)
	if err != nil {
		t.Fatalf("LoadAndVerify: %v", err)
	}
	return vi, hashHex, int64(len(payload))
}

func makeSyntheticCase(hashHex string, byteLen int64) *domain.Case {
	seq := int64(100)
	proto := 21
	return &domain.Case{
		SchemaVersion:  "1.0.0",
		CaseID:         "case_synthetic_unit7",
		Title:          "Synthetic Unit 7 Evidence Fixture",
		Summary:        "Strictly synthetic test fixture for Unit 7 evidence bundle tests.",
		Classification: domain.ClassificationSyntheticFixture,
		Source: domain.Source{
			Kind:  "synthetic_construction",
			Notes: "Synthetic test fixture",
		},
		Network: domain.Network{
			Name:            "Synthetic Standalone Network",
			ProtocolVersion: &proto,
			LedgerSequence:  &seq,
		},
		Input: domain.Input{
			Format:     "LedgerCloseMeta",
			Encoding:   "xdr_binary",
			SHA256:     hashHex,
			ByteLength: byteLen,
			ObjectPath: "synthetic/input.xdr",
		},
		Limitations: []string{"synthetic test fixture"},
	}
}

func makeSyntheticRunOutput(t *testing.T, runID, caseID, inputSHA string, execStatus string, exitCode int, projJSON string) *adapter.RunOutput {
	t.Helper()
	adapter.Init()

	var rawProj *json.RawMessage
	if projJSON != "" {
		rm := json.RawMessage(projJSON)
		rawProj = &rm
	}
	detectedType := "LedgerCloseMeta"
	if execStatus != adapter.ExecutionStatusCompleted {
		detectedType = ""
	}
	var dtPtr *string
	if detectedType != "" {
		dtPtr = &detectedType
	}
	decodeStatus := adapter.DecodeStatusSuccess
	if execStatus != adapter.ExecutionStatusCompleted {
		decodeStatus = adapter.DecodeStatusNotAttempted
	}

	res := &adapter.Result{
		SchemaVersion:  adapter.ProtocolVersion,
		RunID:          runID,
		CaseID:         caseID,
		AdapterID:      adapter.AdapterIDGoIngest,
		AdapterVersion: adapter.DefaultGoAdapterVersion,
		SDKVersion:     adapter.PinnedGoSDKVersion,
		Execution: adapter.ResultExecution{
			Status:     execStatus,
			ExitCode:   exitCode,
			DurationMs: 12,
		},
		Input: adapter.ResultInput{SHA256: inputSHA},
		Decode: adapter.ResultDecode{
			Status:       decodeStatus,
			DetectedType: dtPtr,
		},
		SemanticProjection: rawProj,
		Diagnostics:        []adapter.ResultDiagnostic{},
		CapabilitiesUsed:   []string{"decode_ledger_meta"},
	}

	// Marshal exact stdout bytes and run through ProcessRunner-like validation or direct construction
	// Note: for real ProcessRunner stdout capture tests, see TestProcessRunner_RealGoIngestAdapter_ProtocolIntegration.
	// Here we use a real ProcessRunner or helper to construct RunOutput. Let's use a real test adapter binary or
	// verify via ProcessRunner in an integration test below.
	outBytes, err := adapter.EncodeResult(res)
	if err != nil {
		t.Fatalf("EncodeResult: %v", err)
	}
	_ = outBytes
	return &adapter.RunOutput{
		Result:          res,
		Stderr:          []byte{},
		StderrTruncated: false,
		StderrBytesSeen: 0,
		ActualExitCode:  exitCode,
		ActualDuration:  12 * time.Millisecond,
	}
}

func makeValidBuildInput(t *testing.T, runID string, startedAt, completedAt time.Time) evidence.BuildInput {
	t.Helper()
	vi, hashHex, byteLen := makeSyntheticVerifiedInput(t, []byte("synthetic-ledger-bytes"))
	c := makeSyntheticCase(hashHex, byteLen)
	outcome := domain.RunOutcomePass
	run := &domain.Run{
		RunID:       runID,
		CaseID:      c.CaseID,
		AdapterID:   adapter.AdapterIDGoIngest,
		State:       domain.RunStateCompleted,
		Outcome:     &outcome,
		StartedAt:   startedAt,
		CompletedAt: &completedAt,
	}
	projJSON := `{"xdr_type":"LedgerCloseMeta","ledger":{"sequence":100},"transactions":{"total":0,"hashes":[],"hashes_truncated":false}}`
	out := makeSyntheticRunOutput(t, runID, c.CaseID, hashHex, adapter.ExecutionStatusCompleted, 0, projJSON)

	seq := int64(100)
	txCnt := 0
	eval, err := assertion.Evaluate([]domain.AssertionExpectation{
		{AssertionID: "assertion_seq", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: json.Number("100")},
		{AssertionID: "assertion_sha", Kind: domain.AssertionKindInputHashEquals, Expected: hashHex},
	}, assertion.EvaluationInput{
		Projection: &domain.SemanticProjection{
			DecodedType:       "LedgerCloseMeta",
			LedgerSequence:    &seq,
			TransactionCount:  &txCnt,
			TransactionHashes: []string{},
		},
		VerifiedInputSHA256: hashHex,
		SupportedKinds:      assertion.GoIngestSupportedKinds(),
		EvidenceRefs:        []string{"logical://input/sha256/" + hashHex},
	})
	if err != nil {
		t.Fatalf("assertion.Evaluate: %v", err)
	}

	return evidence.BuildInput{
		Case:          c,
		Run:           run,
		VerifiedInput: vi,
		AdapterTarget: adapter.RequestTarget{
			AdapterID:      adapter.AdapterIDGoIngest,
			AdapterVersion: adapter.DefaultGoAdapterVersion,
			SDKVersion:     adapter.PinnedGoSDKVersion,
		},
		AdapterOutput: out,
		Assertions:    eval.Results,
		Worker: evidence.WorkerMetadata{
			Version:   "0.1.0-test",
			CommitSHA: nil, // genuinely unavailable in unit test
		},
	}
}

func TestWriteAndVerifyBundle_SuccessAndTamperDetection(t *testing.T) {
	evRoot := t.TempDir()
	start := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Second)
	input := makeValidBuildInput(t, "run_unit7_success", start, end)

	wr, err := evidence.WriteBundle(evRoot, input)
	if err != nil {
		t.Fatalf("WriteBundle failed: %v", err)
	}
	if wr.Verification == nil || wr.Verification.RunID != "run_unit7_success" {
		t.Fatalf("unexpected verification result: %+v", wr.Verification)
	}
	if wr.Manifest.Worker.CommitSHA != nil {
		t.Errorf("expected nil Worker.CommitSHA, got %v", *wr.Manifest.Worker.CommitSHA)
	}
	if wr.Manifest.Run.State != domain.RunStateCompleted || *wr.Manifest.Run.Outcome != domain.RunOutcomePass {
		t.Errorf("unexpected run state/outcome: %s / %v", wr.Manifest.Run.State, wr.Manifest.Run.Outcome)
	}

	// Independent read-only verification must succeed and not modify files.
	projPath := filepath.Join(wr.BundleDir, "artifacts", "semantic-projection.json")
	beforeProj, err := os.ReadFile(projPath)
	if err != nil {
		t.Fatalf("read projection artifact: %v", err)
	}
	vr, err := evidence.VerifyBundle(evRoot, "run_unit7_success")
	if err != nil {
		t.Fatalf("VerifyBundle failed: %v", err)
	}
	if vr.ArtifactCount != 2 {
		t.Errorf("ArtifactCount = %d, want 2 (semantic-projection.json + assertions.json)", vr.ArtifactCount)
	}
	afterProj, err := os.ReadFile(projPath)
	if err != nil {
		t.Fatalf("read projection artifact after verify: %v", err)
	}
	if !bytes.Equal(beforeProj, afterProj) {
		t.Error("VerifyBundle modified stored artifact")
	}

	// Re-writing same run ID must fail with ErrBundleExists.
	if _, err := evidence.WriteBundle(evRoot, input); !errors.Is(err, evidence.ErrBundleExists) {
		t.Errorf("expected ErrBundleExists on duplicate WriteBundle, got %v", err)
	}
}

func TestWriteBundle_RealProcessRunnerStdoutPreserved(t *testing.T) {
	tmpBinDir := t.TempDir()
	binName := "go-ingest-adapter"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tmpBinDir, binName)

	buildCmd := exec.Command("go", "build", "-o", binPath, "github.com/LedgerWorkbench/stellar-integrity-workbench/cmd/go-ingest-adapter")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build go-ingest-adapter: %v\n%s", err, string(out))
	}

	adapter.Init()
	reg, err := adapter.NewProductionRegistry(adapter.ProductionRegistryConfig{
		GoIngestBinaryPath: binPath,
		GoIngestVersion:    adapter.DefaultGoAdapterVersion,
		GoSDKVersion:       adapter.PinnedGoSDKVersion,
	})
	if err != nil {
		t.Fatalf("NewProductionRegistry: %v", err)
	}
	runner, err := adapter.NewProcessRunner(adapter.ProcessRunnerConfig{Registry: reg})
	if err != nil {
		t.Fatalf("NewProcessRunner: %v", err)
	}

	// Stage malformed V1-discriminant synthetic bytes; adapter returns structured
	// decode error on stdout (exit code 3), which ProcessRunner validates and captures.
	vi, hashHex, byteLen := makeSyntheticVerifiedInput(t, []byte{0x00, 0x00, 0x00, 0x01, 0xde, 0xad, 0xbe, 0xef})
	c := makeSyntheticCase(hashHex, byteLen)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start := time.Now().UTC()
	out, err := runner.Run(ctx, adapter.RunParams{
		RunID:       "run_unit7_real_stdout",
		CaseID:      c.CaseID,
		Operation:   adapter.OperationDecodeLedgerMeta,
		InputFormat: adapter.InputFormatLedgerCloseMeta,
		Target: adapter.RequestTarget{
			AdapterID:      adapter.AdapterIDGoIngest,
			AdapterVersion: adapter.DefaultGoAdapterVersion,
			SDKVersion:     adapter.PinnedGoSDKVersion,
		},
		Limits: adapter.RequestLimits{
			TimeoutMs:      10000,
			MaxInputBytes:  1024 * 1024,
			MaxOutputBytes: 1024 * 1024,
		},
	}, vi)
	if err != nil {
		t.Fatalf("runner.Run: %v", err)
	}
	end := time.Now().UTC()

	evRoot := t.TempDir()
	wr, err := evidence.WriteBundle(evRoot, evidence.BuildInput{
		Case:          c,
		Run:           &domain.Run{RunID: "run_unit7_real_stdout", CaseID: c.CaseID, AdapterID: adapter.AdapterIDGoIngest, State: domain.RunStateFailed, StartedAt: start, CompletedAt: &end},
		VerifiedInput: vi,
		AdapterTarget: adapter.RequestTarget{
			AdapterID:      adapter.AdapterIDGoIngest,
			AdapterVersion: adapter.DefaultGoAdapterVersion,
			SDKVersion:     adapter.PinnedGoSDKVersion,
		},
		AdapterOutput: out,
		Worker:        evidence.WorkerMetadata{Version: "0.1.0-test", CommitSHA: nil},
	})
	if err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}

	storedStdout, err := os.ReadFile(filepath.Join(wr.BundleDir, "artifacts", "adapter-stdout.json"))
	if err != nil {
		t.Fatalf("read stored adapter-stdout.json: %v", err)
	}
	if !bytes.Equal(storedStdout, out.StdoutCopy()) {
		t.Error("stored adapter-stdout.json does not match ProcessRunner StdoutCopy() byte-for-byte")
	}
}

func TestWriteBundle_RegressionAndFailedStates(t *testing.T) {
	evRoot := t.TempDir()
	start := time.Date(2026, 11, 27, 8, 5, 0, 0, time.UTC)
	end := start.Add(1 * time.Second)

	t.Run("completed_regression_preserved", func(t *testing.T) {
		in := makeValidBuildInput(t, "run_unit7_regression", start, end)
		reg := domain.RunOutcomeRegression
		in.Run.Outcome = &reg
		in.Assertions[0].Status = domain.AssertionStatusFail

		wr, err := evidence.WriteBundle(evRoot, in)
		if err != nil {
			t.Fatalf("WriteBundle failed: %v", err)
		}
		if wr.Manifest.Run.State != domain.RunStateCompleted {
			t.Errorf("State = %q, want completed", wr.Manifest.Run.State)
		}
		if wr.Manifest.Run.Outcome == nil || *wr.Manifest.Run.Outcome != domain.RunOutcomeRegression {
			t.Errorf("Outcome = %v, want regression", wr.Manifest.Run.Outcome)
		}
	})

	t.Run("failed_run_has_nil_outcome", func(t *testing.T) {
		in := makeValidBuildInput(t, "run_unit7_failed", start, end)
		in.Run.State = domain.RunStateFailed
		in.Run.Outcome = nil
		in.Assertions = nil
		in.AdapterOutput = nil

		wr, err := evidence.WriteBundle(evRoot, in)
		if err != nil {
			t.Fatalf("WriteBundle failed for operational failure: %v", err)
		}
		if wr.Manifest.Run.State != domain.RunStateFailed {
			t.Errorf("State = %q, want failed", wr.Manifest.Run.State)
		}
		if wr.Manifest.Run.Outcome != nil {
			t.Errorf("Outcome = %v, want nil for failed run", *wr.Manifest.Run.Outcome)
		}
	})

	t.Run("failed_run_with_fabricated_outcome_rejected", func(t *testing.T) {
		in := makeValidBuildInput(t, "run_unit7_bad_failed", start, end)
		in.Run.State = domain.RunStateFailed
		// Outcome is still RunOutcomePass -> must be rejected!
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput, got %v", err)
		}
		if _, err := os.Stat(filepath.Join(evRoot, "run_unit7_bad_failed")); !errors.Is(err, os.ErrNotExist) {
			t.Error("expected no bundle directory created on input validation failure")
		}
	})

	t.Run("partial_write_cleanup_on_invalid_projection", func(t *testing.T) {
		in := makeValidBuildInput(t, "run_unit7_partial", start, end)
		badProj := json.RawMessage(`not-json`)
		in.AdapterOutput.Result.SemanticProjection = &badProj
		if _, err := evidence.WriteBundle(evRoot, in); err == nil {
			t.Fatal("expected error for invalid semantic_projection JSON")
		}
		if _, err := os.Stat(filepath.Join(evRoot, "run_unit7_partial")); !errors.Is(err, os.ErrNotExist) {
			t.Error("expected partial run directory to be absent")
		}
	})
}

func TestVerifyBundle_DetectsTamperingAndViolations(t *testing.T) {
	setupBundle := func(t *testing.T, runID string) string {
		t.Helper()
		evRoot := t.TempDir()
		start := time.Date(2026, 11, 27, 8, 10, 0, 0, time.UTC)
		in := makeValidBuildInput(t, runID, start, start.Add(time.Second))
		if _, err := evidence.WriteBundle(evRoot, in); err != nil {
			t.Fatalf("WriteBundle: %v", err)
		}
		return evRoot
	}

	t.Run("modified_artifact_detected", func(t *testing.T) {
		evRoot := setupBundle(t, "run_tamper_mod")
		artPath := filepath.Join(evRoot, "run_tamper_mod", "artifacts", "semantic-projection.json")
		_ = os.Chmod(artPath, 0o600)
		// Same length modification to specifically trigger SHA-256 mismatch
		orig, _ := os.ReadFile(artPath)
		orig[1] = 'X'
		_ = os.WriteFile(artPath, orig, 0o400)

		_, err := evidence.VerifyBundle(evRoot, "run_tamper_mod")
		if !errors.Is(err, evidence.ErrHashMismatch) {
			t.Errorf("expected ErrHashMismatch, got %v", err)
		}
	})

	t.Run("byte_length_mismatch_detected", func(t *testing.T) {
		evRoot := setupBundle(t, "run_tamper_len")
		artPath := filepath.Join(evRoot, "run_tamper_len", "artifacts", "semantic-projection.json")
		_ = os.Chmod(artPath, 0o600)
		_ = os.WriteFile(artPath, []byte(`{"x":1}`), 0o400)

		_, err := evidence.VerifyBundle(evRoot, "run_tamper_len")
		if !errors.Is(err, evidence.ErrByteLengthMismatch) {
			t.Errorf("expected ErrByteLengthMismatch, got %v", err)
		}
	})

	t.Run("missing_artifact_detected", func(t *testing.T) {
		evRoot := setupBundle(t, "run_tamper_missing")
		artPath := filepath.Join(evRoot, "run_tamper_missing", "artifacts", "assertions.json")
		if err := os.Remove(artPath); err != nil {
			t.Fatalf("remove artifact: %v", err)
		}
		_, err := evidence.VerifyBundle(evRoot, "run_tamper_missing")
		if !errors.Is(err, evidence.ErrArtifactMissing) {
			t.Errorf("expected ErrArtifactMissing, got %v", err)
		}
	})

	t.Run("extra_unlisted_artifact_detected", func(t *testing.T) {
		evRoot := setupBundle(t, "run_tamper_extra")
		extraPath := filepath.Join(evRoot, "run_tamper_extra", "artifacts", "rogue.json")
		if err := os.WriteFile(extraPath, []byte(`{}`), 0o400); err != nil {
			t.Fatalf("write extra: %v", err)
		}
		_, err := evidence.VerifyBundle(evRoot, "run_tamper_extra")
		if !errors.Is(err, evidence.ErrExtraArtifact) {
			t.Errorf("expected ErrExtraArtifact, got %v", err)
		}
	})

	t.Run("symlink_rejected", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires elevated privileges on Windows")
		}
		evRoot := setupBundle(t, "run_tamper_sym")
		artPath := filepath.Join(evRoot, "run_tamper_sym", "artifacts", "assertions.json")
		target := filepath.Join(evRoot, "outside.json")
		_ = os.WriteFile(target, []byte(`[]`), 0o600)
		_ = os.Remove(artPath)
		if err := os.Symlink(target, artPath); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		_, err := evidence.VerifyBundle(evRoot, "run_tamper_sym")
		if !errors.Is(err, evidence.ErrSymlinkRejected) {
			t.Errorf("expected ErrSymlinkRejected, got %v", err)
		}
	})

	t.Run("path_traversal_run_id_rejected", func(t *testing.T) {
		evRoot := t.TempDir()
		_, err := evidence.VerifyBundle(evRoot, "../escape")
		if !errors.Is(err, evidence.ErrUnsafePath) {
			t.Errorf("expected ErrUnsafePath, got %v", err)
		}
	})

	t.Run("run_id_mismatch_rejected", func(t *testing.T) {
		evRoot := setupBundle(t, "run_dir_a")
		if err := os.Rename(filepath.Join(evRoot, "run_dir_a"), filepath.Join(evRoot, "run_dir_b")); err != nil {
			t.Fatalf("rename: %v", err)
		}
		_, err := evidence.VerifyBundle(evRoot, "run_dir_b")
		if !errors.Is(err, evidence.ErrRunIDMismatch) {
			t.Errorf("expected ErrRunIDMismatch, got %v", err)
		}
	})
}

func TestManifestDecodeValidation(t *testing.T) {
	evRoot := t.TempDir()
	start := time.Date(2026, 11, 27, 8, 15, 0, 0, time.UTC)
	wr, err := evidence.WriteBundle(evRoot, makeValidBuildInput(t, "run_manifest_check", start, start.Add(time.Second)))
	if err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}
	validBytes, err := os.ReadFile(wr.ManifestPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	t.Run("valid_manifest_passes", func(t *testing.T) {
		if _, err := evidence.DecodeAndValidateManifest(validBytes); err != nil {
			t.Fatalf("DecodeAndValidateManifest failed: %v", err)
		}
	})

	t.Run("unsupported_major_version", func(t *testing.T) {
		bad := bytes.Replace(validBytes, []byte(`"1.0.0"`), []byte(`"2.0.0"`), 1)
		if _, err := evidence.DecodeAndValidateManifest(bad); !errors.Is(err, evidence.ErrUnsupportedSchemaVersion) {
			t.Errorf("expected ErrUnsupportedSchemaVersion, got %v", err)
		}
	})

	t.Run("trailing_json_rejected", func(t *testing.T) {
		bad := append(append([]byte(nil), validBytes...), []byte(`{"extra":1}`)...)
		if _, err := evidence.DecodeAndValidateManifest(bad); !errors.Is(err, evidence.ErrManifestInvalid) {
			t.Errorf("expected ErrManifestInvalid for trailing JSON, got %v", err)
		}
	})

	t.Run("unknown_field_rejected", func(t *testing.T) {
		bad := bytes.Replace(validBytes, []byte(`"schema_version": "1.0.0",`), []byte(`"schema_version": "1.0.0", "unknown_extra": 1,`), 1)
		if _, err := evidence.DecodeAndValidateManifest(bad); !errors.Is(err, evidence.ErrManifestInvalid) {
			t.Errorf("expected ErrManifestInvalid for unknown field, got %v", err)
		}
	})
}

func TestDeterminism_TimestampsDoNotAffectSemanticArtifacts(t *testing.T) {
	evRoot := t.TempDir()
	t1 := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 27, 12, 30, 0, 0, time.UTC)

	wr1, err := evidence.WriteBundle(evRoot, makeValidBuildInput(t, "run_det_1", t1, t1.Add(time.Second)))
	if err != nil {
		t.Fatalf("WriteBundle 1: %v", err)
	}
	wr2, err := evidence.WriteBundle(evRoot, makeValidBuildInput(t, "run_det_2", t2, t2.Add(5*time.Second)))
	if err != nil {
		t.Fatalf("WriteBundle 2: %v", err)
	}

	hashByRole := func(arts []evidence.ArtifactRef) map[string]string {
		m := make(map[string]string, len(arts))
		for _, a := range arts {
			m[a.Role] = a.SHA256
		}
		return m
	}

	h1 := hashByRole(wr1.Manifest.Artifacts)
	h2 := hashByRole(wr2.Manifest.Artifacts)

	if h1[evidence.ArtifactRoleSemanticProjection] != h2[evidence.ArtifactRoleSemanticProjection] {
		t.Errorf("semantic projection hashes differed across timestamps: %s vs %s",
			h1[evidence.ArtifactRoleSemanticProjection], h2[evidence.ArtifactRoleSemanticProjection])
	}
	if h1[evidence.ArtifactRoleAssertions] != h2[evidence.ArtifactRoleAssertions] {
		t.Errorf("assertions hashes differed across timestamps: %s vs %s",
			h1[evidence.ArtifactRoleAssertions], h2[evidence.ArtifactRoleAssertions])
	}
}

func TestWriteBundle_InputAndAdapterCrossChecks(t *testing.T) {
	evRoot := t.TempDir()
	start := time.Date(2026, 11, 27, 8, 20, 0, 0, time.UTC)

	t.Run("case_input_hash_mismatch_rejected", func(t *testing.T) {
		in := makeValidBuildInput(t, "run_xcheck_1", start, start.Add(time.Second))
		in.Case.Input.SHA256 = strings.Repeat("0", 64)
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput, got %v", err)
		}
	})

	t.Run("adapter_reported_input_hash_mismatch_rejected", func(t *testing.T) {
		in := makeValidBuildInput(t, "run_xcheck_2", start, start.Add(time.Second))
		in.AdapterOutput.Result.Input.SHA256 = strings.Repeat("f", 64)
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput, got %v", err)
		}
	})
}
