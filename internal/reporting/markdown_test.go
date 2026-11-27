package reporting_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/evidence"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/reporting"
)

func TestRenderMarkdown_StatesOutcomesAndConsistency(t *testing.T) {
	pass := domain.RunOutcomePass
	reg := domain.RunOutcomeRegression
	inc := domain.RunOutcomeInconclusive
	na := domain.RunOutcomeNotApplicable

	t.Run("completed_pass", func(t *testing.T) {
		m := makeSyntheticManifest(domain.RunStateCompleted, &pass, []domain.AssertionResult{
			{
				AssertionID:  "assertion_seq",
				Kind:         domain.AssertionKindLedgerSequenceEquals,
				Expected:     json.Number("4294967295"),
				Observed:     json.Number("4294967295"),
				Status:       domain.AssertionStatusPass,
				Explanation:  "matched",
				EvidenceRefs: []string{"logical://input/sha256/" + testInputSHA},
			},
		})
		md, err := reporting.RenderMarkdown(m)
		if err != nil {
			t.Fatalf("RenderMarkdown failed: %v", err)
		}
		s := string(md)
		for _, want := range []string{
			"Execution State:** completed",
			"Conformance Outcome:** pass",
			"Execution completed and assertions passed.",
			"Host Duration (ms):** 42",
			"Adapter Duration (ms):** 15",
			testInputSHA,
			testArt1SHA,
			"4294967295",
		} {
			if !strings.Contains(s, want) {
				t.Errorf("Markdown missing %q", want)
			}
		}

		// Cross-renderer consistency with JSON.
		js, err := reporting.RenderJSON(m)
		if err != nil {
			t.Fatalf("RenderJSON failed: %v", err)
		}
		decoded, _ := evidence.DecodeAndValidateManifest(js)
		if *decoded.Run.Outcome != domain.RunOutcomePass {
			t.Errorf("JSON outcome = %s, want pass", *decoded.Run.Outcome)
		}
	})

	t.Run("completed_regression", func(t *testing.T) {
		m := makeSyntheticManifest(domain.RunStateCompleted, &reg, []domain.AssertionResult{
			{
				AssertionID: "assertion_seq",
				Kind:        domain.AssertionKindLedgerSequenceEquals,
				Expected:    json.Number("100"),
				Observed:    json.Number("200"),
				Status:      domain.AssertionStatusFail,
				Explanation: "mismatch",
			},
		})
		md, err := reporting.RenderMarkdown(m)
		if err != nil {
			t.Fatalf("RenderMarkdown failed: %v", err)
		}
		s := string(md)
		if !strings.Contains(s, "Execution State:** completed") || !strings.Contains(s, "Conformance Outcome:** regression") {
			t.Errorf("Markdown did not distinguish completed execution with regression outcome:\n%s", s)
		}

		js, _ := reporting.RenderJSON(m)
		decoded, _ := evidence.DecodeAndValidateManifest(js)
		if decoded.Run.State != domain.RunStateCompleted || *decoded.Run.Outcome != domain.RunOutcomeRegression {
			t.Errorf("JSON state/outcome mismatch: %s / %v", decoded.Run.State, decoded.Run.Outcome)
		}
	})

	t.Run("completed_inconclusive_and_not_applicable", func(t *testing.T) {
		mInc := makeSyntheticManifest(domain.RunStateCompleted, &inc, []domain.AssertionResult{
			{
				AssertionID: "assertion_seq",
				Kind:        domain.AssertionKindLedgerSequenceEquals,
				Expected:    json.Number("100"),
				Status:      domain.AssertionStatusInconclusive,
				Explanation: "unavailable",
			},
		})
		mdInc, err := reporting.RenderMarkdown(mInc)
		if err != nil {
			t.Fatalf("RenderMarkdown inconclusive: %v", err)
		}
		if !strings.Contains(string(mdInc), "Conformance Outcome:** inconclusive") {
			t.Error("expected inconclusive outcome in Markdown")
		}

		mNA := makeSyntheticManifest(domain.RunStateCompleted, &na, []domain.AssertionResult{
			{
				AssertionID: "assertion_seq",
				Kind:        domain.AssertionKindLedgerSequenceEquals,
				Expected:    json.Number("100"),
				Status:      domain.AssertionStatusSkipped,
				Explanation: "unsupported",
			},
		})
		mdNA, err := reporting.RenderMarkdown(mNA)
		if err != nil {
			t.Fatalf("RenderMarkdown not_applicable: %v", err)
		}
		if !strings.Contains(string(mdNA), "Conformance Outcome:** not\\_applicable") {
			t.Errorf("expected not_applicable outcome in Markdown, got:\n%s", string(mdNA))
		}
	})

	t.Run("failed_and_cancelled_with_partial_assertions", func(t *testing.T) {
		partial := []domain.AssertionResult{
			{
				AssertionID: "assertion_sha",
				Kind:        domain.AssertionKindInputHashEquals,
				Expected:    testInputSHA,
				Observed:    testInputSHA,
				Status:      domain.AssertionStatusPass,
				Explanation: "verified input matched before failure",
			},
		}

		for _, state := range []domain.RunState{domain.RunStateFailed, domain.RunStateCancelled} {
			m := makeSyntheticManifest(state, nil, partial)
			md, err := reporting.RenderMarkdown(m)
			if err != nil {
				t.Fatalf("RenderMarkdown(%s): %v", state, err)
			}
			s := string(md)
			if !strings.Contains(s, "no completed conformance outcome recorded") {
				t.Errorf("%s Markdown missing explicit no-outcome statement", state)
			}
			if !strings.Contains(s, "Partial Results") {
				t.Errorf("%s Markdown missing Partial Results label", state)
			}

			js, err := reporting.RenderJSON(m)
			if err != nil {
				t.Fatalf("RenderJSON(%s): %v", state, err)
			}
			decoded, _ := evidence.DecodeAndValidateManifest(js)
			if decoded.Run.State != state || decoded.Run.Outcome != nil {
				t.Errorf("%s JSON state/outcome mismatch: %s / %v", state, decoded.Run.State, decoded.Run.Outcome)
			}
		}
	})
}

