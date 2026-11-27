package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/corpus"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/schemas"
)

const (
	// SchemaVersion is the current evidence bundle manifest schema version.
	SchemaVersion = "1.0.0"

	// SupportedMajorVersionPrefix is the supported major schema version prefix.
	SupportedMajorVersionPrefix = "1."

	// MaxManifestBytes bounds the size of manifest.json read by VerifyBundle (1 MiB).
	MaxManifestBytes int64 = 1 * 1024 * 1024

	// ManifestFilename is the fixed manifest filename inside a run bundle.
	ManifestFilename = "manifest.json"

	// ArtifactsDirname is the fixed artifacts subdirectory inside a run bundle.
	ArtifactsDirname = "artifacts"

	// Artifact roles.
	ArtifactRoleAdapterStdout      = "adapter_stdout"
	ArtifactRoleSemanticProjection = "semantic_projection"
	ArtifactRoleAssertions         = "assertions"
	ArtifactRoleInputCopy          = "input_copy"
)

var (
	runIDPattern        = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	artifactPathPattern = regexp.MustCompile(`^artifacts/[a-zA-Z0-9_-][a-zA-Z0-9_.-]*$`)

	evidenceSchemaOnce sync.Once
	evidenceSchema     *jsonschema.Schema
	evidenceSchemaErr  error
)

func getEvidenceSchema() (*jsonschema.Schema, error) {
	evidenceSchemaOnce.Do(func() {
		c := jsonschema.NewCompiler()
		c.AssertFormat()
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemas.EvidenceBundleSchemaJSON))
		if err != nil {
			evidenceSchemaErr = fmt.Errorf("parse evidence-bundle schema: %w", err)
			return
		}
		if err := c.AddResource("evidence-bundle.schema.json", doc); err != nil {
			evidenceSchemaErr = fmt.Errorf("add evidence-bundle schema resource: %w", err)
			return
		}
		evidenceSchema, evidenceSchemaErr = c.Compile("evidence-bundle.schema.json")
	})
	return evidenceSchema, evidenceSchemaErr
}

// Manifest is the versioned, self-contained evidence manifest stored at
// <evidence-root>/<run-id>/manifest.json.
type Manifest struct {
	SchemaVersion  string                   `json:"schema_version"`
	RunID          string                   `json:"run_id"`
	CaseID         string                   `json:"case_id"`
	Classification domain.Classification    `json:"classification"`
	Provenance     Provenance               `json:"provenance"`
	Input          InputEvidence            `json:"input"`
	Worker         WorkerMetadata           `json:"worker"`
	Adapter        AdapterMetadata          `json:"adapter"`
	Run            RunMetadata              `json:"run"`
	Assertions     []domain.AssertionResult `json:"assertions"`
	Diagnostics    DiagnosticsEvidence      `json:"diagnostics"`
	Artifacts      []ArtifactRef            `json:"artifacts"`
}

// Provenance records the origin metadata of the corpus case.
type Provenance struct {
	Kind              string  `json:"kind"`
	URL               *string `json:"url"`
	Repository        *string `json:"repository"`
	IssueNumber       *int    `json:"issue_number"`
	PullRequestNumber *int    `json:"pull_request_number"`
	RetrievedAt       *string `json:"retrieved_at"`
	Notes             string  `json:"notes"`
}

// InputEvidence records verified input integrity and corpus-relative reference.
type InputEvidence struct {
	SHA256          string  `json:"sha256"`
	ByteLength      int64   `json:"byte_length"`
	Format          string  `json:"format"`
	Encoding        string  `json:"encoding"`
	ObjectPath      string  `json:"object_path"`
	DetectedType    *string `json:"detected_type"`
	NetworkName     string  `json:"network_name"`
	LedgerSequence  *int64  `json:"ledger_sequence"`
	ProtocolVersion *int    `json:"protocol_version"`
}

