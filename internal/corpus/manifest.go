package corpus

import (
	"fmt"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
)

// Manifest represents the JSON schema representation of a corpus case manifest.
type Manifest struct {
	SchemaVersion         string                `json:"schema_version"`
	CaseID                string                `json:"case_id"`
	Title                 string                `json:"title"`
	Summary               string                `json:"summary"`
	Classification        Classification        `json:"classification"`
	Source                Source                `json:"source"`
	Network               Network               `json:"network"`
	Input                 Input                 `json:"input"`
	Expectations          []Expectation         `json:"expectations"`
	ImplementationContext ImplementationContext `json:"implementation_context"`
	Limitations           []string              `json:"limitations"`
}

// Source represents the provenance metadata in a manifest.
type Source struct {
	Kind              string  `json:"kind"`
	URL               *string `json:"url"`
	Repository        *string `json:"repository"`
	IssueNumber       *int    `json:"issue_number"`
	PullRequestNumber *int    `json:"pull_request_number"`
	RetrievedAt       *string `json:"retrieved_at"`
	Notes             string  `json:"notes"`
}

// Network represents the network parameters and ledger sequence in a manifest.
type Network struct {
	Name            string `json:"name"`
	ProtocolVersion *int   `json:"protocol_version"`
	LedgerSequence  *int64 `json:"ledger_sequence"`
}

// Input represents the immutable input object metadata in a manifest.
type Input struct {
	Format     string `json:"format"`
	Encoding   string `json:"encoding"`
	SHA256     string `json:"sha256"`
	ByteLength int64  `json:"byte_length"`
	ObjectPath string `json:"object_path"`
}

// Expectation declares an individual assertion condition in a manifest.
type Expectation struct {
	AssertionID string `json:"assertion_id"`
	Kind        string `json:"kind"`
	Expected    any    `json:"expected"`
	Description string `json:"description"`
}

// ImplementationContext captures surrounding software versions in a manifest.
type ImplementationContext struct {
	StellarCoreVersion *string           `json:"stellar_core_version"`
	SDKVersions        map[string]string `json:"sdk_versions"`
	IndexerVersion     *string           `json:"indexer_version"`
}

// ToDomain converts a validated manifest into a typed domain.Case model.
func (m *Manifest) ToDomain() (*domain.Case, error) {
	if m == nil {
		return nil, fmt.Errorf("cannot convert nil manifest to domain case")
	}

	var retrievedAt *time.Time
	if m.Source.RetrievedAt != nil {
		parsed, err := time.Parse(time.RFC3339, *m.Source.RetrievedAt)
		if err != nil {
			return nil, fmt.Errorf("invalid retrieved_at timestamp %q: %w", *m.Source.RetrievedAt, err)
		}
		retrievedAt = &parsed
	}

	expectations := make([]domain.AssertionExpectation, len(m.Expectations))
	for i, exp := range m.Expectations {
		kind := domain.AssertionKind(exp.Kind)
		if err := kind.Validate(); err != nil {
			return nil, fmt.Errorf("assertion %q has invalid kind: %w", exp.AssertionID, err)
		}

		var typedExpected any
		switch kind {
		case domain.AssertionKindLedgerSequenceEquals:
			n, err := domain.ParseExactInteger(exp.Expected)
			if err != nil {
				return nil, fmt.Errorf("assertion %q has invalid expected ledger sequence: %w", exp.AssertionID, err)
			}
			if n < 0 || n > domain.MaxStellarSequence {
				return nil, fmt.Errorf("assertion %q expected ledger sequence %d out of range [0, %d]", exp.AssertionID, n, domain.MaxStellarSequence)
			}
			typedExpected = n

		case domain.AssertionKindTransactionCountEquals:
			n, err := domain.ParseExactInteger(exp.Expected)
			if err != nil {
				return nil, fmt.Errorf("assertion %q has invalid expected transaction count: %w", exp.AssertionID, err)
			}
			if n < 0 || n > domain.MaxStellarTransactionCount {
				return nil, fmt.Errorf("assertion %q expected transaction count %d out of range [0, %d]", exp.AssertionID, n, domain.MaxStellarTransactionCount)
			}
			typedExpected = n

		case domain.AssertionKindTransactionHashPresent,
			domain.AssertionKindDecodedTypeEquals,
			domain.AssertionKindInputHashEquals:
			s, ok := exp.Expected.(string)
			if !ok {
				return nil, fmt.Errorf("assertion %q expected string value for %s, got %T", exp.AssertionID, kind, exp.Expected)
			}
			typedExpected = s

		default:
			typedExpected = exp.Expected
		}

		expectations[i] = domain.AssertionExpectation{
			AssertionID: exp.AssertionID,
			Kind:        kind,
			Expected:    typedExpected,
			Description: exp.Description,
		}
	}

	sdkVersions := make(map[string]string, len(m.ImplementationContext.SDKVersions))
	for k, v := range m.ImplementationContext.SDKVersions {
		sdkVersions[k] = v
	}

	limitations := make([]string, len(m.Limitations))
	copy(limitations, m.Limitations)

	if err := ValidateObjectPath(m.Input.ObjectPath); err != nil {
		return nil, fmt.Errorf("invalid input.object_path: %w", err)
	}

	return &domain.Case{
		SchemaVersion:  m.SchemaVersion,
		CaseID:         m.CaseID,
		Title:          m.Title,
		Summary:        m.Summary,
		Classification: domain.Classification(m.Classification),
		Source: domain.Source{
			Kind:              m.Source.Kind,
			URL:               m.Source.URL,
			Repository:        m.Source.Repository,
			IssueNumber:       m.Source.IssueNumber,
			PullRequestNumber: m.Source.PullRequestNumber,
			RetrievedAt:       retrievedAt,
			Notes:             m.Source.Notes,
		},
		Network: domain.Network{
			Name:            m.Network.Name,
			ProtocolVersion: m.Network.ProtocolVersion,
			LedgerSequence:  m.Network.LedgerSequence,
		},
		Input: domain.Input{
			Format:     m.Input.Format,
			Encoding:   m.Input.Encoding,
			SHA256:     m.Input.SHA256,
			ByteLength: m.Input.ByteLength,
			ObjectPath: m.Input.ObjectPath,
		},
		Expectations: expectations,
		ImplementationContext: domain.ImplementationContext{
			StellarCoreVersion: m.ImplementationContext.StellarCoreVersion,
			SDKVersions:        sdkVersions,
			IndexerVersion:     m.ImplementationContext.IndexerVersion,
		},
		Limitations: limitations,
	}, nil
}
