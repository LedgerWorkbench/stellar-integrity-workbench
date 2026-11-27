package goingest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// LoadedInput holds verified raw input bytes and their observed metadata.
type LoadedInput struct {
	Data   []byte
	SHA256 string
	Size   int64
}

// DecodedLedgerMeta holds a decoded and structurally validated LedgerCloseMeta
// value along with its outer XDR union variant (0, 1, or 2).
type DecodedLedgerMeta struct {
	Meta         xdr.LedgerCloseMeta
	OuterVariant int32
}

// LoadAndVerifyInput reads the worker-staged input file without modifying or
// normalizing it, enforces regular-file and maxBytes limits, computes SHA-256
// over the exact bytes, and verifies it against expectedSHA256.
func LoadAndVerifyInput(path, expectedSHA256 string, maxBytes int64) (*LoadedInput, error) {
	if strings.TrimSpace(path) == "" {
		return nil, newError(ErrMissingInput, "input", "empty_input_path",
			"input path must not be empty", nil)
	}
	if maxBytes <= 0 {
		return nil, newError(ErrInternal, "input", "invalid_max_input_bytes",
			fmt.Sprintf("max_input_bytes must be positive, got %d", maxBytes), nil)
	}

	// Reject symlinks and non-regular files before opening.
	linfo, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, newError(ErrMissingInput, "input", "input_not_found",
				"staged input file does not exist", err)
		}
		return nil, newError(ErrInternal, "input", "input_stat_failed",
			"failed to stat staged input file", err)
	}
	if linfo.Mode()&os.ModeSymlink != 0 {
		return nil, newError(ErrNotRegularFile, "input", "input_symlink_rejected",
			"staged input path must not be a symbolic link", nil)
	}
	if !linfo.Mode().IsRegular() {
		return nil, newError(ErrNotRegularFile, "input", "input_not_regular_file",
			fmt.Sprintf("staged input path is not a regular file (mode %s)", linfo.Mode()), nil)
	}

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, newError(ErrMissingInput, "input", "input_not_found",
				"staged input file does not exist", err)
		}
		return nil, newError(ErrInternal, "input", "input_open_failed",
			"failed to open staged input file", err)
	}
	defer f.Close()

	// Re-verify the opened file descriptor is still a regular file and within bounds.
	finfo, err := f.Stat()
	if err != nil {
		return nil, newError(ErrInternal, "input", "input_fstat_failed",
			"failed to stat opened input file", err)
	}
	if !finfo.Mode().IsRegular() {
		return nil, newError(ErrNotRegularFile, "input", "input_not_regular_file",
			fmt.Sprintf("opened input is not a regular file (mode %s)", finfo.Mode()), nil)
	}
	if finfo.Size() > maxBytes {
		return nil, newError(ErrInputTooLarge, "input", "input_too_large",
			fmt.Sprintf("input size %d bytes exceeds limit of %d bytes", finfo.Size(), maxBytes), nil)
	}

	// Read at most maxBytes+1 to detect concurrent growth beyond the limit.
	h := sha256.New()
	limited := io.LimitReader(f, maxBytes+1)
	data, err := io.ReadAll(io.TeeReader(limited, h))
	if err != nil {
		return nil, newError(ErrInternal, "input", "input_read_failed",
			"failed to read staged input file", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, newError(ErrInputTooLarge, "input", "input_too_large",
			fmt.Sprintf("input size exceeds limit of %d bytes", maxBytes), nil)
	}

	actualSHA256 := hex.EncodeToString(h.Sum(nil))
	if actualSHA256 != expectedSHA256 {
		return nil, newError(ErrInputHashMismatch, "input", "input_hash_mismatch",
			fmt.Sprintf("computed SHA-256 %s does not match expected %s", actualSHA256, expectedSHA256), nil)
	}

	return &LoadedInput{
		Data:   data,
		SHA256: actualSHA256,
		Size:   int64(len(data)),
	}, nil
}

// DecodeLedgerCloseMeta decodes raw XDR bytes into xdr.LedgerCloseMeta using
// the official Stellar Go SDK's SafeUnmarshal (which rejects truncated XDR and
// trailing bytes) and validates the outer and inner union variants before any
// SDK accessor is invoked.
func DecodeLedgerCloseMeta(data []byte) (*DecodedLedgerMeta, error) {
	if len(data) == 0 {
		return nil, newError(ErrXDRDecodeFailed, "decode", "empty_xdr_input",
			"input XDR payload is empty", nil)
	}

	var lcm xdr.LedgerCloseMeta
	if err := xdr.SafeUnmarshal(data, &lcm); err != nil {
		return nil, newError(ErrXDRDecodeFailed, "decode", "xdr_unmarshal_failed",
			"failed to unmarshal LedgerCloseMeta XDR", err)
	}

	if err := validateLedgerCloseMetaStructure(&lcm); err != nil {
		return nil, err
	}

	return &DecodedLedgerMeta{
		Meta:         lcm,
		OuterVariant: lcm.V,
	}, nil
}

