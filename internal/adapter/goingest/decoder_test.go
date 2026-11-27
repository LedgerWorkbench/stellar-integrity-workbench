package goingest

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// All fixtures in this file are strictly synthetic, constructed using the
// official github.com/stellar/go-stellar-sdk v0.9.0 XDR types.

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func makeSyntheticHash(seed byte) xdr.Hash {
	var h xdr.Hash
	for i := range h {
		h[i] = seed + byte(i)
	}
	return h
}

func makeSyntheticLedgerHeader(seq uint32, protocol uint32, closeTime uint64) xdr.LedgerHeaderHistoryEntry {
	return xdr.LedgerHeaderHistoryEntry{
		Hash: makeSyntheticHash(0x10),
		Header: xdr.LedgerHeader{
			LedgerVersion:      xdr.Uint32(protocol),
			PreviousLedgerHash: makeSyntheticHash(0x20),
			ScpValue: xdr.StellarValue{
				CloseTime: xdr.TimePoint(closeTime),
			},
			LedgerSeq: xdr.Uint32(seq),
		},
	}
}

func makeSyntheticTxEnvelope() xdr.TransactionEnvelope {
	var src xdr.MuxedAccount
	src.Type = xdr.CryptoKeyTypeKeyTypeEd25519
	var pk xdr.Uint256
	pk[0] = 0x42
	src.Ed25519 = &pk

	return xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTx,
		V1: &xdr.TransactionV1Envelope{
			Tx: xdr.Transaction{
				SourceAccount: src,
				Fee:           100,
				SeqNum:        1,
			},
		},
	}
}

func makeSyntheticTxResultPair(txHash xdr.Hash, success bool) xdr.TransactionResultPair {
	code := xdr.TransactionResultCodeTxSuccess
	if !success {
		code = xdr.TransactionResultCodeTxFailed
	}
	return xdr.TransactionResultPair{
		TransactionHash: txHash,
		Result: xdr.TransactionResult{
			FeeCharged: 100,
			Result: xdr.TransactionResultResult{
				Code:    code,
				Results: &[]xdr.OperationResult{},
			},
		},
	}
}

func makeSyntheticLCMv0(t *testing.T) (xdr.LedgerCloseMeta, []byte) {
	t.Helper()
	h1 := makeSyntheticHash(0xa1)
	h2 := makeSyntheticHash(0xb2)

	lcm := xdr.LedgerCloseMeta{
		V: 0,
		V0: &xdr.LedgerCloseMetaV0{
			LedgerHeader: makeSyntheticLedgerHeader(100, 19, 1700000100),
			TxSet: xdr.TransactionSet{
				Txs: []xdr.TransactionEnvelope{
					makeSyntheticTxEnvelope(),
					makeSyntheticTxEnvelope(),
				},
			},
			TxProcessing: []xdr.TransactionResultMeta{
				{
					Result:            makeSyntheticTxResultPair(h1, true),
					TxApplyProcessing: xdr.TransactionMeta{V: 0, Operations: &[]xdr.OperationMeta{}},
				},
				{
					Result:            makeSyntheticTxResultPair(h2, false),
					TxApplyProcessing: xdr.TransactionMeta{V: 0, Operations: &[]xdr.OperationMeta{}},
				},
			},
		},
	}

	var buf bytes.Buffer
	if _, err := xdr.Marshal(&buf, lcm); err != nil {
		t.Fatalf("failed to marshal synthetic LCM V0: %v", err)
	}
	return lcm, buf.Bytes()
}

func makeSyntheticLCMv1(t *testing.T, txOutcomes []bool) (xdr.LedgerCloseMeta, []byte) {
	t.Helper()
	txs := make([]xdr.TransactionEnvelope, len(txOutcomes))
	processing := make([]xdr.TransactionResultMeta, len(txOutcomes))
	for i, ok := range txOutcomes {
		txs[i] = makeSyntheticTxEnvelope()
		processing[i] = xdr.TransactionResultMeta{
			Result:            makeSyntheticTxResultPair(makeSyntheticHash(byte(0x30+i)), ok),
			TxApplyProcessing: xdr.TransactionMeta{V: 0, Operations: &[]xdr.OperationMeta{}},
		}
	}

	components := []xdr.TxSetComponent{
		{
			Type: xdr.TxSetComponentTypeTxsetCompTxsMaybeDiscountedFee,
			TxsMaybeDiscountedFee: &xdr.TxSetComponentTxsMaybeDiscountedFee{
				Txs: txs,
			},
		},
	}

	lcm := xdr.LedgerCloseMeta{
		V: 1,
		V1: &xdr.LedgerCloseMetaV1{
			LedgerHeader: makeSyntheticLedgerHeader(200, 21, 1700000200),
			TxSet: xdr.GeneralizedTransactionSet{
				V: 1,
				V1TxSet: &xdr.TransactionSetV1{
					Phases: []xdr.TransactionPhase{
						{
							V:            0,
							V0Components: &components,
						},
					},
				},
			},
			TxProcessing: processing,
		},
	}

	var buf bytes.Buffer
	if _, err := xdr.Marshal(&buf, lcm); err != nil {
		t.Fatalf("failed to marshal synthetic LCM V1: %v", err)
	}
	return lcm, buf.Bytes()
}