// WorkerMetadata records trusted build information for the worker process.
// Genuinely unavailable values (such as CommitSHA) are represented as nil.
type WorkerMetadata struct {
	Version   string  `json:"version"`
	CommitSHA *string `json:"commit_sha"`
}

// AdapterMetadata identifies the adapter and pinned SDK used for the run.
type AdapterMetadata struct {
	AdapterID      string `json:"adapter_id"`
	AdapterVersion string `json:"adapter_version"`
	SDKVersion     string `json:"sdk_version"`
}

// RunMetadata records execution lifecycle timestamps, state, and conformance outcome.
// AdapterDurationMs records the duration self-reported by the adapter;
// HostDurationMs records the process wall-clock duration measured by the host runner.
type RunMetadata struct {
	StartedAt              string             `json:"started_at"`
	CompletedAt            string             `json:"completed_at"`
	State                  domain.RunState    `json:"state"`
	AdapterExecutionStatus *string            `json:"adapter_execution_status"`
	AdapterExitCode        *int               `json:"adapter_exit_code"`
	AdapterDurationMs      *int64             `json:"adapter_duration_ms"`
	HostExitCode           *int               `json:"host_exit_code"`
	HostDurationMs         *int64             `json:"host_duration_ms"`
	Outcome                *domain.RunOutcome `json:"outcome"`
}

// DiagnosticsEvidence records safe structured diagnostics and stderr bounds.
type DiagnosticsEvidence struct {
	AdapterDiagnostics     []adapter.ResultDiagnostic `json:"adapter_diagnostics"`
	Limitations            []string                   `json:"limitations"`
	AdapterStderrBytesSeen *int64                     `json:"adapter_stderr_bytes_seen"`
	AdapterStderrTruncated *bool                      `json:"adapter_stderr_truncated"`
}

// ArtifactRef records the relative path, role, media type, size, and SHA-256
// of a stored artifact inside the run bundle.
type ArtifactRef struct {
	Path       string `json:"path"`
	Role       string `json:"role"`
	MediaType  string `json:"media_type"`
	ByteLength int64  `json:"byte_length"`
	SHA256     string `json:"sha256"`
}

// ValidateRunID checks that runID is a safe single directory name.
func ValidateRunID(runID string) error {
	if strings.TrimSpace(runID) == "" {
		return fmt.Errorf("%w: run_id must not be empty", ErrUnsafePath)
	}
	if len(runID) > 256 {
		return fmt.Errorf("%w: run_id exceeds 256 characters", ErrUnsafePath)
	}
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("%w: run_id %q must match ^[a-zA-Z0-9_-]+$", ErrUnsafePath, runID)
	}
	return nil
}

// ValidateArtifactPath checks that path is a safe relative path under artifacts/.
func ValidateArtifactPath(path string) error {
	if strings.Contains(path, "..") || strings.Contains(path, "\\") || strings.HasPrefix(path, "/") {
		return fmt.Errorf("%w: unsafe artifact path %q", ErrUnsafePath, path)
	}
	if !artifactPathPattern.MatchString(path) {
		return fmt.Errorf("%w: artifact path %q must be a safe relative path under artifacts/", ErrUnsafePath, path)
	}
	return nil
}

