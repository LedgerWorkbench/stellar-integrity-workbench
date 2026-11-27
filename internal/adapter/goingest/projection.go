package goingest

import (
	"fmt"

	ingestledger "github.com/stellar/go-stellar-sdk/ingest/ledger"
)

const (
	// ProjectionVersion is the schema version of the go-ingest semantic projection.
	ProjectionVersion = "1"

	// MaxProjectedTxHashes bounds the number of transaction hashes included in
	// SemanticProjection.Transactions.Hashes to keep semantic outputs bounded.
	MaxProjectedTxHashes = 256
)

// SemanticProjection is the typed, deterministic semantic summary extracted
// from a decoded xdr.LedgerCloseMeta using official Stellar Go SDK accessors
// and ingest/ledger helpers.
type SemanticProjection struct {
	ProjectionVersion string                 `json:"projection_version"`
	XDRType           string                 `json:"xdr_type"`
	OuterVariant      int32                  `json:"outer_variant"`
	Ledger            LedgerProjection       `json:"ledger"`
	Transactions      TransactionsProjection `json:"transactions"`
}

// LedgerProjection holds verified ledger-header metadata.
type LedgerProjection struct {
	Sequence           uint32 `json:"sequence"`
	ProtocolVersion    uint32 `json:"protocol_version"`
	CloseTimeUnix      int64  `json:"close_time_unix"`
	LedgerHash         string `json:"ledger_hash"`
	PreviousLedgerHash string `json:"previous_ledger_hash"`
}

// TransactionsProjection holds verified transaction counts and bounded
// transaction hashes in processing order (matching TxProcessing[i]).
type TransactionsProjection struct {
	Total           uint32   `json:"total"`
	Successful      uint32   `json:"successful"`
	Failed          uint32   `json:"failed"`
	Hashes          []string `json:"hashes"`
	HashesTruncated bool     `json:"hashes_truncated"`
}

// BuildSemanticProjection constructs a deterministic SemanticProjection from
// a validated DecodedLedgerMeta using MaxProjectedTxHashes as the hash bound.
func BuildSemanticProjection(decoded *DecodedLedgerMeta) (*SemanticProjection, error) {
	return BuildSemanticProjectionWithLimit(decoded, MaxProjectedTxHashes)
}

// BuildSemanticProjectionWithLimit constructs a deterministic SemanticProjection
// with an explicit upper bound on projected transaction hashes.
func BuildSemanticProjectionWithLimit(decoded *DecodedLedgerMeta, maxHashes int) (proj *SemanticProjection, err error) {
	if decoded == nil {
		return nil, newError(ErrProjectionFailed, "projection", "nil_decoded_meta",
			"decoded LedgerCloseMeta must not be nil", nil)
	}
	if maxHashes < 0 {
		return nil, newError(ErrProjectionFailed, "projection", "invalid_hash_limit",
			fmt.Sprintf("maxHashes must be non-negative, got %d", maxHashes), nil)
	}

	// Defensive boundary: convert any unexpected SDK panic into a structured error.
	defer func() {
		if r := recover(); r != nil {
			proj = nil
			err = newError(ErrProjectionFailed, "projection", "sdk_panic_recovered",
				fmt.Sprintf("unexpected panic while projecting LedgerCloseMeta: %v", r), nil)
		}
	}()

	if err := validateLedgerCloseMetaStructure(&decoded.Meta); err != nil {
		return nil, err
	}

	successCount, totalCount := ingestledger.TransactionCounts(decoded.Meta)
	processingCount := decoded.Meta.CountTransactions()
	if int(totalCount) != processingCount {
		return nil, newError(ErrProjectionFailed, "projection", "tx_count_inconsistent",
			fmt.Sprintf("TransactionCounts reported %d transactions but CountTransactions reported %d",
				totalCount, processingCount), nil)
	}
	if successCount > totalCount {
		return nil, newError(ErrProjectionFailed, "projection", "tx_success_exceeds_total",
			fmt.Sprintf("successful transaction count %d exceeds total %d", successCount, totalCount), nil)
	}

	failedCount := totalCount - successCount

	limit := processingCount
	truncated := false
	if limit > maxHashes {
		limit = maxHashes
		truncated = true
	}

	hashes := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		hashes = append(hashes, decoded.Meta.TransactionHash(i).HexString())
	}

	return &SemanticProjection{
		ProjectionVersion: ProjectionVersion,
		XDRType:           "LedgerCloseMeta",
		OuterVariant:      decoded.OuterVariant,
		Ledger: LedgerProjection{
			Sequence:           decoded.Meta.LedgerSequence(),
			ProtocolVersion:    decoded.Meta.ProtocolVersion(),
			CloseTimeUnix:      decoded.Meta.LedgerCloseTime(),
			LedgerHash:         decoded.Meta.LedgerHash().HexString(),
			PreviousLedgerHash: decoded.Meta.PreviousLedgerHash().HexString(),
		},
		Transactions: TransactionsProjection{
			Total:           totalCount,
			Successful:      successCount,
			Failed:          failedCount,
			Hashes:          hashes,
			HashesTruncated: truncated,
		},
	}, nil
}
