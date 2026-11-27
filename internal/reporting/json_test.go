package reporting_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/adapter"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/evidence"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/reporting"
)

const (
	testInputSHA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testArt1SHA   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testArt2SHA   = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	testArt3SHA   = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	testCommitSHA = "1234567890abcdef1234567890abcdef12345678"
)

func makeSyntheticManifest(state domain.RunState, outcome *domain.RunOutcome, assertions []domain.AssertionResult) *evidence.Manifest {
	url := "https://example.invalid/synthetic/sample.xdr"
	retrievedAt := "2026-11-27T07:00:00Z"
	detectedType := "LedgerCloseMeta"
	seq := int64(4294967295)
	proto := 21
	commit := testCommitSHA
	execStatus := "completed"
	exitCode := 0
	adapterDur := int64(15)
	hostExit := 0
	hostDur := int64(42)
	stderrSeen := int64(128)
	stderrTrunc := false

	artifacts := []evidence.ArtifactRef{
		{
			Path:       "artifacts/adapter-stdout.json",
			Role:       evidence.ArtifactRoleAdapterStdout,
			MediaType:  "application/json",
			ByteLength: 512,
			SHA256:     testArt1SHA,
		},
		{
			Path:       "artifacts/semantic-projection.json",
			Role:       evidence.ArtifactRoleSemanticProjection,
			MediaType:  "application/json",
			ByteLength: 256,
			SHA256:     testArt2SHA,
		},
		{
			Path:       "artifacts/assertions.json",
			Role:       evidence.ArtifactRoleAssertions,
			MediaType:  "application/json",
			ByteLength: 384,
			SHA256:     testArt3SHA,
		},
	}

	if state != domain.RunStateCompleted {
		execStatus = string(state)
		exitCode = 3
		hostExit = 3
		detectedType = ""
		artifacts = []evidence.ArtifactRef{artifacts[0]}
	}

	var detectedPtr *string
	if detectedType != "" {
		detectedPtr = &detectedType
	}

	return &evidence.Manifest{
		SchemaVersion:  evidence.SchemaVersion,
		RunID:          "run_synthetic_report_001",
		CaseID:         "case_synthetic_report_001",
		Classification: domain.ClassificationSyntheticFixture,
		Provenance: evidence.Provenance{
			Kind:        "synthetic_construction",
			URL:         &url,
			RetrievedAt: &retrievedAt,
			Notes:       "Synthetic manifest for reporting unit tests",
		},
		Input: evidence.InputEvidence{
			SHA256:          testInputSHA,
			ByteLength:      1024,
			Format:          "LedgerCloseMeta",
			Encoding:        "xdr_binary",
			ObjectPath:      "synthetic/sample.xdr",
			DetectedType:    detectedPtr,
			NetworkName:     "Synthetic Standalone Network",
			LedgerSequence:  &seq,
			ProtocolVersion: &proto,
		},
		Worker: evidence.WorkerMetadata{
			Version:   "0.1.0-test",
			CommitSHA: &commit,
		},
		Adapter: evidence.AdapterMetadata{
			AdapterID:      adapter.AdapterIDGoIngest,
			AdapterVersion: "dev",
			SDKVersion:     "v0.9.0",
		},
		Run: evidence.RunMetadata{
			StartedAt:              "2026-11-27T08:00:00Z",
			CompletedAt:            "2026-11-27T08:00:02Z",
			State:                  state,
			AdapterExecutionStatus: &execStatus,
			AdapterExitCode:        &exitCode,
			AdapterDurationMs:      &adapterDur,
			HostExitCode:           &hostExit,
			HostDurationMs:         &hostDur,
			Outcome:                outcome,
		},
		Assertions: assertions,
		Diagnostics: evidence.DiagnosticsEvidence{
			AdapterDiagnostics: []adapter.ResultDiagnostic{
				{Stage: "decode", Code: "diag_note", Message: "synthetic diagnostic"},
			},
			Limitations:            []string{"synthetic fixture only"},
			AdapterStderrBytesSeen: &stderrSeen,
			AdapterStderrTruncated: &stderrTrunc,
		},
		Artifacts: artifacts,
	}
}

