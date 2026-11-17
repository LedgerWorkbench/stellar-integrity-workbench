package corpus

import (
	"fmt"
)

// Classification represents the evidence classification tier of a corpus case.
//
// Each corpus case must have exactly one classification that truthfully reflects
// its origin and verification status. Classifications are not interchangeable
// quality badges; a synthetic case must never be described as captured public
// ledger data.
type Classification string

const (
	// ClassificationVerifiedLedgerSample indicates authentic ledger data obtained
	// from a traceable source (such as a Galexie archive, captive core, or RPC)
	// and independently checked for provenance and integrity.
	ClassificationVerifiedLedgerSample Classification = "verified_ledger_sample"

	// ClassificationHistoricalRegression indicates a case associated with a
	// documented historical implementation defect. The exact input and source
	// of the expected behaviour must be recorded.
	ClassificationHistoricalRegression Classification = "historical_regression"

	// ClassificationReproducedRegression indicates a historical or newly discovered
	// defect independently reproduced by this Workbench with captured execution evidence.
	ClassificationReproducedRegression Classification = "reproduced_regression"

	// ClassificationSyntheticFixture indicates generated test input or a manually
	// constructed test case. It must never be represented as captured public ledger data.
	ClassificationSyntheticFixture Classification = "synthetic_fixture"

	// ClassificationUnverified indicates source or expected behaviour is incomplete.
	// This classification cannot be promoted to verified or reproduced by automated
	// execution alone.
	ClassificationUnverified Classification = "unverified"
)

// AllClassifications returns all recognized corpus classification tiers.
func AllClassifications() []Classification {
	return []Classification{
		ClassificationVerifiedLedgerSample,
		ClassificationHistoricalRegression,
		ClassificationReproducedRegression,
		ClassificationSyntheticFixture,
		ClassificationUnverified,
	}
}

// IsValid reports whether the classification is one of the recognized types.
func (c Classification) IsValid() bool {
	switch c {
	case ClassificationVerifiedLedgerSample,
		ClassificationHistoricalRegression,
		ClassificationReproducedRegression,
		ClassificationSyntheticFixture,
		ClassificationUnverified:
		return true
	default:
		return false
	}
}

// RequiresAuthenticSource reports whether this classification mandates an authentic
// traceable ledger source reference (e.g. verified ledger samples).
func (c Classification) RequiresAuthenticSource() bool {
	return c == ClassificationVerifiedLedgerSample
}

// RequiresDefectDocumentation reports whether this classification mandates a
// reference to a documented historical or reproduced defect (issue/PR/upstream ref).
func (c Classification) RequiresDefectDocumentation() bool {
	return c == ClassificationHistoricalRegression || c == ClassificationReproducedRegression
}

// Validate checks that the classification is valid.
func (c Classification) Validate() error {
	if !c.IsValid() {
		return fmt.Errorf("unknown corpus classification %q; must be one of: %v", c, AllClassifications())
	}
	return nil
}
