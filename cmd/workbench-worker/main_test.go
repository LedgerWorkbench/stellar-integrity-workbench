package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/evidence"
)

var (
	testBinDir      string
	testWorkerBin   string
	testGoIngestBin string
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "siw-worker-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	testBinDir = tmpDir

	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	testGoIngestBin = filepath.Join(tmpDir, "go-ingest-adapter"+suffix)
	testWorkerBin = filepath.Join(tmpDir, "workbench-worker"+suffix)

	buildAdapter := exec.Command("go", "build", "-o", testGoIngestBin,
		"github.com/LedgerWorkbench/stellar-integrity-workbench/cmd/go-ingest-adapter")
	if out, err := buildAdapter.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build go-ingest-adapter: %v\n%s\n", err, string(out))
		_ = os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	buildWorker := exec.Command("go", "build", "-o", testWorkerBin, ".")
	if out, err := buildWorker.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build workbench-worker: %v\n%s\n", err, string(out))
		_ = os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	exitCode := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(exitCode)
}

func makeSyntheticLedgerXDR(t *testing.T, seq uint32, protocol uint32) []byte {
	t.Helper()
	var h, txHash xdr.Hash
	h[0] = 0x11
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
							Tx: xdr.Transaction{SourceAccount: src, Fee: 100, SeqNum: 1},
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

func makeSyntheticCorpusAndManifest(t *testing.T, payload []byte, expectedSeq uint32) (manifestPath, inputRoot, relObjPath, outDir, shaHex string) {
	t.Helper()
	base := t.TempDir()
	inputRoot = filepath.Join(base, "corpus")
	relObjPath = "synthetic/ledger.xdr"
	objFull := filepath.Join(inputRoot, relObjPath)
	if err := os.MkdirAll(filepath.Dir(objFull), 0o755); err != nil {
		t.Fatalf("mkdir corpus: %v", err)
	}
	if err := os.WriteFile(objFull, payload, 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	sum := sha256.Sum256(payload)
	shaHex = hex.EncodeToString(sum[:])

	manifest := map[string]any{
		"schema_version": "1.0.0",
		"case_id":        "case_synthetic_e2e_01",
		"title":          "Synthetic E2E Fixture",
		"summary":        "Synthetic test case for workbench-worker Unit 9 tests.",
		"classification": "synthetic_fixture",
		"source": map[string]any{
			"kind":  "synthetic_construction",
			"notes": "Synthetic fixture constructed in unit test.",
		},
		"network": map[string]any{
			"name":             "Synthetic Standalone Network",
			"protocol_version": 21,
			"ledger_sequence":  expectedSeq,
		},
		"input": map[string]any{
			"format":      "LedgerCloseMeta",
			"encoding":    "xdr_binary",
			"sha256":      shaHex,
			"byte_length": len(payload),
			"object_path": relObjPath,
		},
		"expectations": []map[string]any{
			{
				"assertion_id": "assertion_seq_match",
				"kind":         "ledger_sequence_equals",
				"expected":     expectedSeq,
				"description":  "Ledger sequence matches expected",
			},
			{
				"assertion_id": "assertion_type_match",
				"kind":         "decoded_type_equals",
				"expected":     "LedgerCloseMeta",
				"description":  "Decoded type is LedgerCloseMeta",
			},
			{
				"assertion_id": "assertion_hash_match",
				"kind":         "input_hash_equals",
				"expected":     shaHex,
				"description":  "Input SHA-256 matches verified input",
			},
		},
		"implementation_context": map[string]any{
			"stellar_core_version": nil,
			"sdk_versions":         map[string]string{"go": "v0.9.0"},
			"indexer_version":      nil,
		},
		"limitations": []string{"synthetic test fixture"},
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	manifestPath = filepath.Join(base, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	outDir = filepath.Join(base, "artifacts")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir outDir: %v", err)
	}
	return manifestPath, inputRoot, relObjPath, outDir, shaHex
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestCLI_HelpVersionAndUsageErrors(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"help"}, &out, &errBuf); code != exitOK {
		t.Errorf("help exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "workbench-worker") {
		t.Errorf("help output missing workbench-worker: %s", out.String())
	}

	out.Reset()
	if code := run([]string{"version"}, &out, &errBuf); code != exitOK {
		t.Errorf("version exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "workbench-worker") {
		t.Errorf("version output missing workbench-worker: %s", out.String())
	}

	if code := run([]string{}, &out, &errBuf); code != exitUsageError {
		t.Errorf("no-args exit = %d, want %d", code, exitUsageError)
	}
	if code := run([]string{"run", "--unknown-flag"}, &out, &errBuf); code != exitUsageError {
		t.Errorf("unknown-flag exit = %d, want %d", code, exitUsageError)
	}
	if code := run([]string{"run", "--manifest", "m.json"}, &out, &errBuf); code != exitUsageError {
		t.Errorf("missing required flags exit = %d, want %d", code, exitUsageError)
	}
}

func TestWorker_InputAndManifestValidationErrors(t *testing.T) {
	payload := makeSyntheticLedgerXDR(t, 100, 21)

	t.Run("invalid_manifest_json", func(t *testing.T) {
		mPath, root, rel, outDir, _ := makeSyntheticCorpusAndManifest(t, payload, 100)
		_ = os.WriteFile(mPath, []byte("{bad-json"), 0o600)
		_, code, err := executeRun(context.Background(), runConfig{
			ManifestPath: mPath, InputRoot: root, InputObject: rel, OutputDir: outDir,
			TimeoutMs: adapter.DefaultTimeoutMs, MaxInputBytes: 1024 * 1024, MaxOutputBytes: 1024 * 1024,
			adapterBinaryPath: testGoIngestBin,
		}, discardLogger())
		if code != exitInputError || err == nil {
			t.Errorf("got code=%d err=%v, want exitInputError", code, err)
		}
	})

	t.Run("oversized_manifest_rejected", func(t *testing.T) {
		mPath, root, rel, outDir, _ := makeSyntheticCorpusAndManifest(t, payload, 100)
		_ = os.WriteFile(mPath, bytes.Repeat([]byte(" "), int(evidence.MaxManifestBytes)+10), 0o600)
		_, code, _ := executeRun(context.Background(), runConfig{
			ManifestPath: mPath, InputRoot: root, InputObject: rel, OutputDir: outDir,
			TimeoutMs: adapter.DefaultTimeoutMs, MaxInputBytes: 1024 * 1024, MaxOutputBytes: 1024 * 1024,
			adapterBinaryPath: testGoIngestBin,
		}, discardLogger())
		if code != exitInputError {
			t.Errorf("got code=%d, want exitInputError", code)
		}
	})

	t.Run("input_path_mismatch_rejected", func(t *testing.T) {
		mPath, root, _, outDir, _ := makeSyntheticCorpusAndManifest(t, payload, 100)
		_, code, _ := executeRun(context.Background(), runConfig{
			ManifestPath: mPath, InputRoot: root, InputObject: "synthetic/other.xdr", OutputDir: outDir,
			TimeoutMs: adapter.DefaultTimeoutMs, MaxInputBytes: 1024 * 1024, MaxOutputBytes: 1024 * 1024,
			adapterBinaryPath: testGoIngestBin,
		}, discardLogger())
		if code != exitInputError {
			t.Errorf("got code=%d, want exitInputError", code)
		}
	})

	t.Run("sha256_mismatch_rejected_before_adapter", func(t *testing.T) {
		mPath, root, rel, outDir, _ := makeSyntheticCorpusAndManifest(t, payload, 100)
		// Corrupt the input file after manifest creation.
		_ = os.WriteFile(filepath.Join(root, rel), []byte("tampered-bytes"), 0o644)
		_, code, _ := executeRun(context.Background(), runConfig{
			ManifestPath: mPath, InputRoot: root, InputObject: rel, OutputDir: outDir,
			TimeoutMs: adapter.DefaultTimeoutMs, MaxInputBytes: 1024 * 1024, MaxOutputBytes: 1024 * 1024,
			adapterBinaryPath: "/nonexistent/adapter/should/not/run",
		}, discardLogger())
		if code != exitInputError {
			t.Errorf("got code=%d, want exitInputError (must fail before adapter)", code)
		}
	})

	t.Run("missing_output_dir_rejected", func(t *testing.T) {
		mPath, root, rel, outDir, _ := makeSyntheticCorpusAndManifest(t, payload, 100)
		_, code, _ := executeRun(context.Background(), runConfig{
			ManifestPath: mPath, InputRoot: root, InputObject: rel, OutputDir: filepath.Join(outDir, "missing"),
			TimeoutMs: adapter.DefaultTimeoutMs, MaxInputBytes: 1024 * 1024, MaxOutputBytes: 1024 * 1024,
			adapterBinaryPath: testGoIngestBin,
		}, discardLogger())
		if code != exitInputError {
			t.Errorf("got code=%d, want exitInputError", code)
		}
	})
}

func TestWorker_AdapterFailureInvalidXDRAndCancellation(t *testing.T) {
	t.Run("invalid_xdr_yields_exitAdapterError_not_regression", func(t *testing.T) {
		badXDR := []byte{0x00, 0x00, 0x00, 0x01, 0xde, 0xad, 0xbe, 0xef}
		mPath, root, rel, outDir, _ := makeSyntheticCorpusAndManifest(t, badXDR, 100)
		_, code, _ := executeRun(context.Background(), runConfig{
			ManifestPath: mPath, InputRoot: root, InputObject: rel, OutputDir: outDir,
			TimeoutMs: adapter.DefaultTimeoutMs, MaxInputBytes: 1024 * 1024, MaxOutputBytes: 1024 * 1024,
			adapterBinaryPath: testGoIngestBin,
		}, discardLogger())
		if code != exitAdapterError {
			t.Errorf("got code=%d, want %d (exitAdapterError)", code, exitAdapterError)
		}
	})

	t.Run("adapter_unavailable_yields_exitAdapterError", func(t *testing.T) {
		payload := makeSyntheticLedgerXDR(t, 100, 21)
		mPath, root, rel, outDir, _ := makeSyntheticCorpusAndManifest(t, payload, 100)
		_, code, _ := executeRun(context.Background(), runConfig{
			ManifestPath: mPath, InputRoot: root, InputObject: rel, OutputDir: outDir,
			TimeoutMs: adapter.DefaultTimeoutMs, MaxInputBytes: 1024 * 1024, MaxOutputBytes: 1024 * 1024,
			adapterBinaryPath: filepath.Join(t.TempDir(), "missing-adapter"),
		}, discardLogger())
		if code != exitAdapterError {
			t.Errorf("got code=%d, want %d (exitAdapterError)", code, exitAdapterError)
		}
	})

	t.Run("cancelled_run_yields_exitCancelled", func(t *testing.T) {
		payload := makeSyntheticLedgerXDR(t, 100, 21)
		mPath, root, rel, outDir, _ := makeSyntheticCorpusAndManifest(t, payload, 100)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, code, _ := executeRun(ctx, runConfig{
			ManifestPath: mPath, InputRoot: root, InputObject: rel, OutputDir: outDir,
			TimeoutMs: adapter.DefaultTimeoutMs, MaxInputBytes: 1024 * 1024, MaxOutputBytes: 1024 * 1024,
			adapterBinaryPath: testGoIngestBin,
		}, discardLogger())
		if code != exitCancelled {
			t.Errorf("got code=%d, want %d (exitCancelled)", code, exitCancelled)
		}
	})
}

func TestWorker_OutcomeToExitCodeMapping(t *testing.T) {
	tests := []struct {
		outcome domain.RunOutcome
		want    int
	}{
		{domain.RunOutcomePass, exitOK},
		{domain.RunOutcomeRegression, exitAssertionFailure},
		{domain.RunOutcomeInconclusive, exitInconclusive},
		{domain.RunOutcomeNotApplicable, exitNotApplicable},
	}
	for _, tc := range tests {
		if got := outcomeToExitCode(tc.outcome); got != tc.want {
			t.Errorf("outcomeToExitCode(%q) = %d, want %d", tc.outcome, got, tc.want)
		}
	}
}

func TestEndToEnd_SubprocessSyntheticPassRegressionAndDeterminism(t *testing.T) {
	payload := makeSyntheticLedgerXDR(t, 4242, 21)

	t.Run("pass_and_repeated_run_determinism", func(t *testing.T) {
		mPath, root, rel, outDir, _ := makeSyntheticCorpusAndManifest(t, payload, 4242)

		runOnce := func() *evidence.Manifest {
			cmd := exec.Command(testWorkerBin, "run",
				"--manifest", mPath,
				"--input-root", root,
				"--input", rel,
				"--output-dir", outDir,
				"--json",
			)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("workbench-worker pass run failed: %v\nstderr: %s", err, stderr.String())
			}

			// Validate stdout JSON report and compare byte-for-byte with bundle manifest.json and <run_id>.json.
			m, err := evidence.DecodeAndValidateManifest(stdout.Bytes())
			if err != nil {
				t.Fatalf("DecodeAndValidateManifest(stdout): %v", err)
			}
			if m.Run.State != domain.RunStateCompleted || *m.Run.Outcome != domain.RunOutcomePass {
				t.Fatalf("unexpected state/outcome: %s / %v", m.Run.State, m.Run.Outcome)
			}

			vr, err := evidence.VerifyBundle(outDir, m.RunID)
			if err != nil {
				t.Fatalf("VerifyBundle(%s): %v", m.RunID, err)
			}
			if vr.ArtifactCount != 3 {
				t.Errorf("ArtifactCount = %d, want 3", vr.ArtifactCount)
			}

			bundleManifestBytes, err := os.ReadFile(filepath.Join(outDir, m.RunID, "manifest.json"))
			if err != nil {
				t.Fatalf("read bundle manifest.json: %v", err)
			}
			reportJSONBytes, err := os.ReadFile(filepath.Join(outDir, m.RunID+".json"))
			if err != nil {
				t.Fatalf("read report json: %v", err)
			}
			if !bytes.Equal(stdout.Bytes(), reportJSONBytes) {
				t.Error("stdout JSON does not match <run_id>.json")
			}
			if !bytes.Equal(bundleManifestBytes, reportJSONBytes) {
				t.Error("bundle manifest.json does not match <run_id>.json byte-for-byte")
			}
			if _, err := os.Stat(filepath.Join(outDir, m.RunID+".md")); err != nil {
				t.Errorf("expected Markdown report <run_id>.md: %v", err)
			}
			return m
		}

		m1 := runOnce()
		m2 := runOnce()
		if m1.RunID == m2.RunID {
			t.Fatalf("expected distinct run IDs across runs, both got %q", m1.RunID)
		}

		// Compare semantic results: semantic-projection.json and assertions (excluding volatile run IDs/timestamps/durations).
		proj1, _ := os.ReadFile(filepath.Join(outDir, m1.RunID, "artifacts", "semantic-projection.json"))
		proj2, _ := os.ReadFile(filepath.Join(outDir, m2.RunID, "artifacts", "semantic-projection.json"))
		if !bytes.Equal(proj1, proj2) {
			t.Error("semantic-projection.json differed between repeated runs")
		}
		if !reflect.DeepEqual(m1.Assertions, m2.Assertions) {
			t.Errorf("assertions differed between repeated runs:\n%+v\nvs\n%+v", m1.Assertions, m2.Assertions)
		}
		if *m1.Run.Outcome != *m2.Run.Outcome {
			t.Errorf("outcomes differed: %s vs %s", *m1.Run.Outcome, *m2.Run.Outcome)
		}
	})

	t.Run("regression_returns_exit_4_with_completed_evidence", func(t *testing.T) {
		// Expect sequence 9999 while ledger actually has 4242 -> regression (exit 4).
		mPath, root, rel, outDir, _ := makeSyntheticCorpusAndManifest(t, payload, 9999)
		cmd := exec.Command(testWorkerBin, "run",
			"--manifest", mPath,
			"--input-root", root,
			"--input", rel,
			"--output-dir", outDir,
			"--json",
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		var exitErr *exec.ExitError
		if err == nil || !strings.Contains(err.Error(), "exit status") {
			t.Fatalf("expected exit code 4, got err=%v", err)
		}
		if ok := (func() bool {
			if ee, ok := err.(*exec.ExitError); ok {
				exitErr = ee
				return ee.ExitCode() == exitAssertionFailure
			}
			return false
		})(); !ok {
			t.Fatalf("got exit code %v, want %d (stderr: %s)", exitErr, exitAssertionFailure, stderr.String())
		}

		m, decErr := evidence.DecodeAndValidateManifest(stdout.Bytes())
		if decErr != nil {
			t.Fatalf("DecodeAndValidateManifest(regression stdout): %v", decErr)
		}
		if m.Run.State != domain.RunStateCompleted || *m.Run.Outcome != domain.RunOutcomeRegression {
			t.Errorf("unexpected state/outcome: %s / %v", m.Run.State, m.Run.Outcome)
		}
		if _, vErr := evidence.VerifyBundle(outDir, m.RunID); vErr != nil {
			t.Errorf("VerifyBundle(regression): %v", vErr)
		}
	})
}