func TestRenderJSON_OutcomesAndStates(t *testing.T) {
	pass := domain.RunOutcomePass
	reg := domain.RunOutcomeRegression
	inc := domain.RunOutcomeInconclusive
	na := domain.RunOutcomeNotApplicable

	tests := []struct {
		name       string
		state      domain.RunState
		outcome    *domain.RunOutcome
		assertions []domain.AssertionResult
	}{
		{
			name:    "completed_pass_max_uint32",
			state:   domain.RunStateCompleted,
			outcome: &pass,
			assertions: []domain.AssertionResult{
				{
					AssertionID:  "assertion_seq",
					Kind:         domain.AssertionKindLedgerSequenceEquals,
					Expected:     json.Number("4294967295"),
					Observed:     json.Number("4294967295"),
					Status:       domain.AssertionStatusPass,
					Explanation:  "observed ledger sequence 4294967295 matches expected 4294967295",
					EvidenceRefs: []string{"logical://input/sha256/" + testInputSHA},
				},
			},
		},
		{
			name:    "completed_regression",
			state:   domain.RunStateCompleted,
			outcome: &reg,
			assertions: []domain.AssertionResult{
				{
					AssertionID: "assertion_seq",
					Kind:        domain.AssertionKindLedgerSequenceEquals,
					Expected:    json.Number("100"),
					Observed:    json.Number("4294967295"),
					Status:      domain.AssertionStatusFail,
					Explanation: "observed ledger sequence 4294967295 does not match expected 100",
				},
			},
		},
		{
			name:    "completed_inconclusive",
			state:   domain.RunStateCompleted,
			outcome: &inc,
			assertions: []domain.AssertionResult{
				{
					AssertionID: "assertion_seq",
					Kind:        domain.AssertionKindLedgerSequenceEquals,
					Expected:    json.Number("100"),
					Observed:    nil,
					Status:      domain.AssertionStatusInconclusive,
					Explanation: "observed ledger sequence is unavailable",
				},
			},
		},
		{
			name:    "completed_not_applicable",
			state:   domain.RunStateCompleted,
			outcome: &na,
			assertions: []domain.AssertionResult{
				{
					AssertionID: "assertion_seq",
					Kind:        domain.AssertionKindLedgerSequenceEquals,
					Expected:    json.Number("100"),
					Status:      domain.AssertionStatusSkipped,
					Explanation: "not supported by adapter",
				},
			},
		},
		{
			name:       "failed_with_null_outcome",
			state:      domain.RunStateFailed,
			outcome:    nil,
			assertions: []domain.AssertionResult{},
		},
		{
			name:       "cancelled_with_null_outcome",
			state:      domain.RunStateCancelled,
			outcome:    nil,
			assertions: []domain.AssertionResult{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := makeSyntheticManifest(tc.state, tc.outcome, tc.assertions)
			out1, err := reporting.RenderJSON(m)
			if err != nil {
				t.Fatalf("RenderJSON failed: %v", err)
			}
			if len(out1) == 0 || out1[len(out1)-1] != '\n' {
				t.Error("expected RenderJSON output to end with newline")
			}

			// Schema-validate and check semantic equivalence.
			decoded, err := evidence.DecodeAndValidateManifest(out1)
			if err != nil {
				t.Fatalf("DecodeAndValidateManifest failed: %v", err)
			}
			if decoded.Run.State != tc.state {
				t.Errorf("decoded State = %q, want %q", decoded.Run.State, tc.state)
			}
			if (decoded.Run.Outcome == nil) != (tc.outcome == nil) {
				t.Fatalf("decoded Outcome nilness mismatch: got %v, want %v", decoded.Run.Outcome, tc.outcome)
			}
			if tc.outcome != nil && *decoded.Run.Outcome != *tc.outcome {
				t.Errorf("decoded Outcome = %q, want %q", *decoded.Run.Outcome, *tc.outcome)
			}

			// Exact integer preservation check.
			if tc.name == "completed_pass_max_uint32" && !strings.Contains(string(out1), "4294967295") {
				t.Error("expected exact uint32 4294967295 preserved in JSON")
			}

			// Deterministic repeated rendering.
			out2, err := reporting.RenderJSON(m)
			if err != nil {
				t.Fatalf("second RenderJSON failed: %v", err)
			}
			if !bytes.Equal(out1, out2) {
				t.Error("RenderJSON output was not deterministic across repeated calls")
			}
		})
	}
}

func TestRenderJSON_OptionalNullFieldsAndRejections(t *testing.T) {
	t.Run("nil_manifest_rejected", func(t *testing.T) {
		if _, err := reporting.RenderJSON(nil); err == nil {
			t.Fatal("expected error for nil manifest")
		}
	})

	t.Run("invalid_manifest_rejected", func(t *testing.T) {
		pass := domain.RunOutcomePass
		m := makeSyntheticManifest(domain.RunStateFailed, &pass, []domain.AssertionResult{})
		if _, err := reporting.RenderJSON(m); err == nil {
			t.Fatal("expected error for failed manifest with non-nil outcome")
		}
	})

	t.Run("null_optional_fields_preserved", func(t *testing.T) {
		m := makeSyntheticManifest(domain.RunStateFailed, nil, []domain.AssertionResult{})
		m.Provenance.URL = nil
		m.Provenance.RetrievedAt = nil
		m.Worker.CommitSHA = nil
		m.Input.LedgerSequence = nil
		m.Input.ProtocolVersion = nil
		m.Run.AdapterExecutionStatus = nil
		m.Run.AdapterExitCode = nil
		m.Run.AdapterDurationMs = nil
		m.Run.HostExitCode = nil
		m.Run.HostDurationMs = nil
		m.Diagnostics.AdapterStderrBytesSeen = nil
		m.Diagnostics.AdapterStderrTruncated = nil

		out, err := reporting.RenderJSON(m)
		if err != nil {
			t.Fatalf("RenderJSON failed with nil optional fields: %v", err)
		}
		if _, err := evidence.DecodeAndValidateManifest(out); err != nil {
			t.Fatalf("DecodeAndValidateManifest failed: %v", err)
		}
		if !strings.Contains(string(out), `"commit_sha": null`) {
			t.Error("expected commit_sha: null in JSON output")
		}
	})
}