func makeSyntheticLCMv2(t *testing.T, txOutcomes []bool) (xdr.LedgerCloseMeta, []byte) {
	t.Helper()
	txs := make([]xdr.TransactionEnvelope, len(txOutcomes))
	processing := make([]xdr.TransactionResultMetaV1, len(txOutcomes))
	for i, ok := range txOutcomes {
		txs[i] = makeSyntheticTxEnvelope()
		processing[i] = xdr.TransactionResultMetaV1{
			Result:            makeSyntheticTxResultPair(makeSyntheticHash(byte(0x50+i)), ok),
			TxApplyProcessing: xdr.TransactionMeta{V: 0, Operations: &[]xdr.OperationMeta{}},
		}
	}

	lcm := xdr.LedgerCloseMeta{
		V: 2,
		V2: &xdr.LedgerCloseMetaV2{
			LedgerHeader: makeSyntheticLedgerHeader(300, 23, 1700000300),
			TxSet: xdr.GeneralizedTransactionSet{
				V: 1,
				V1TxSet: &xdr.TransactionSetV1{
					Phases: []xdr.TransactionPhase{
						{
							V: 1,
							ParallelTxsComponent: &xdr.ParallelTxsComponent{
								ExecutionStages: []xdr.ParallelTxExecutionStage{
									{txs},
								},
							},
						},
					},
				},
			},
			TxProcessing: processing,
		},
	}

	var buf bytes.Buffer
	if _, err := xdr.Marshal(&buf, lcm); err != nil {
		t.Fatalf("failed to marshal synthetic LCM V2: %v", err)
	}
	return lcm, buf.Bytes()
}

func TestDecodeLedgerCloseMeta_SupportedVariants(t *testing.T) {
	_, v0Bytes := makeSyntheticLCMv0(t)
	_, v1Bytes := makeSyntheticLCMv1(t, []bool{true, false, true})
	_, v2Bytes := makeSyntheticLCMv2(t, []bool{true, true})

	tests := []struct {
		name        string
		data        []byte
		wantVariant int32
		wantSeq     uint32
	}{
		{name: "synthetic V0", data: v0Bytes, wantVariant: 0, wantSeq: 100},
		{name: "synthetic V1", data: v1Bytes, wantVariant: 1, wantSeq: 200},
		{name: "synthetic V2", data: v2Bytes, wantVariant: 2, wantSeq: 300},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decoded, err := DecodeLedgerCloseMeta(tc.data)
			if err != nil {
				t.Fatalf("DecodeLedgerCloseMeta failed: %v", err)
			}
			if decoded.OuterVariant != tc.wantVariant {
				t.Errorf("OuterVariant = %d, want %d", decoded.OuterVariant, tc.wantVariant)
			}
			if gotSeq := decoded.Meta.LedgerSequence(); gotSeq != tc.wantSeq {
				t.Errorf("LedgerSequence() = %d, want %d", gotSeq, tc.wantSeq)
			}
		})
	}
}

