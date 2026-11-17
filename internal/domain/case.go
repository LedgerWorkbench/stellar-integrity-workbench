package domain

import (
	"time"
)

// Classification represents the evidence classification tier of a case.
type Classification string

const (
	ClassificationVerifiedLedgerSample Classification = "verified_ledger_sample"
	ClassificationHistoricalRegression Classification = "historical_regression"
	ClassificationReproducedRegression Classification = "reproduced_regression"
	ClassificationSyntheticFixture     Classification = "synthetic_fixture"
	ClassificationUnverified           Classification = "unverified"
)

// Source describes the origin and provenance of the corpus case.
type Source struct {
	Kind              string     `json:"kind"`
	URL               *string    `json:"url,omitempty"`
	Repository        *string    `json:"repository,omitempty"`
	IssueNumber       *int       `json:"issue_number,omitempty"`
	PullRequestNumber *int       `json:"pull_request_number,omitempty"`
	RetrievedAt       *time.Time `json:"retrieved_at,omitempty"`
	Notes             string     `json:"notes"`
}

// Network describes the Stellar network context for the sample.
// Unknown values are explicitly represented as nil pointers, never invented defaults.
type Network struct {
	Name            string `json:"name"`
	ProtocolVersion *int   `json:"protocol_version"`
	LedgerSequence  *int64 `json:"ledger_sequence"`
}

// Input describes the immutable input object reference and integrity hashes.
type Input struct {
	Format     string `json:"format"`
	Encoding   string `json:"encoding"`
	SHA256     string `json:"sha256"`
	ByteLength int64  `json:"byte_length"`
	ObjectPath string `json:"object_path"`
}

// ImplementationContext captures software versions relevant to the case.
// Missing versions are represented as nil, never default version strings.
type ImplementationContext struct {
	StellarCoreVersion *string           `json:"stellar_core_version"`
	SDKVersions        map[string]string `json:"sdk_versions"`
	IndexerVersion     *string           `json:"indexer_version"`
}

// ImplementationTarget records a software implementation name and version.
type ImplementationTarget struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// DifferentialEvidence captures proof of differential behaviour between versions.
type DifferentialEvidence struct {
	EvidenceRef string `json:"evidence_ref"`
	SHA256      string `json:"sha256"`
	Notes       string `json:"notes,omitempty"`
}

// ReproductionEvidence records verified differential evidence required for reproduced_regression cases.
type ReproductionEvidence struct {
	AffectedImplementation ImplementationTarget `json:"affected_implementation"`
	FixedImplementation    ImplementationTarget `json:"fixed_implementation"`
	DifferentialEvidence   DifferentialEvidence `json:"differential_evidence"`
}

// Case represents a validated, immutable corpus case domain model.
type Case struct {
	SchemaVersion         string                 `json:"schema_version"`
	CaseID                string                 `json:"case_id"`
	Title                 string                 `json:"title"`
	Summary               string                 `json:"summary"`
	Classification        Classification         `json:"classification"`
	Source                Source                 `json:"source"`
	Network               Network                `json:"network"`
	Input                 Input                  `json:"input"`
	Expectations          []AssertionExpectation `json:"expectations"`
	ImplementationContext ImplementationContext  `json:"implementation_context"`
	Limitations           []string               `json:"limitations"`
	Reproduction          *ReproductionEvidence  `json:"reproduction,omitempty"`
}