// DecodeAndValidateManifest strictly decodes manifest JSON (rejecting unknown
// fields and trailing JSON values), validates it against the embedded Draft
// 2020-12 JSON Schema, and checks typed domain invariants.
func DecodeAndValidateManifest(data []byte) (*Manifest, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%w: manifest JSON is empty", ErrManifestInvalid)
	}

	// Single-document JSON parse with UseNumber for schema validation.
	var raw any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON: %v", ErrManifestInvalid, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%w: trailing JSON value after manifest", ErrManifestInvalid)
		}
		return nil, fmt.Errorf("%w: trailing data after manifest: %v", ErrManifestInvalid, err)
	}

	// Check schema_version major version before schema validation for a clear sentinel.
	if obj, ok := raw.(map[string]any); ok {
		if sv, ok := obj["schema_version"].(string); ok {
			if !strings.HasPrefix(sv, SupportedMajorVersionPrefix) {
				return nil, fmt.Errorf("%w: schema_version %q is not supported (expected major 1.x)",
					ErrUnsupportedSchemaVersion, sv)
			}
		}
	}

	sch, err := getEvidenceSchema()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if err := sch.Validate(raw); err != nil {
		return nil, fmt.Errorf("%w: schema validation failed: %v", ErrManifestInvalid, err)
	}

	// Strict typed decode with DisallowUnknownFields and UseNumber.
	var m Manifest
	tdec := json.NewDecoder(bytes.NewReader(data))
	tdec.DisallowUnknownFields()
	tdec.UseNumber()
	if err := tdec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: typed decode failed: %v", ErrManifestInvalid, err)
	}

	if err := ValidateManifest(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ValidateManifest checks typed structural and semantic invariants on m.
func ValidateManifest(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("%w: manifest is nil", ErrManifestInvalid)
	}
	if !strings.HasPrefix(m.SchemaVersion, SupportedMajorVersionPrefix) {
		return fmt.Errorf("%w: schema_version %q is not supported", ErrUnsupportedSchemaVersion, m.SchemaVersion)
	}
	if err := ValidateRunID(m.RunID); err != nil {
		return fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if strings.TrimSpace(m.CaseID) == "" {
		return fmt.Errorf("%w: case_id must not be empty", ErrManifestInvalid)
	}
	if err := corpus.ValidateObjectPath(m.Input.ObjectPath); err != nil {
		return fmt.Errorf("%w: invalid input.object_path: %v", ErrManifestInvalid, err)
	}
	if !IsValidSHA256(m.Input.SHA256) {
		return fmt.Errorf("%w: invalid input.sha256 %q", ErrManifestInvalid, m.Input.SHA256)
	}
	if m.Input.ByteLength < 0 {
		return fmt.Errorf("%w: negative input.byte_length %d", ErrManifestInvalid, m.Input.ByteLength)
	}

	if m.Provenance.RetrievedAt != nil {
		if _, err := parseUTCRFC3339("provenance.retrieved_at", *m.Provenance.RetrievedAt); err != nil {
			return err
		}
	}

	startedAt, err := parseUTCRFC3339("run.started_at", m.Run.StartedAt)
	if err != nil {
		return err
	}
	completedAt, err := parseUTCRFC3339("run.completed_at", m.Run.CompletedAt)
	if err != nil {
		return err
	}
	if completedAt.Before(startedAt) {
		return fmt.Errorf("%w: run.completed_at (%s) precedes run.started_at (%s)",
			ErrManifestInvalid, m.Run.CompletedAt, m.Run.StartedAt)
	}
	if !m.Run.State.IsTerminal() {
		return fmt.Errorf("%w: run.state %q must be terminal (completed, failed, or cancelled)",
			ErrManifestInvalid, m.Run.State)
	}
	if m.Run.State == domain.RunStateCompleted {
		if m.Run.Outcome == nil || !m.Run.Outcome.IsValid() {
			return fmt.Errorf("%w: completed run must have a valid outcome", ErrManifestInvalid)
		}
		if len(m.Assertions) == 0 {
			return fmt.Errorf("%w: completed run manifest must contain at least one assertion", ErrManifestInvalid)
		}
	} else if m.Run.Outcome != nil {
		return fmt.Errorf("%w: non-completed run (state=%s) must not have an outcome (%s)",
			ErrManifestInvalid, m.Run.State, *m.Run.Outcome)
	}

	seenAssertionIDs := make(map[string]struct{}, len(m.Assertions))
	for i, a := range m.Assertions {
		if strings.TrimSpace(a.AssertionID) == "" {
			return fmt.Errorf("%w: assertion[%d] has empty assertion_id", ErrManifestInvalid, i)
		}
		if _, dup := seenAssertionIDs[a.AssertionID]; dup {
			return fmt.Errorf("%w: duplicate assertion_id %q", ErrManifestInvalid, a.AssertionID)
		}
		seenAssertionIDs[a.AssertionID] = struct{}{}
		if err := a.Kind.Validate(); err != nil {
			return fmt.Errorf("%w: assertion %q: %v", ErrManifestInvalid, a.AssertionID, err)
		}
		if err := a.Status.Validate(); err != nil {
			return fmt.Errorf("%w: assertion %q: %v", ErrManifestInvalid, a.AssertionID, err)
		}
	}

	if m.Run.State == domain.RunStateCompleted {
		derivedOutcome, err := domain.DeriveOutcome(m.Assertions)
		if err != nil {
			return fmt.Errorf("%w: derive outcome from manifest assertions: %v", ErrManifestInvalid, err)
		}
		if derivedOutcome != *m.Run.Outcome {
			return fmt.Errorf("%w: manifest run.outcome %q does not match outcome %q derived from assertions",
				ErrManifestInvalid, *m.Run.Outcome, derivedOutcome)
		}
	}

	seenArtifacts := make(map[string]struct{}, len(m.Artifacts))
	roleCounts := make(map[string]int, len(m.Artifacts))
	for i, art := range m.Artifacts {
		if err := ValidateArtifactPath(art.Path); err != nil {
			return fmt.Errorf("%w: artifact[%d]: %v", ErrManifestInvalid, i, err)
		}
		if _, dup := seenArtifacts[art.Path]; dup {
			return fmt.Errorf("%w: duplicate artifact path %q", ErrManifestInvalid, art.Path)
		}
		seenArtifacts[art.Path] = struct{}{}

		if !isValidArtifactRole(art.Role) {
			return fmt.Errorf("%w: artifact %q has invalid role %q", ErrManifestInvalid, art.Path, art.Role)
		}
		roleCounts[art.Role]++
		if roleCounts[art.Role] > 1 {
			return fmt.Errorf("%w: duplicate artifact role %q", ErrManifestInvalid, art.Role)
		}

		if !IsValidSHA256(art.SHA256) {
			return fmt.Errorf("%w: artifact %q has invalid sha256 %q", ErrManifestInvalid, art.Path, art.SHA256)
		}
		if art.ByteLength < 0 {
			return fmt.Errorf("%w: artifact %q has negative byte_length %d", ErrManifestInvalid, art.Path, art.ByteLength)
		}
	}

	if m.Run.State == domain.RunStateCompleted {
		if roleCounts[ArtifactRoleAdapterStdout] != 1 {
			return fmt.Errorf("%w: completed run manifest must include exactly one %q artifact",
				ErrManifestInvalid, ArtifactRoleAdapterStdout)
		}
		if roleCounts[ArtifactRoleAssertions] != 1 {
			return fmt.Errorf("%w: completed run manifest must include exactly one %q artifact",
				ErrManifestInvalid, ArtifactRoleAssertions)
		}
	}

	return nil
}

func parseUTCRFC3339(field, value string) (time.Time, error) {
	if !strings.HasSuffix(value, "Z") {
		return time.Time{}, fmt.Errorf("%w: %s %q must use UTC 'Z' suffix", ErrManifestInvalid, field, value)
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid %s %q: %v", ErrManifestInvalid, field, value, err)
	}
	if _, offset := t.Zone(); offset != 0 {
		return time.Time{}, fmt.Errorf("%w: %s %q must be in UTC", ErrManifestInvalid, field, value)
	}
	return t, nil
}

func isValidArtifactRole(role string) bool {
	switch role {
	case ArtifactRoleAdapterStdout,
		ArtifactRoleSemanticProjection,
		ArtifactRoleAssertions,
		ArtifactRoleInputCopy:
		return true
	default:
		return false
	}
}