func TestDecodeLedgerCloseMeta_InvalidInputs(t *testing.T) {
	_, validBytes := makeSyntheticLCMv1(t, []bool{true})

	// Different top-level XDR type (ScVal)
	scVal := xdr.ScVal{Type: xdr.ScValTypeScvU32, U32: new(xdr.Uint32)}
	*scVal.U32 = 42
	var scBuf bytes.Buffer
	if _, err := xdr.Marshal(&scBuf, scVal); err != nil {
		t.Fatalf("marshal ScVal: %v", err)
	}

	// Trailing bytes after valid LedgerCloseMeta
	withTrailing := append(append([]byte(nil), validBytes...), 0x01, 0x02, 0x03, 0x04)

	// Truncated XDR
	truncated := validBytes[:len(validBytes)/2]

	// Random invalid bytes
	randomBytes := []byte{0xff, 0xfe, 0xfd, 0xfc, 0x00, 0x11, 0x22, 0x33}

	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "truncated", data: truncated},
		{name: "random_bytes", data: randomBytes},
		{name: "different_xdr_type_scval", data: scBuf.Bytes()},
		{name: "trailing_bytes", data: withTrailing},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeLedgerCloseMeta(tc.data)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !errors.Is(err, ErrXDRDecodeFailed) {
				t.Errorf("expected ErrXDRDecodeFailed for %s, got %v", tc.name, err)
			}
		})
	}
}

func TestDecodeLedgerCloseMeta_UnsupportedVariant(t *testing.T) {
	// Outer variant 99 encoded in XDR (4-byte big-endian int32)
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], 99)

	// SafeUnmarshal rejects unknown union switch in go-xdr with ErrXDRDecodeFailed,
	// while structural validation catches an unhandled variant directly.
	_, err := DecodeLedgerCloseMeta(buf[:])
	if err == nil {
		t.Fatal("expected error for unsupported discriminant 99, got nil")
	}

	// Also verify structural validation catches an unsupported outer variant directly.
	lcm := xdr.LedgerCloseMeta{V: 99}
	err = validateLedgerCloseMetaStructure(&lcm)
	if !errors.Is(err, ErrUnsupportedXDRVariant) {
		t.Errorf("expected ErrUnsupportedXDRVariant, got %v", err)
	}
}

func TestLoadAndVerifyInput(t *testing.T) {
	_, payload := makeSyntheticLCMv1(t, []bool{true})
	expectedHash := sha256Hex(payload)

	dir := t.TempDir()
	validPath := filepath.Join(dir, "input.xdr")
	if err := os.WriteFile(validPath, payload, 0o400); err != nil {
		t.Fatalf("write valid input: %v", err)
	}

	t.Run("valid_input", func(t *testing.T) {
		loaded, err := LoadAndVerifyInput(validPath, expectedHash, int64(len(payload)))
		if err != nil {
			t.Fatalf("LoadAndVerifyInput failed: %v", err)
		}
		if loaded.SHA256 != expectedHash {
			t.Errorf("SHA256 = %q, want %q", loaded.SHA256, expectedHash)
		}
		if loaded.Size != int64(len(payload)) {
			t.Errorf("Size = %d, want %d", loaded.Size, len(payload))
		}
		if !bytes.Equal(loaded.Data, payload) {
			t.Error("loaded bytes do not match original bytes")
		}
	})

	t.Run("hash_mismatch", func(t *testing.T) {
		wrongHash := sha256Hex([]byte("different"))
		_, err := LoadAndVerifyInput(validPath, wrongHash, int64(len(payload)))
		if !errors.Is(err, ErrInputHashMismatch) {
			t.Errorf("expected ErrInputHashMismatch, got %v", err)
		}
	})

	t.Run("too_large", func(t *testing.T) {
		_, err := LoadAndVerifyInput(validPath, expectedHash, int64(len(payload)-1))
		if !errors.Is(err, ErrInputTooLarge) {
			t.Errorf("expected ErrInputTooLarge, got %v", err)
		}
	})

	t.Run("missing_file", func(t *testing.T) {
		_, err := LoadAndVerifyInput(filepath.Join(dir, "missing.xdr"), expectedHash, 1024)
		if !errors.Is(err, ErrMissingInput) {
			t.Errorf("expected ErrMissingInput, got %v", err)
		}
	})

	t.Run("directory_rejected", func(t *testing.T) {
		_, err := LoadAndVerifyInput(dir, expectedHash, 1024)
		if !errors.Is(err, ErrNotRegularFile) {
			t.Errorf("expected ErrNotRegularFile for directory, got %v", err)
		}
	})

	t.Run("symlink_rejected", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires elevated privileges on Windows")
		}
		linkPath := filepath.Join(dir, "link.xdr")
		if err := os.Symlink(validPath, linkPath); err != nil {
			t.Fatalf("create symlink: %v", err)
		}
		_, err := LoadAndVerifyInput(linkPath, expectedHash, int64(len(payload)))
		if !errors.Is(err, ErrNotRegularFile) {
			t.Errorf("expected ErrNotRegularFile for symlink, got %v", err)
		}
	})
}
