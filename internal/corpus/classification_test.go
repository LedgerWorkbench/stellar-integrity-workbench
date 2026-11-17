package corpus_test

import (
	"testing"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/corpus"
)

func TestClassification_Validation(t *testing.T) {
	all := corpus.AllClassifications()
	if len(all) != 5 {
		t.Fatalf("expected 5 classifications, got %d", len(all))
	}

	for _, c := range all {
		if !c.IsValid() {
			t.Errorf("expected classification %q to be valid", c)
		}
		if err := c.Validate(); err != nil {
			t.Errorf("expected Validate() nil for %q, got: %v", c, err)
		}
	}

	// Authentic source policy check
	if !corpus.ClassificationVerifiedLedgerSample.RequiresAuthenticSource() {
		t.Errorf("verified_ledger_sample must require authentic source")
	}
	if corpus.ClassificationSyntheticFixture.RequiresAuthenticSource() {
		t.Errorf("synthetic_fixture must NOT require authentic source")
	}

	// Defect documentation policy check
	if !corpus.ClassificationHistoricalRegression.RequiresDefectDocumentation() {
		t.Errorf("historical_regression must require defect documentation")
	}
	if !corpus.ClassificationReproducedRegression.RequiresDefectDocumentation() {
		t.Errorf("reproduced_regression must require defect documentation")
	}
	if corpus.ClassificationVerifiedLedgerSample.RequiresDefectDocumentation() {
		t.Errorf("verified_ledger_sample does not inherently require defect documentation")
	}

	// Invalid classification
	invalid := corpus.Classification("production_sample")
	if invalid.IsValid() {
		t.Errorf("expected %q to be invalid", invalid)
	}
	if err := invalid.Validate(); err == nil {
		t.Errorf("expected Validate() error for %q", invalid)
	}
}
