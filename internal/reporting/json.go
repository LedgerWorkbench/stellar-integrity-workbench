// Package reporting renders schema-validated JSON reports and deterministic
// Markdown reports from a verified Unit 7 evidence.Manifest.
package reporting

import (
	"encoding/json"
	"fmt"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/evidence"
)

// RenderJSON serializes a validated evidence.Manifest as deterministic,
// indented JSON (with a trailing newline) and verifies the rendered bytes
// against the embedded evidence-bundle JSON Schema and manifest invariants.
func RenderJSON(manifest *evidence.Manifest) ([]byte, error) {
	if manifest == nil {
		return nil, fmt.Errorf("cannot render JSON report: manifest must not be nil")
	}
	if err := evidence.ValidateManifest(manifest); err != nil {
		return nil, fmt.Errorf("cannot render JSON report: %w", err)
	}
	if err := validateReportSafety(manifest); err != nil {
		return nil, err
	}

	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal JSON report: %w", err)
	}
	out = append(out, '\n')

	if _, err := evidence.DecodeAndValidateManifest(out); err != nil {
		return nil, fmt.Errorf("rendered JSON report failed schema validation: %w", err)
	}
	return out, nil
}
