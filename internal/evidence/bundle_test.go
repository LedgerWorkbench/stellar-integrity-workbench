package evidence_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter/goingest"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/assertion"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/evidence"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/source/localfile"
)

var testGoIngestBinary string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "siw-evidence-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	binName := "go-ingest-adapter"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	testGoIngestBinary = filepath.Join(tmpDir, binName)

	buildCmd := exec.Command("go", "build", "-o", testGoIngestBinary,
		"github.com/LedgerWorkbench/stellar-integrity-workbench/cmd/go-ingest-adapter")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build go-ingest-adapter: %v\n%s\n", err, string(out))
		_ = os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	adapter.Init()
	exitCode := m.Run()
	if err := os.RemoveAll(tmpDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to clean up temp dir %s: %v\n", tmpDir, err)
	}
	os.Exit(exitCode)
}

func makeSyntheticLedgerXDR(t *testing.T, seq uint32, protocol uint32) []byte {
	t.Helper()
	var h xdr.Hash
	h[0] = 0x11
	var txHash xdr.Hash
	txHash[0] = 0x22

	var src xdr.MuxedAccount
	src.Type = xdr.CryptoKeyTypeKeyTypeEd25519
	var pk xdr.Uint256
	pk[0] = 0x33
	src.Ed25519 = &pk

	components := []xdr.TxSetComponent{
		{
			Type: xdr.TxSetComponentTypeTxsetCompTxsMaybeDiscountedFee,
			TxsMaybeDiscountedFee: &xdr.TxSetComponentTxsMaybeDiscountedFee{
				Txs: []xdr.TransactionEnvelope{
					{
						Type: xdr.EnvelopeTypeEnvelopeTypeTx,
						V1: &xdr.TransactionV1Envelope{
							Tx: xdr.Transaction{
								SourceAccount: src,
								Fee:           100,
								SeqNum:        1,
							},
						},
					},
				},
			},
		},
	}

	lcm := xdr.LedgerCloseMeta{
		V: 1,
		V1: &xdr.LedgerCloseMetaV1{
			LedgerHeader: xdr.LedgerHeaderHistoryEntry{
				Hash: h,
				Header: xdr.LedgerHeader{
					LedgerVersion: xdr.Uint32(protocol),
					ScpValue:      xdr.StellarValue{CloseTime: 1700000100},
					LedgerSeq:     xdr.Uint32(seq),
				},
			},
			TxSet: xdr.GeneralizedTransactionSet{
				V: 1,
				V1TxSet: &xdr.TransactionSetV1{
					Phases: []xdr.TransactionPhase{{V: 0, V0Components: &components}},
				},
			},
			TxProcessing: []xdr.TransactionResultMeta{
				{
					Result: xdr.TransactionResultPair{
						TransactionHash: txHash,
						Result: xdr.TransactionResult{
							FeeCharged: 100,
							Result: xdr.TransactionResultResult{
								Code:    xdr.TransactionResultCodeTxSuccess,
								Results: &[]xdr.OperationResult{},
							},
						},
					},
					TxApplyProcessing: xdr.TransactionMeta{V: 0, Operations: &[]xdr.OperationMeta{}},
				},
			},
		},
	}

	var buf bytes.Buffer
	if _, err := xdr.Marshal(&buf, lcm); err != nil {
		t.Fatalf("marshal synthetic LCM: %v", err)
	}
	return buf.Bytes()
}

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