// validateLedgerCloseMetaStructure checks outer LedgerCloseMeta.V and inner
// transaction-set/processing invariants so downstream SDK accessors (which
// panic on unsupported variants or mismatched slices) cannot panic.
func validateLedgerCloseMetaStructure(lcm *xdr.LedgerCloseMeta) error {
	switch lcm.V {
	case 0:
		if lcm.V0 == nil {
			return newError(ErrXDRDecodeFailed, "decode", "nil_lcm_v0",
				"LedgerCloseMeta.V is 0 but V0 payload is nil", nil)
		}
		if len(lcm.V0.TxSet.Txs) != len(lcm.V0.TxProcessing) {
			return newError(ErrXDRDecodeFailed, "decode", "tx_count_mismatch",
				fmt.Sprintf("V0 TxSet has %d transactions but TxProcessing has %d",
					len(lcm.V0.TxSet.Txs), len(lcm.V0.TxProcessing)), nil)
		}
		return nil

	case 1:
		if lcm.V1 == nil {
			return newError(ErrXDRDecodeFailed, "decode", "nil_lcm_v1",
				"LedgerCloseMeta.V is 1 but V1 payload is nil", nil)
		}
		return validateGeneralizedTxSet(&lcm.V1.TxSet, len(lcm.V1.TxProcessing), 1)

	case 2:
		if lcm.V2 == nil {
			return newError(ErrXDRDecodeFailed, "decode", "nil_lcm_v2",
				"LedgerCloseMeta.V is 2 but V2 payload is nil", nil)
		}
		return validateGeneralizedTxSet(&lcm.V2.TxSet, len(lcm.V2.TxProcessing), 2)

	default:
		return newError(ErrUnsupportedXDRVariant, "decode", "unsupported_lcm_variant",
			fmt.Sprintf("unsupported LedgerCloseMeta outer variant V=%d (supported: 0, 1, 2)", lcm.V), nil)
	}
}

func validateGeneralizedTxSet(txSet *xdr.GeneralizedTransactionSet, txProcessingCount int, lcmVariant int32) error {
	if txSet.V != 1 || txSet.V1TxSet == nil {
		return newError(ErrUnsupportedXDRVariant, "decode", "unsupported_tx_set_variant",
			fmt.Sprintf("LedgerCloseMeta V%d has unsupported GeneralizedTransactionSet.V=%d", lcmVariant, txSet.V), nil)
	}

	envelopeCount := 0
	for i, phase := range txSet.V1TxSet.Phases {
		switch phase.V {
		case 0:
			if phase.V0Components == nil {
				return newError(ErrXDRDecodeFailed, "decode", "nil_phase_v0_components",
					fmt.Sprintf("TransactionPhase[%d].V is 0 but V0Components is nil", i), nil)
			}
			for _, comp := range *phase.V0Components {
				if comp.TxsMaybeDiscountedFee == nil {
					return newError(ErrXDRDecodeFailed, "decode", "nil_txs_maybe_discounted_fee",
						fmt.Sprintf("TransactionPhase[%d] component has nil TxsMaybeDiscountedFee", i), nil)
				}
				envelopeCount += len(comp.TxsMaybeDiscountedFee.Txs)
			}
		case 1:
			if phase.ParallelTxsComponent == nil {
				return newError(ErrXDRDecodeFailed, "decode", "nil_parallel_txs_component",
					fmt.Sprintf("TransactionPhase[%d].V is 1 but ParallelTxsComponent is nil", i), nil)
			}
			for _, stage := range phase.ParallelTxsComponent.ExecutionStages {
				for _, cluster := range stage {
					envelopeCount += len(cluster)
				}
			}
		default:
			return newError(ErrUnsupportedXDRVariant, "decode", "unsupported_tx_phase_variant",
				fmt.Sprintf("TransactionPhase[%d] has unsupported variant V=%d", i, phase.V), nil)
		}
	}

	if envelopeCount != txProcessingCount {
		return newError(ErrXDRDecodeFailed, "decode", "tx_count_mismatch",
			fmt.Sprintf("V%d TxSet has %d transactions but TxProcessing has %d",
				lcmVariant, envelopeCount, txProcessingCount), nil)
	}

	return nil
}