func TestRenderMarkdown_SafetyDeterminismAndMissingMetadata(t *testing.T) {
	t.Run("nil_and_invalid_manifest_rejected", func(t *testing.T) {
		if _, err := reporting.RenderMarkdown(nil); err == nil {
			t.Fatal("expected error for nil manifest")
		}
		pass := domain.RunOutcomePass
		bad := makeSyntheticManifest(domain.RunStateFailed, &pass, []domain.AssertionResult{})
		if _, err := reporting.RenderMarkdown(bad); err == nil {
			t.Fatal("expected error for invalid manifest")
		}
	})

	t.Run("escapes_untrusted_markdown_and_newlines", func(t *testing.T) {
		pass := domain.RunOutcomePass
		m := makeSyntheticManifest(domain.RunStateCompleted, &pass, []domain.AssertionResult{
			{
				AssertionID: "assertion_1",
				Kind:        domain.AssertionKindDecodedTypeEquals,
				Expected:    "LedgerCloseMeta",
				Observed:    "LedgerCloseMeta",
				Status:      domain.AssertionStatusPass,
				Explanation: "bad | pipe `code`\n# injected heading [click](javascript:alert(1))",
			},
		})
		m.Provenance.Notes = "line1\n| col1 | col2 |\n`tick`"
		m.Diagnostics.Limitations = []string{"limit *bold* _ital_ <script>"}

		md, err := reporting.RenderMarkdown(m)
		if err != nil {
			t.Fatalf("RenderMarkdown failed: %v", err)
		}
		s := string(md)
		if strings.Contains(s, "\n# injected heading") {
			t.Error("newline injection created an unescaped Markdown heading")
		}
		if strings.Contains(s, "[click](javascript:alert(1))") {
			t.Error("unescaped Markdown link injected")
		}
		if strings.Contains(s, "<script>") {
			t.Error("unescaped HTML tag injected")
		}
	})

	t.Run("missing_optional_metadata_not_fabricated", func(t *testing.T) {
		m := makeSyntheticManifest(domain.RunStateFailed, nil, []domain.AssertionResult{})
		m.Worker.CommitSHA = nil
		m.Input.DetectedType = nil
		m.Input.LedgerSequence = nil
		m.Input.ProtocolVersion = nil
		m.Run.HostDurationMs = nil
		m.Run.HostExitCode = nil

		md, err := reporting.RenderMarkdown(m)
		if err != nil {
			t.Fatalf("RenderMarkdown failed: %v", err)
		}
		if !strings.Contains(string(md), "Worker Commit SHA:** not recorded") {
			t.Error("expected missing Worker Commit SHA to render as 'not recorded'")
		}
	})

	t.Run("deterministic_output", func(t *testing.T) {
		pass := domain.RunOutcomePass
		m := makeSyntheticManifest(domain.RunStateCompleted, &pass, []domain.AssertionResult{
			{
				AssertionID: "assertion_seq",
				Kind:        domain.AssertionKindLedgerSequenceEquals,
				Expected:    json.Number("100"),
				Observed:    json.Number("100"),
				Status:      domain.AssertionStatusPass,
				Explanation: "matched",
			},
		})
		md1, err := reporting.RenderMarkdown(m)
		if err != nil {
			t.Fatalf("first RenderMarkdown: %v", err)
		}
		md2, err := reporting.RenderMarkdown(m)
		if err != nil {
			t.Fatalf("second RenderMarkdown: %v", err)
		}
		if !bytes.Equal(md1, md2) {
			t.Error("RenderMarkdown was not deterministic across repeated calls")
		}
	})
}