func makeSyntheticCase(hashHex string, byteLen int64, seq int64, expectations []domain.AssertionExpectation) *domain.Case {
	proto := 21
	retrieved := time.Date(2026, 11, 27, 7, 0, 0, 0, time.UTC)
	return &domain.Case{
		SchemaVersion:  "1.0.0",
		CaseID:         "case_synthetic_unit7",
		Title:          "Synthetic Unit 7 Evidence Fixture",
		Summary:        "Strictly synthetic test fixture for Unit 7 evidence bundle tests.",
		Classification: domain.ClassificationSyntheticFixture,
		Source: domain.Source{
			Kind:        "synthetic_construction",
			RetrievedAt: &retrieved,
			Notes:       "Synthetic test fixture",
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
		Expectations: expectations,
		Limitations:  []string{"synthetic test fixture"},
	}
}

func runRealGoIngest(t *testing.T, runID, caseID string, vi *localfile.VerifiedInput) *adapter.RunOutput {
	t.Helper()
	reg, err := adapter.NewProductionRegistry(adapter.ProductionRegistryConfig{
		GoIngestBinaryPath: testGoIngestBinary,
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	out, err := runner.Run(ctx, adapter.RunParams{
		RunID:       runID,
		CaseID:      caseID,
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
	return out
}

func makeCompletedBuildInput(t *testing.T, runID string, expectedSeq int64, startedAt, completedAt time.Time) evidence.BuildInput {
	t.Helper()
	xdrBytes := makeSyntheticLedgerXDR(t, 100, 21)
	vi, hashHex, byteLen := makeSyntheticVerifiedInput(t, xdrBytes)
	expectations := []domain.AssertionExpectation{
		{AssertionID: "assertion_seq", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: json.Number(fmt.Sprintf("%d", expectedSeq))},
		{AssertionID: "assertion_sha", Kind: domain.AssertionKindInputHashEquals, Expected: hashHex},
	}
	c := makeSyntheticCase(hashHex, byteLen, 100, expectations)
	out := runRealGoIngest(t, runID, c.CaseID, vi)

	var gp goingest.SemanticProjection
	if err := json.Unmarshal(*out.Result.SemanticProjection, &gp); err != nil {
		t.Fatalf("unmarshal SemanticProjection: %v", err)
	}
	dp, err := assertion.ProjectionFromGoIngest(&gp)
	if err != nil {
		t.Fatalf("ProjectionFromGoIngest: %v", err)
	}
	eval, err := assertion.Evaluate(expectations, assertion.EvaluationInput{
		Projection:          dp,
		VerifiedInputSHA256: hashHex,
		SupportedKinds:      assertion.GoIngestSupportedKinds(),
		EvidenceRefs:        []string{"logical://input/sha256/" + hashHex},
	})
	if err != nil {
		t.Fatalf("assertion.Evaluate: %v", err)
	}

	outcome := eval.Outcome
	return evidence.BuildInput{
		Case: c,
		Run: &domain.Run{
			RunID:       runID,
			CaseID:      c.CaseID,
			AdapterID:   adapter.AdapterIDGoIngest,
			State:       domain.RunStateCompleted,
			Outcome:     &outcome,
			StartedAt:   startedAt,
			CompletedAt: &completedAt,
		},
		VerifiedInput: vi,
		AdapterTarget: adapter.RequestTarget{
			AdapterID:      adapter.AdapterIDGoIngest,
			AdapterVersion: adapter.DefaultGoAdapterVersion,
			SDKVersion:     adapter.PinnedGoSDKVersion,
		},
		AdapterOutput: out,
		Assertions:    eval.Results,
		Worker:        evidence.WorkerMetadata{Version: "0.1.0-test", CommitSHA: nil},
	}
}

func TestWriteAndVerifyBundle_CompletedPassAndRegression(t *testing.T) {
	evRoot := t.TempDir()
	start := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Second)

	t.Run("completed_pass_with_real_stdout", func(t *testing.T) {
		in := makeCompletedBuildInput(t, "run_unit7_pass", 100, start, end)
		wr, err := evidence.WriteBundle(evRoot, in)
		if err != nil {
			t.Fatalf("WriteBundle failed: %v", err)
		}
		if wr.Verification.ArtifactCount != 3 {
			t.Errorf("ArtifactCount = %d, want 3 (adapter-stdout, semantic-projection, assertions)", wr.Verification.ArtifactCount)
		}
		if wr.Manifest.Run.State != domain.RunStateCompleted || *wr.Manifest.Run.Outcome != domain.RunOutcomePass {
			t.Errorf("unexpected run state/outcome: %s / %v", wr.Manifest.Run.State, wr.Manifest.Run.Outcome)
		}
		if wr.Manifest.Run.HostExitCode == nil || *wr.Manifest.Run.HostExitCode != 0 {
			t.Errorf("expected HostExitCode = 0, got %v", wr.Manifest.Run.HostExitCode)
		}
		if wr.Manifest.Run.HostDurationMs == nil || wr.Manifest.Run.AdapterDurationMs == nil {
			t.Error("expected both HostDurationMs and AdapterDurationMs to be populated")
		}

		storedStdout, err := os.ReadFile(filepath.Join(wr.BundleDir, "artifacts", "adapter-stdout.json"))
		if err != nil {
			t.Fatalf("read adapter-stdout.json: %v", err)
		}
		if !bytes.Equal(storedStdout, in.AdapterOutput.StdoutCopy()) {
			t.Error("stored adapter-stdout.json does not match ProcessRunner StdoutCopy() byte-for-byte")
		}

		// Independent read-only verification must not modify stored files.
		if _, err := evidence.VerifyBundle(evRoot, "run_unit7_pass"); err != nil {
			t.Fatalf("VerifyBundle failed: %v", err)
		}
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrBundleExists) {
			t.Errorf("expected ErrBundleExists on duplicate WriteBundle, got %v", err)
		}
	})

	t.Run("completed_regression_preserved", func(t *testing.T) {
		in := makeCompletedBuildInput(t, "run_unit7_regression", 999, start, end)
		if *in.Run.Outcome != domain.RunOutcomeRegression {
			t.Fatalf("expected regression outcome from evaluator, got %s", *in.Run.Outcome)
		}
		wr, err := evidence.WriteBundle(evRoot, in)
		if err != nil {
			t.Fatalf("WriteBundle failed: %v", err)
		}
		if wr.Manifest.Run.State != domain.RunStateCompleted || *wr.Manifest.Run.Outcome != domain.RunOutcomeRegression {
			t.Errorf("unexpected state/outcome: %s / %v", wr.Manifest.Run.State, wr.Manifest.Run.Outcome)
		}
	})

	t.Run("exact_uint32_4294967295_evaluated_and_verified", func(t *testing.T) {
		xdrBytes := makeSyntheticLedgerXDR(t, 4294967295, 21)
		vi, hashHex, byteLen := makeSyntheticVerifiedInput(t, xdrBytes)
		expectations := []domain.AssertionExpectation{
			{AssertionID: "assertion_seq_max", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: json.Number("4294967295")},
			{AssertionID: "assertion_sha_max", Kind: domain.AssertionKindInputHashEquals, Expected: hashHex},
		}
		c := makeSyntheticCase(hashHex, byteLen, 4294967295, expectations)
		out := runRealGoIngest(t, "run_unit7_max_u32", c.CaseID, vi)

		var gp goingest.SemanticProjection
		if err := json.Unmarshal(*out.Result.SemanticProjection, &gp); err != nil {
			t.Fatalf("unmarshal SemanticProjection: %v", err)
		}
		dp, err := assertion.ProjectionFromGoIngest(&gp)
		if err != nil {
			t.Fatalf("ProjectionFromGoIngest: %v", err)
		}
		eval, err := assertion.Evaluate(expectations, assertion.EvaluationInput{
			Projection:          dp,
			VerifiedInputSHA256: hashHex,
			SupportedKinds:      assertion.GoIngestSupportedKinds(),
		})
		if err != nil {
			t.Fatalf("assertion.Evaluate: %v", err)
		}
		outcome := eval.Outcome
		wr, err := evidence.WriteBundle(evRoot, evidence.BuildInput{
			Case:          c,
			Run:           &domain.Run{RunID: "run_unit7_max_u32", CaseID: c.CaseID, AdapterID: adapter.AdapterIDGoIngest, State: domain.RunStateCompleted, Outcome: &outcome, StartedAt: start, CompletedAt: &end},
			VerifiedInput: vi,
			AdapterTarget: adapter.RequestTarget{AdapterID: adapter.AdapterIDGoIngest, AdapterVersion: adapter.DefaultGoAdapterVersion, SDKVersion: adapter.PinnedGoSDKVersion},
			AdapterOutput: out,
			Assertions:    eval.Results,
			Worker:        evidence.WorkerMetadata{Version: "0.1.0-test", CommitSHA: nil},
		})
		if err != nil {
			t.Fatalf("WriteBundle: %v", err)
		}
		if *wr.Manifest.Run.Outcome != domain.RunOutcomePass {
			t.Errorf("Outcome = %s, want pass", *wr.Manifest.Run.Outcome)
		}
	})
}

func buildCustomTestAdapterBinary(t *testing.T, sleepMs int, reportedDurationMs int64, nullProjection bool) string {
	t.Helper()
	srcDir := t.TempDir()
	src := fmt.Sprintf(`package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	reqBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(4)
	}
	var req struct {
		RunID       string `+"`json:\"run_id\"`"+`
		CaseID      string `+"`json:\"case_id\"`"+`
		InputSHA256 string `+"`json:\"input_sha256\"`"+`
		Target      struct {
			AdapterID      string `+"`json:\"adapter_id\"`"+`
			AdapterVersion string `+"`json:\"adapter_version\"`"+`
			SDKVersion     string `+"`json:\"sdk_version\"`"+`
		} `+"`json:\"target\"`"+`
	}
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		os.Exit(4)
	}
	time.Sleep(%d * time.Millisecond)

	var proj any = nil
	if !%t {
		proj = map[string]any{
			"projection_version": "1",
			"xdr_type":           "LedgerCloseMeta",
			"outer_variant":      1,
			"ledger": map[string]any{
				"sequence":             100,
				"protocol_version":     21,
				"close_time_unix":      1700000100,
				"ledger_hash":          "1100000000000000000000000000000000000000000000000000000000000000",
				"previous_ledger_hash": "0000000000000000000000000000000000000000000000000000000000000000",
			},
			"transactions": map[string]any{
				"total":            0,
				"successful":       0,
				"failed":           0,
				"hashes":           []string{},
				"hashes_truncated": false,
			},
		}
	}

	res := map[string]any{
		"schema_version":  "1",
		"run_id":          req.RunID,
		"case_id":         req.CaseID,
		"adapter_id":      req.Target.AdapterID,
		"adapter_version": req.Target.AdapterVersion,
		"sdk_version":     req.Target.SDKVersion,
		"execution": map[string]any{
			"status":      "completed",
			"exit_code":   0,
			"duration_ms": %d,
		},
		"input": map[string]any{
			"sha256": req.InputSHA256,
		},
		"decode": map[string]any{
			"status":        "success",
			"detected_type": "LedgerCloseMeta",
		},
		"semantic_projection": proj,
		"diagnostics":         []any{},
		"capabilities_used":   []string{"decode_ledger_meta"},
	}
	b, _ := json.Marshal(res)
	fmt.Printf("%%s\n", b)
}
`, sleepMs, nullProjection, reportedDurationMs)

	mainPath := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(mainPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write test adapter source: %v", err)
	}
	binName := "test-custom-adapter"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(srcDir, binName)
	cmd := exec.Command("go", "build", "-o", binPath, mainPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build custom test adapter: %v\n%s", err, string(out))
	}
	return binPath
}

func runCustomAdapter(t *testing.T, binPath, runID, caseID string, vi *localfile.VerifiedInput) *adapter.RunOutput {
	t.Helper()
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := runner.Run(ctx, adapter.RunParams{
		RunID:       runID,
		CaseID:      caseID,
		Operation:   adapter.OperationDecodeLedgerMeta,
		InputFormat: adapter.InputFormatLedgerCloseMeta,
		Target:      adapter.RequestTarget{AdapterID: adapter.AdapterIDGoIngest, AdapterVersion: adapter.DefaultGoAdapterVersion, SDKVersion: adapter.PinnedGoSDKVersion},
		Limits:      adapter.RequestLimits{TimeoutMs: 10000, MaxInputBytes: 1024 * 1024, MaxOutputBytes: 1024 * 1024},
	}, vi)
	if err != nil {
		t.Fatalf("runner.Run: %v", err)
	}
	return out
}

func TestWriteBundle_GenuineNullProjectionAndDuration(t *testing.T) {
	evRoot := t.TempDir()
	start := time.Date(2026, 11, 27, 8, 8, 0, 0, time.UTC)
	end := start.Add(time.Second)

	t.Run("genuine_null_projection_input_hash_passes_and_seq_inconclusive", func(t *testing.T) {
		bin := buildCustomTestAdapterBinary(t, 0, 5, true)
		vi, hashHex, byteLen := makeSyntheticVerifiedInput(t, []byte("synthetic-null-proj"))
		expectations := []domain.AssertionExpectation{
			{AssertionID: "assertion_sha", Kind: domain.AssertionKindInputHashEquals, Expected: hashHex},
		}
		c := makeSyntheticCase(hashHex, byteLen, 100, expectations)
		out := runCustomAdapter(t, bin, "run_null_proj_pass", c.CaseID, vi)

		eval, err := assertion.Evaluate(expectations, assertion.EvaluationInput{
			Projection:          nil,
			VerifiedInputSHA256: hashHex,
			SupportedKinds:      assertion.GoIngestSupportedKinds(),
		})
		if err != nil {
			t.Fatalf("assertion.Evaluate: %v", err)
		}
		outcome := eval.Outcome
		wr, err := evidence.WriteBundle(evRoot, evidence.BuildInput{
			Case:          c,
			Run:           &domain.Run{RunID: "run_null_proj_pass", CaseID: c.CaseID, AdapterID: adapter.AdapterIDGoIngest, State: domain.RunStateCompleted, Outcome: &outcome, StartedAt: start, CompletedAt: &end},
			VerifiedInput: vi,
			AdapterTarget: adapter.RequestTarget{AdapterID: adapter.AdapterIDGoIngest, AdapterVersion: adapter.DefaultGoAdapterVersion, SDKVersion: adapter.PinnedGoSDKVersion},
			AdapterOutput: out,
			Assertions:    eval.Results,
			Worker:        evidence.WorkerMetadata{Version: "0.1.0-test", CommitSHA: nil},
		})
		if err != nil {
			t.Fatalf("WriteBundle: %v", err)
		}
		if wr.Verification.ArtifactCount != 2 {
			t.Errorf("ArtifactCount = %d, want 2 (adapter-stdout + assertions)", wr.Verification.ArtifactCount)
		}

		// Projection-dependent assertion against genuine null projection yields inconclusive.
		exp2 := []domain.AssertionExpectation{
			{AssertionID: "assertion_seq", Kind: domain.AssertionKindLedgerSequenceEquals, Expected: json.Number("100")},
			{AssertionID: "assertion_sha", Kind: domain.AssertionKindInputHashEquals, Expected: hashHex},
		}
		c2 := makeSyntheticCase(hashHex, byteLen, 100, exp2)
		out2 := runCustomAdapter(t, bin, "run_null_proj_inc", c2.CaseID, vi)
		eval2, err := assertion.Evaluate(exp2, assertion.EvaluationInput{
			Projection:          nil,
			VerifiedInputSHA256: hashHex,
			SupportedKinds:      assertion.GoIngestSupportedKinds(),
		})
		if err != nil {
			t.Fatalf("assertion.Evaluate 2: %v", err)
		}
		if eval2.Outcome != domain.RunOutcomeInconclusive {
			t.Fatalf("expected inconclusive outcome, got %s", eval2.Outcome)
		}
		out2Outcome := eval2.Outcome
		if _, err := evidence.WriteBundle(evRoot, evidence.BuildInput{
			Case:          c2,
			Run:           &domain.Run{RunID: "run_null_proj_inc", CaseID: c2.CaseID, AdapterID: adapter.AdapterIDGoIngest, State: domain.RunStateCompleted, Outcome: &out2Outcome, StartedAt: start, CompletedAt: &end},
			VerifiedInput: vi,
			AdapterTarget: adapter.RequestTarget{AdapterID: adapter.AdapterIDGoIngest, AdapterVersion: adapter.DefaultGoAdapterVersion, SDKVersion: adapter.PinnedGoSDKVersion},
			AdapterOutput: out2,
			Assertions:    eval2.Results,
			Worker:        evidence.WorkerMetadata{Version: "0.1.0-test", CommitSHA: nil},
		}); err != nil {
			t.Fatalf("WriteBundle inconclusive: %v", err)
		}
	})

	t.Run("authentic_host_vs_adapter_duration", func(t *testing.T) {
		// Adapter sleeps 35ms and self-reports duration_ms = 9999 in its actual stdout.
		bin := buildCustomTestAdapterBinary(t, 35, 9999, false)
		vi, hashHex, byteLen := makeSyntheticVerifiedInput(t, []byte("synthetic-dur"))
		expectations := []domain.AssertionExpectation{
			{AssertionID: "assertion_sha", Kind: domain.AssertionKindInputHashEquals, Expected: hashHex},
		}
		c := makeSyntheticCase(hashHex, byteLen, 100, expectations)
		out := runCustomAdapter(t, bin, "run_auth_dur", c.CaseID, vi)

		var gp goingest.SemanticProjection
		_ = json.Unmarshal(*out.Result.SemanticProjection, &gp)
		dp, _ := assertion.ProjectionFromGoIngest(&gp)
		eval, err := assertion.Evaluate(expectations, assertion.EvaluationInput{
			Projection:          dp,
			VerifiedInputSHA256: hashHex,
			SupportedKinds:      assertion.GoIngestSupportedKinds(),
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		outcome := eval.Outcome
		wr, err := evidence.WriteBundle(evRoot, evidence.BuildInput{
			Case:          c,
			Run:           &domain.Run{RunID: "run_auth_dur", CaseID: c.CaseID, AdapterID: adapter.AdapterIDGoIngest, State: domain.RunStateCompleted, Outcome: &outcome, StartedAt: start, CompletedAt: &end},
			VerifiedInput: vi,
			AdapterTarget: adapter.RequestTarget{AdapterID: adapter.AdapterIDGoIngest, AdapterVersion: adapter.DefaultGoAdapterVersion, SDKVersion: adapter.PinnedGoSDKVersion},
			AdapterOutput: out,
			Assertions:    eval.Results,
			Worker:        evidence.WorkerMetadata{Version: "0.1.0-test", CommitSHA: nil},
		})
		if err != nil {
			t.Fatalf("WriteBundle: %v", err)
		}
		if wr.Manifest.Run.AdapterDurationMs == nil || *wr.Manifest.Run.AdapterDurationMs != 9999 {
			t.Errorf("AdapterDurationMs = %v, want 9999", wr.Manifest.Run.AdapterDurationMs)
		}
		if wr.Manifest.Run.HostDurationMs == nil || *wr.Manifest.Run.HostDurationMs < 30 || *wr.Manifest.Run.HostDurationMs == 9999 {
			t.Errorf("HostDurationMs = %v, expected >= 30 and != 9999", wr.Manifest.Run.HostDurationMs)
		}
	})
}

func TestWriteBundle_CompletedRunConsistencyRejections(t *testing.T) {
	evRoot := t.TempDir()
	start := time.Date(2026, 11, 27, 8, 5, 0, 0, time.UTC)
	end := start.Add(time.Second)

	t.Run("pass_despite_failing_assertion_rejected", func(t *testing.T) {
		in := makeCompletedBuildInput(t, "run_bad_pass", 999, start, end)
		pass := domain.RunOutcomePass
		in.Run.Outcome = &pass
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput, got %v", err)
		}
	})

	t.Run("regression_when_all_pass_rejected", func(t *testing.T) {
		in := makeCompletedBuildInput(t, "run_bad_reg", 100, start, end)
		reg := domain.RunOutcomeRegression
		in.Run.Outcome = &reg
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput, got %v", err)
		}
	})

	t.Run("missing_assertions_rejected", func(t *testing.T) {
		in := makeCompletedBuildInput(t, "run_no_assertions", 100, start, end)
		in.Assertions = nil
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput, got %v", err)
		}
	})

	t.Run("reordered_assertion_ids_rejected", func(t *testing.T) {
		in := makeCompletedBuildInput(t, "run_reordered", 100, start, end)
		in.Assertions[0], in.Assertions[1] = in.Assertions[1], in.Assertions[0]
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput, got %v", err)
		}
	})

	t.Run("duplicate_assertion_ids_rejected", func(t *testing.T) {
		in := makeCompletedBuildInput(t, "run_dup_id", 100, start, end)
		in.Assertions[1].AssertionID = in.Assertions[0].AssertionID
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput, got %v", err)
		}
	})

	t.Run("kind_mismatch_rejected", func(t *testing.T) {
		in := makeCompletedBuildInput(t, "run_kind_mismatch", 100, start, end)
		in.Assertions[0].Kind = domain.AssertionKindTransactionCountEquals
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput, got %v", err)
		}
	})

	t.Run("expected_value_mismatch_rejected", func(t *testing.T) {
		in := makeCompletedBuildInput(t, "run_exp_mismatch", 100, start, end)
		in.Assertions[0].Expected = int64(101)
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput, got %v", err)
		}
	})

	t.Run("mutating_result_after_process_runner_rejected", func(t *testing.T) {
		in1 := makeCompletedBuildInput(t, "run_mut_proj", 100, start, end)
		tamperedProj := json.RawMessage(`{"tampered":true}`)
		in1.AdapterOutput.Result.SemanticProjection = &tamperedProj
		if _, err := evidence.WriteBundle(evRoot, in1); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput when Result.SemanticProjection mutated, got %v", err)
		}

		in2 := makeCompletedBuildInput(t, "run_mut_dur", 100, start, end)
		in2.AdapterOutput.Result.Execution.DurationMs = 99999
		if _, err := evidence.WriteBundle(evRoot, in2); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput when Result.Execution.DurationMs mutated, got %v", err)
		}

		in3 := makeCompletedBuildInput(t, "run_mut_diag", 100, start, end)
		in3.AdapterOutput.Result.Diagnostics = []adapter.ResultDiagnostic{{Stage: "decode", Code: "tamper", Message: "tampered"}}
		if _, err := evidence.WriteBundle(evRoot, in3); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput when Result.Diagnostics mutated, got %v", err)
		}
	})

	t.Run("fabricated_observed_or_status_rejected_by_recalculation", func(t *testing.T) {
		// Pass status with incorrect observed sequence
		in1 := makeCompletedBuildInput(t, "run_fab_obs", 100, start, end)
		in1.Assertions[0].Observed = int64(999)
		if _, err := evidence.WriteBundle(evRoot, in1); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput for fabricated observed sequence, got %v", err)
		}

		// Pass status with omitted observed value
		in2 := makeCompletedBuildInput(t, "run_omit_obs", 100, start, end)
		in2.Assertions[0].Observed = nil
		if _, err := evidence.WriteBundle(evRoot, in2); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput for omitted observed value, got %v", err)
		}

		// Inconsistent evidence_refs across results
		in3 := makeCompletedBuildInput(t, "run_bad_refs", 100, start, end)
		in3.Assertions[1].EvidenceRefs = []string{"logical://other/ref"}
		if _, err := evidence.WriteBundle(evRoot, in3); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput for inconsistent evidence_refs, got %v", err)
		}

		// Unknown adapter ID fails closed
		in4 := makeCompletedBuildInput(t, "run_unknown_adapter", 100, start, end)
		in4.Run.AdapterID = "unknown-adapter"
		in4.AdapterTarget.AdapterID = "unknown-adapter"
		if _, err := evidence.WriteBundle(evRoot, in4); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput for unknown adapter ID, got %v", err)
		}
	})

	t.Run("missing_adapter_output_rejected_for_completed_run", func(t *testing.T) {
		in := makeCompletedBuildInput(t, "run_no_adapter_out", 100, start, end)
		in.AdapterOutput = nil
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput, got %v", err)
		}
	})

	t.Run("empty_stdout_rejected_for_completed_run", func(t *testing.T) {
		in := makeCompletedBuildInput(t, "run_empty_stdout", 100, start, end)
		// Construct a RunOutput with Result but empty rawStdout.
		in.AdapterOutput = &adapter.RunOutput{
			Result:         in.AdapterOutput.Result,
			ActualExitCode: 0,
			ActualDuration: 5 * time.Millisecond,
		}
		if _, err := evidence.WriteBundle(evRoot, in); !errors.Is(err, evidence.ErrInvalidBundleInput) {
			t.Errorf("expected ErrInvalidBundleInput for empty StdoutCopy(), got %v", err)
		}
	})
}

