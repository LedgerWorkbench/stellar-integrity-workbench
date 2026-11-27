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

	// Truncated XDR (keep supported discriminant 1 from validBytes, truncate the body)
	truncated := validBytes[:len(validBytes)/2]

	// Random invalid bytes whose first 4 bytes form discriminant -66052,
	// which is outside {0, 1, 2} and thus classified as unsupported variant.
	randomBytes := []byte{0xff, 0xfe, 0xfd, 0xfc, 0x00, 0x11, 0x22, 0x33}

	t.Run("empty", func(t *testing.T) {
		_, err := DecodeLedgerCloseMeta(nil)
		if err == nil {
			t.Fatal("expected error for empty input, got nil")
		}
		if !errors.Is(err, ErrXDRDecodeFailed) {
			t.Errorf("expected ErrXDRDecodeFailed for empty, got %v", err)
		}
	})

	t.Run("truncated", func(t *testing.T) {
		_, err := DecodeLedgerCloseMeta(truncated)
		if err == nil {
			t.Fatal("expected error for truncated input, got nil")
		}
		if !errors.Is(err, ErrXDRDecodeFailed) {
			t.Errorf("expected ErrXDRDecodeFailed for truncated, got %v", err)
		}
	})

	t.Run("trailing_bytes", func(t *testing.T) {
		_, err := DecodeLedgerCloseMeta(withTrailing)
		if err == nil {
			t.Fatal("expected error for trailing bytes, got nil")
		}
		if !errors.Is(err, ErrXDRDecodeFailed) {
			t.Errorf("expected ErrXDRDecodeFailed for trailing_bytes, got %v", err)
		}
	})

	// Inputs whose first 4 bytes form a discriminant outside {0, 1, 2}
	// are classified as ErrUnsupportedXDRVariant by the view pre-check.
	t.Run("random_bytes_unsupported_discriminant", func(t *testing.T) {
		_, err := DecodeLedgerCloseMeta(randomBytes)
		if err == nil {
			t.Fatal("expected error for random bytes, got nil")
		}
		if !errors.Is(err, ErrUnsupportedXDRVariant) {
			t.Errorf("expected ErrUnsupportedXDRVariant for random bytes (disc -66052), got %v", err)
		}
	})

	t.Run("different_xdr_type_scval_unsupported_discriminant", func(t *testing.T) {
		_, err := DecodeLedgerCloseMeta(scBuf.Bytes())
		if err == nil {
			t.Fatal("expected error for ScVal input, got nil")
		}
		// ScVal type ScvU32 has discriminant 3, which is outside {0, 1, 2}
		// and classified as unsupported variant by the view pre-check.
		if !errors.Is(err, ErrUnsupportedXDRVariant) {
			t.Errorf("expected ErrUnsupportedXDRVariant for ScVal (disc 3), got %v", err)
		}
	})
}

func TestDecodeLedgerCloseMeta_UnsupportedOuterVariant(t *testing.T) {
	// After the XDR view pre-check, DecodeLedgerCloseMeta should return
	// ErrUnsupportedXDRVariant (not ErrXDRDecodeFailed) for outer
	// discriminants outside the supported set {0, 1, 2}.
	tests := []struct {
		name string
		disc uint32
	}{
		{name: "discriminant_99", disc: 99},
		{name: "discriminant_42", disc: 42},
		{name: "discriminant_3", disc: 3},
		{name: "discriminant_max_int32", disc: 0x7FFFFFFF},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Construct a valid 4-byte big-endian discriminant followed
			// by padding (the view only reads 4 bytes for V()).
			var buf [8]byte
			binary.BigEndian.PutUint32(buf[:4], tc.disc)

			_, err := DecodeLedgerCloseMeta(buf[:])
			if err == nil {
				t.Fatalf("expected error for unsupported discriminant %d, got nil", tc.disc)
			}
			if !errors.Is(err, ErrUnsupportedXDRVariant) {
				t.Errorf("expected ErrUnsupportedXDRVariant for discriminant %d, got %v", tc.disc, err)
			}
			// Must not be a generic decode failure.
			if errors.Is(err, ErrXDRDecodeFailed) {
				t.Errorf("discriminant %d should not be classified as ErrXDRDecodeFailed", tc.disc)
			}
		})
	}

	// Also verify structural validation catches an unsupported outer variant directly.
	lcm := xdr.LedgerCloseMeta{V: 99}
	err := validateLedgerCloseMetaStructure(&lcm)
	if !errors.Is(err, ErrUnsupportedXDRVariant) {
		t.Errorf("expected ErrUnsupportedXDRVariant from validateLedgerCloseMetaStructure, got %v", err)
	}
}

func TestDecodeLedgerCloseMeta_TruncatedDiscriminant(t *testing.T) {
	// Fewer than 4 bytes: the view cannot read the discriminant.
	tests := []struct {
		name string
		data []byte
	}{
		{name: "one_byte", data: []byte{0x00}},
		{name: "two_bytes", data: []byte{0x00, 0x00}},
		{name: "three_bytes", data: []byte{0x00, 0x00, 0x00}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeLedgerCloseMeta(tc.data)
			if err == nil {
				t.Fatal("expected error for truncated discriminant, got nil")
			}
			if !errors.Is(err, ErrXDRDecodeFailed) {
				t.Errorf("expected ErrXDRDecodeFailed, got %v", err)
			}
		})
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
