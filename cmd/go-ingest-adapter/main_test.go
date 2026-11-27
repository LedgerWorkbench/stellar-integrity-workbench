package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter/goingest"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/source/localfile"
	"github.com/stellar/go-stellar-sdk/xdr"
)

var testAdapterBinary string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "siw-go-ingest-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	binName := "go-ingest-adapter"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	testAdapterBinary = filepath.Join(tmpDir, binName)

	buildCmd := exec.Command("go", "build", "-o", testAdapterBinary, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build go-ingest-adapter: %v\n%s\n", err, string(out))
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to clean up temp dir %s: %v\n", tmpDir, rmErr)
		}
		os.Exit(1)
	}

	adapter.Init()
	exitCode := m.Run()
	if err := os.RemoveAll(tmpDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to clean up temp dir %s: %v\n", tmpDir, err)
	}
	os.Exit(exitCode)
}

func makeSyntheticLedgerBytes(t *testing.T, seq uint32, protocol uint32) []byte {
	t.Helper()
	var h xdr.Hash
	h[0] = 0xab
	var txHash xdr.Hash
	txHash[0] = 0xcd

	var src xdr.MuxedAccount
	src.Type = xdr.CryptoKeyTypeKeyTypeEd25519
	var pk xdr.Uint256
	pk[0] = 0x01
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
					ScpValue: xdr.StellarValue{
						CloseTime: 1700000500,
					},
					LedgerSeq: xdr.Uint32(seq),
				},
			},
			TxSet: xdr.GeneralizedTransactionSet{
				V: 1,
				V1TxSet: &xdr.TransactionSetV1{
					Phases: []xdr.TransactionPhase{
						{V: 0, V0Components: &components},
					},
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
					TxApplyProcessing: xdr.TransactionMeta{
						V:          0,
						Operations: &[]xdr.OperationMeta{},
					},
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

func stageVerifiedInput(t *testing.T, data []byte) *localfile.VerifiedInput {
	t.Helper()
	rootDir := t.TempDir()
	relPath := "case/input.xdr"
	fullPath := filepath.Join(rootDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	sum := sha256.Sum256(data)
	hashHex := hex.EncodeToString(sum[:])

	store, err := localfile.NewStore(localfile.Config{
		RootDir:  rootDir,
		MaxBytes: 10 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	v, err := store.LoadAndVerify(relPath, int64(len(data)), hashHex)
	if err != nil {
		t.Fatalf("store.LoadAndVerify: %v", err)
	}
	return v
}

func newTestRunner(t *testing.T) *adapter.ProcessRunner {
	t.Helper()
	reg, err := adapter.NewProductionRegistry(adapter.ProductionRegistryConfig{
		GoIngestBinaryPath: testAdapterBinary,
		GoIngestVersion:    adapter.DefaultGoAdapterVersion,
		GoSDKVersion:       adapter.PinnedGoSDKVersion,
	})
	if err != nil {
		t.Fatalf("NewProductionRegistry: %v", err)
	}
	runner, err := adapter.NewProcessRunner(adapter.ProcessRunnerConfig{
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("NewProcessRunner: %v", err)
	}
	return runner
}

func TestGoIngestAdapter_ProcessRunner_Success(t *testing.T) {
	payload := makeSyntheticLedgerBytes(t, 4242, 22)
	verified := stageVerifiedInput(t, payload)
	runner := newTestRunner(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	out, err := runner.Run(ctx, adapter.RunParams{
		RunID:       "run_unit5_success",
		CaseID:      "case_synthetic_001",
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
	}, verified)
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}

	if out.ActualExitCode != 0 {
		t.Errorf("ActualExitCode = %d, want 0", out.ActualExitCode)
	}
	if out.Result.Execution.Status != adapter.ExecutionStatusCompleted {
		t.Errorf("Execution.Status = %q, want %q", out.Result.Execution.Status, adapter.ExecutionStatusCompleted)
	}
	if out.Result.Decode.Status != adapter.DecodeStatusSuccess {
		t.Errorf("Decode.Status = %q, want %q", out.Result.Decode.Status, adapter.DecodeStatusSuccess)
	}
	if out.Result.Decode.DetectedType == nil || *out.Result.Decode.DetectedType != "LedgerCloseMeta" {
		t.Errorf("DetectedType = %v, want LedgerCloseMeta", out.Result.Decode.DetectedType)
	}
	if out.Result.SemanticProjection == nil {
		t.Fatal("SemanticProjection is nil, want populated projection")
	}

	var proj goingest.SemanticProjection
	if err := json.Unmarshal(*out.Result.SemanticProjection, &proj); err != nil {
		t.Fatalf("unmarshal SemanticProjection: %v", err)
	}
	if proj.Ledger.Sequence != 4242 {
		t.Errorf("proj.Ledger.Sequence = %d, want 4242", proj.Ledger.Sequence)
	}
	if proj.Ledger.ProtocolVersion != 22 {
		t.Errorf("proj.Ledger.ProtocolVersion = %d, want 22", proj.Ledger.ProtocolVersion)
	}
	if proj.Transactions.Total != 1 || proj.Transactions.Successful != 1 || proj.Transactions.Failed != 0 {
		t.Errorf("unexpected transaction counts: %+v", proj.Transactions)
	}
}

func TestGoIngestAdapter_ProcessRunner_MalformedXDR(t *testing.T) {
	// Use data whose first 4 bytes form a supported outer discriminant (V=1)
	// but whose body is truncated garbage, so SafeUnmarshal fails with
	// ErrXDRDecodeFailed (decode.status = "invalid_input").
	malformed := []byte{0x00, 0x00, 0x00, 0x01, 0xDE, 0xAD, 0xBE, 0xEF}
	verified := stageVerifiedInput(t, malformed)
	runner := newTestRunner(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	out, err := runner.Run(ctx, adapter.RunParams{
		RunID:       "run_unit5_malformed",
		CaseID:      "case_synthetic_bad_xdr",
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
	}, verified)
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}

	if out.ActualExitCode != exitDecodeError {
		t.Errorf("ActualExitCode = %d, want %d", out.ActualExitCode, exitDecodeError)
	}
	if out.Result.Execution.Status != adapter.ExecutionStatusFailed {
		t.Errorf("Execution.Status = %q, want %q", out.Result.Execution.Status, adapter.ExecutionStatusFailed)
	}
	if out.Result.Decode.Status != adapter.DecodeStatusInvalidInput {
		t.Errorf("Decode.Status = %q, want %q", out.Result.Decode.Status, adapter.DecodeStatusInvalidInput)
	}
	if out.Result.SemanticProjection != nil {
		t.Errorf("SemanticProjection = %s, want nil", string(*out.Result.SemanticProjection))
	}
}

func TestGoIngestAdapter_ProcessRunner_UnsupportedVariant(t *testing.T) {
	// Construct a 4-byte big-endian int32 discriminant of 99 (unsupported outer variant).
	unsupportedData := []byte{0x00, 0x00, 0x00, 99}
	verified := stageVerifiedInput(t, unsupportedData)
	runner := newTestRunner(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	out, err := runner.Run(ctx, adapter.RunParams{
		RunID:       "run_unit5_unsupported_variant",
		CaseID:      "case_synthetic_unsupported_v99",
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
	}, verified)
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}

	// Verify process-level protocol expectations.
	if out.ActualExitCode != exitDecodeError {
		t.Errorf("ActualExitCode = %d, want %d (exitDecodeError)", out.ActualExitCode, exitDecodeError)
	}
	if out.Result.Execution.Status != adapter.ExecutionStatusFailed {
		t.Errorf("Execution.Status = %q, want %q", out.Result.Execution.Status, adapter.ExecutionStatusFailed)
	}
	if out.Result.Execution.ExitCode != exitDecodeError {
		t.Errorf("Execution.ExitCode = %d, want %d", out.Result.Execution.ExitCode, exitDecodeError)
	}
	if out.Result.Decode.Status != adapter.DecodeStatusUnsupportedType {
		t.Errorf("Decode.Status = %q, want %q", out.Result.Decode.Status, adapter.DecodeStatusUnsupportedType)
	}
	if out.Result.SemanticProjection != nil {
		t.Errorf("SemanticProjection = %s, want nil", string(*out.Result.SemanticProjection))
	}

	// Verify diagnostic code is the specific unsupported outer variant code.
	foundDiagCode := false
	for _, d := range out.Result.Diagnostics {
		if d.Code == "unsupported_lcm_variant" {
			foundDiagCode = true
			break
		}
	}
	if !foundDiagCode {
		t.Errorf("expected diagnostic code 'unsupported_lcm_variant', got diagnostics: %+v", out.Result.Diagnostics)
	}
}

func TestGoIngestAdapter_Direct_HashMismatchAndOversized(t *testing.T) {
	payload := makeSyntheticLedgerBytes(t, 100, 20)
	sum := sha256.Sum256(payload)
	actualHash := hex.EncodeToString(sum[:])
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.xdr")
	if err := os.WriteFile(inputPath, payload, 0o400); err != nil {
		t.Fatalf("write input: %v", err)
	}

	cases := []struct {
		name     string
		hash     string
		maxBytes int64
	}{
		{name: "hash_mismatch", hash: wrongHash, maxBytes: int64(len(payload))},
		{name: "oversized", hash: actualHash, maxBytes: int64(len(payload) - 1)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &adapter.Request{
				SchemaVersion: adapter.ProtocolVersion,
				RunID:         "run_" + tc.name,
				CaseID:        "case_" + tc.name,
				Operation:     adapter.OperationDecodeLedgerMeta,
				InputPath:     inputPath,
				InputFormat:   adapter.InputFormatLedgerCloseMeta,
				InputSHA256:   tc.hash,
				Target: adapter.RequestTarget{
					AdapterID:      adapter.AdapterIDGoIngest,
					AdapterVersion: adapter.DefaultGoAdapterVersion,
					SDKVersion:     adapter.PinnedGoSDKVersion,
				},
				Limits: adapter.RequestLimits{
					TimeoutMs:      5000,
					MaxInputBytes:  tc.maxBytes,
					MaxOutputBytes: 1024 * 1024,
				},
			}
			reqBytes, err := adapter.EncodeRequest(req)
			if err != nil {
				t.Fatalf("EncodeRequest: %v", err)
			}

			cmd := exec.Command(testAdapterBinary)
			cmd.Stdin = bytes.NewReader(reqBytes)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err = cmd.Run()
			if err == nil {
				t.Fatalf("expected non-zero exit code for %s", tc.name)
			}
			exitErr, ok := err.(*exec.ExitError)
			if !ok || exitErr.ExitCode() != exitInputError {
				t.Fatalf("expected exit code %d, got err=%v", exitInputError, err)
			}

			res, err := adapter.DecodeResult(stdout.Bytes())
			if err != nil {
				t.Fatalf("DecodeResult failed (extra stdout?): %v\nstdout: %s", err, stdout.String())
			}
			if res.Decode.Status != adapter.DecodeStatusNotAttempted {
				t.Errorf("Decode.Status = %q, want %q", res.Decode.Status, adapter.DecodeStatusNotAttempted)
			}
			if res.Execution.ExitCode != exitInputError {
				t.Errorf("Execution.ExitCode = %d, want %d", res.Execution.ExitCode, exitInputError)
			}
		})
	}
}