func TestWriteBundle_FailedAndCancelledRuns(t *testing.T) {
	evRoot := t.TempDir()
	start := time.Date(2026, 11, 27, 8, 12, 0, 0, time.UTC)
	end := start.Add(time.Second)

	t.Run("failed_run_without_adapter_output_succeeds", func(t *testing.T) {
		vi, hashHex, byteLen := makeSyntheticVerifiedInput(t, []byte("synthetic-bad"))
		c := makeSyntheticCase(hashHex, byteLen, 100, nil)
		wr, err := evidence.WriteBundle(evRoot, evidence.BuildInput{
			Case:          c,
			Run:           &domain.Run{RunID: "run_failed_pre_adapter", CaseID: c.CaseID, AdapterID: adapter.AdapterIDGoIngest, State: domain.RunStateFailed, StartedAt: start, CompletedAt: &end},
			VerifiedInput: vi,
			AdapterTarget: adapter.RequestTarget{AdapterID: adapter.AdapterIDGoIngest, AdapterVersion: adapter.DefaultGoAdapterVersion, SDKVersion: adapter.PinnedGoSDKVersion},
			Worker:        evidence.WorkerMetadata{Version: "0.1.0-test", CommitSHA: nil},
		})
		if err != nil {
			t.Fatalf("WriteBundle failed: %v", err)
		}
		if wr.Manifest.Run.Outcome != nil || wr.Manifest.Run.HostDurationMs != nil || wr.Manifest.Run.HostExitCode != nil {
			t.Error("expected nil outcome, host_duration_ms, and host_exit_code for pre-adapter failed run")
		}
	})

	t.Run("partial_write_cleanup_confined_to_run_dir", func(t *testing.T) {
		in := makeCompletedBuildInput(t, "run_partial_cleanup", 100, start, end)
		badProj := json.RawMessage(`not-json`)
		in.AdapterOutput.Result.SemanticProjection = &badProj
		if _, err := evidence.WriteBundle(evRoot, in); err == nil {
			t.Fatal("expected error for invalid semantic_projection JSON")
		}
		if _, err := os.Stat(filepath.Join(evRoot, "run_partial_cleanup")); !errors.Is(err, os.ErrNotExist) {
			t.Error("expected partial run directory to be cleaned up")
		}
	})
}

func TestVerifyBundle_DetectsTamperingAndManifestViolations(t *testing.T) {
	setupBundle := func(t *testing.T, runID string) string {
		t.Helper()
		evRoot := t.TempDir()
		start := time.Date(2026, 11, 27, 8, 15, 0, 0, time.UTC)
		in := makeCompletedBuildInput(t, runID, 100, start, start.Add(time.Second))
		if _, err := evidence.WriteBundle(evRoot, in); err != nil {
			t.Fatalf("WriteBundle: %v", err)
		}
		return evRoot
	}

	t.Run("modified_artifact_detected", func(t *testing.T) {
		evRoot := setupBundle(t, "run_tamper_mod")
		artPath := filepath.Join(evRoot, "run_tamper_mod", "artifacts", "semantic-projection.json")
		_ = os.Chmod(artPath, 0o600)
		orig, _ := os.ReadFile(artPath)
		orig[1] = 'X'
		_ = os.WriteFile(artPath, orig, 0o400)
		if _, err := evidence.VerifyBundle(evRoot, "run_tamper_mod"); !errors.Is(err, evidence.ErrHashMismatch) {
			t.Errorf("expected ErrHashMismatch, got %v", err)
		}
	})

	t.Run("byte_length_mismatch_detected", func(t *testing.T) {
		evRoot := setupBundle(t, "run_tamper_len")
		artPath := filepath.Join(evRoot, "run_tamper_len", "artifacts", "semantic-projection.json")
		_ = os.Chmod(artPath, 0o600)
		_ = os.WriteFile(artPath, []byte(`{"x":1}`), 0o400)
		if _, err := evidence.VerifyBundle(evRoot, "run_tamper_len"); !errors.Is(err, evidence.ErrByteLengthMismatch) {
			t.Errorf("expected ErrByteLengthMismatch, got %v", err)
		}
	})

	t.Run("missing_artifact_detected", func(t *testing.T) {
		evRoot := setupBundle(t, "run_tamper_missing")
		_ = os.Remove(filepath.Join(evRoot, "run_tamper_missing", "artifacts", "assertions.json"))
		if _, err := evidence.VerifyBundle(evRoot, "run_tamper_missing"); !errors.Is(err, evidence.ErrArtifactMissing) {
			t.Errorf("expected ErrArtifactMissing, got %v", err)
		}
	})

	t.Run("extra_unlisted_file_and_dir_detected", func(t *testing.T) {
		evRoot := setupBundle(t, "run_tamper_extra")
		_ = os.WriteFile(filepath.Join(evRoot, "run_tamper_extra", "artifacts", "rogue.json"), []byte(`{}`), 0o400)
		if _, err := evidence.VerifyBundle(evRoot, "run_tamper_extra"); !errors.Is(err, evidence.ErrExtraArtifact) {
			t.Errorf("expected ErrExtraArtifact for extra file, got %v", err)
		}

		evRoot2 := setupBundle(t, "run_tamper_subdir")
		_ = os.Mkdir(filepath.Join(evRoot2, "run_tamper_subdir", "artifacts", "nested"), 0o700)
		if _, err := evidence.VerifyBundle(evRoot2, "run_tamper_subdir"); !errors.Is(err, evidence.ErrExtraArtifact) {
			t.Errorf("expected ErrExtraArtifact for unexpected subdir, got %v", err)
		}
	})

	t.Run("bundle_dir_and_artifact_symlinks_rejected", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires elevated privileges on Windows")
		}
		evRoot := setupBundle(t, "run_sym_target")
		if err := os.Symlink(filepath.Join(evRoot, "run_sym_target"), filepath.Join(evRoot, "run_sym_link")); err != nil {
			t.Fatalf("symlink bundle dir: %v", err)
		}
		if _, err := evidence.VerifyBundle(evRoot, "run_sym_link"); !errors.Is(err, evidence.ErrSymlinkRejected) {
			t.Errorf("expected ErrSymlinkRejected for bundle symlink, got %v", err)
		}

		evRoot2 := setupBundle(t, "run_art_sym")
		artPath := filepath.Join(evRoot2, "run_art_sym", "artifacts", "assertions.json")
		target := filepath.Join(evRoot2, "outside.json")
		_ = os.WriteFile(target, []byte(`[]`), 0o600)
		_ = os.Remove(artPath)
		_ = os.Symlink(target, artPath)
		if _, err := evidence.VerifyBundle(evRoot2, "run_art_sym"); !errors.Is(err, evidence.ErrSymlinkRejected) {
			t.Errorf("expected ErrSymlinkRejected for artifact symlink, got %v", err)
		}
	})

	t.Run("oversized_manifest_rejected", func(t *testing.T) {
		evRoot := setupBundle(t, "run_big_manifest")
		mPath := filepath.Join(evRoot, "run_big_manifest", "manifest.json")
		_ = os.Chmod(mPath, 0o600)
		big := bytes.Repeat([]byte(" "), int(evidence.MaxManifestBytes)+10)
		_ = os.WriteFile(mPath, big, 0o400)
		if _, err := evidence.VerifyBundle(evRoot, "run_big_manifest"); !errors.Is(err, evidence.ErrManifestInvalid) {
			t.Errorf("expected ErrManifestInvalid for oversized manifest, got %v", err)
		}
	})

	t.Run("assertions_artifact_disagrees_with_manifest_rejected", func(t *testing.T) {
		evRoot := setupBundle(t, "run_disagree")
		mPath := filepath.Join(evRoot, "run_disagree", "manifest.json")
		artPath := filepath.Join(evRoot, "run_disagree", "artifacts", "assertions.json")
		// Modify assertions.json explanation AND update its hash/len in manifest.json so hash check passes
		// but verifyAssertionsConsistency catches the semantic disagreement!
		artBytes, _ := os.ReadFile(artPath)
		tamperedArt := bytes.Replace(artBytes, []byte("matches expected 100"), []byte("TAMPERED EXPLANATION"), 1)
		_ = os.Chmod(artPath, 0o600)
		_ = os.WriteFile(artPath, tamperedArt, 0o400)

		newSHA, newLen := evidence.ComputeSHA256(tamperedArt)
		mBytes, _ := os.ReadFile(mPath)
		var m evidence.Manifest
		_ = json.Unmarshal(mBytes, &m)
		for i := range m.Artifacts {
			if m.Artifacts[i].Role == evidence.ArtifactRoleAssertions {
				m.Artifacts[i].SHA256 = newSHA
				m.Artifacts[i].ByteLength = newLen
			}
		}
		updatedM, _ := json.MarshalIndent(&m, "", "  ")
		_ = os.Chmod(mPath, 0o600)
		_ = os.WriteFile(mPath, updatedM, 0o400)

		if _, err := evidence.VerifyBundle(evRoot, "run_disagree"); !errors.Is(err, evidence.ErrManifestInvalid) {
			t.Errorf("expected ErrManifestInvalid when assertions.json disagrees with manifest.assertions, got %v", err)
		}
	})
}

func TestManifestDecodeValidation_TimestampsAndConsistency(t *testing.T) {
	evRoot := t.TempDir()
	start := time.Date(2026, 11, 27, 8, 20, 0, 0, time.UTC)
	wr, err := evidence.WriteBundle(evRoot, makeCompletedBuildInput(t, "run_ts_check", 100, start, start.Add(time.Second)))
	if err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}
	validBytes, _ := os.ReadFile(wr.ManifestPath)

	tests := []struct {
		name    string
		mutate  func(b []byte) []byte
		wantErr error
	}{
		{
			name:   "valid_utc_timestamps",
			mutate: func(b []byte) []byte { return b },
		},
		{
			name: "invalid_retrieved_at",
			mutate: func(b []byte) []byte {
				return bytes.Replace(b, []byte(`"2026-11-27T07:00:00Z"`), []byte(`"not-a-timestamp"`), 1)
			},
			wantErr: evidence.ErrManifestInvalid,
		},
		{
			name: "non_utc_retrieved_at",
			mutate: func(b []byte) []byte {
				return bytes.Replace(b, []byte(`"2026-11-27T07:00:00Z"`), []byte(`"2026-11-27T09:00:00+02:00"`), 1)
			},
			wantErr: evidence.ErrManifestInvalid,
		},
		{
			name: "invalid_started_at",
			mutate: func(b []byte) []byte {
				return bytes.Replace(b, []byte(`"2026-11-27T08:20:00Z"`), []byte(`"bad-start"`), 1)
			},
			wantErr: evidence.ErrManifestInvalid,
		},
		{
			name: "invalid_completed_at",
			mutate: func(b []byte) []byte {
				return bytes.Replace(b, []byte(`"2026-11-27T08:20:01Z"`), []byte(`"bad-end"`), 1)
			},
			wantErr: evidence.ErrManifestInvalid,
		},
		{
			name: "completed_before_started",
			mutate: func(b []byte) []byte {
				return bytes.Replace(b, []byte(`"2026-11-27T08:20:01Z"`), []byte(`"2026-11-27T08:19:59Z"`), 1)
			},
			wantErr: evidence.ErrManifestInvalid,
		},
		{
			name: "unsupported_major_version",
			mutate: func(b []byte) []byte {
				return bytes.Replace(b, []byte(`"1.0.0"`), []byte(`"2.0.0"`), 1)
			},
			wantErr: evidence.ErrUnsupportedSchemaVersion,
		},
		{
			name: "trailing_json",
			mutate: func(b []byte) []byte {
				return append(append([]byte(nil), b...), []byte(`{"extra":1}`)...)
			},
			wantErr: evidence.ErrManifestInvalid,
		},
		{
			name: "unknown_field",
			mutate: func(b []byte) []byte {
				return bytes.Replace(b, []byte(`"schema_version": "1.0.0",`), []byte(`"schema_version": "1.0.0", "unknown": 1,`), 1)
			},
			wantErr: evidence.ErrManifestInvalid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := evidence.DecodeAndValidateManifest(tc.mutate(validBytes))
			if tc.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
		})
	}

	t.Run("completed_manifest_missing_assertions_or_roles_rejected", func(t *testing.T) {
		var m evidence.Manifest
		_ = json.Unmarshal(validBytes, &m)
		m.Assertions = []domain.AssertionResult{}
		b, _ := json.Marshal(&m)
		if _, err := evidence.DecodeAndValidateManifest(b); !errors.Is(err, evidence.ErrManifestInvalid) {
			t.Errorf("expected ErrManifestInvalid for completed manifest with empty assertions, got %v", err)
		}
	})
}

func TestDeterminism_TimestampsDoNotAffectSemanticArtifacts(t *testing.T) {
	evRoot := t.TempDir()
	t1 := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 27, 12, 30, 0, 0, time.UTC)

	wr1, err := evidence.WriteBundle(evRoot, makeCompletedBuildInput(t, "run_det_1", 100, t1, t1.Add(time.Second)))
	if err != nil {
		t.Fatalf("WriteBundle 1: %v", err)
	}
	wr2, err := evidence.WriteBundle(evRoot, makeCompletedBuildInput(t, "run_det_2", 100, t2, t2.Add(5*time.Second)))
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
