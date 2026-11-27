package reporting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/evidence"
)

// RenderMarkdown renders a deterministic, human-readable Markdown report from
// a validated evidence.Manifest. All manifest strings are treated as untrusted
// data and escaped to prevent Markdown or table structure injection.
func RenderMarkdown(manifest *evidence.Manifest) ([]byte, error) {
	if manifest == nil {
		return nil, fmt.Errorf("cannot render Markdown report: manifest must not be nil")
	}
	if err := evidence.ValidateManifest(manifest); err != nil {
		return nil, fmt.Errorf("cannot render Markdown report: %w", err)
	}

	// Enforce the exact same schema validation path as RenderJSON so both
	// renderers operate on identically valid manifests without mutating input.
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("validate manifest for Markdown report: %w", err)
	}
	if _, err := evidence.DecodeAndValidateManifest(encoded); err != nil {
		return nil, fmt.Errorf("cannot render Markdown report: %w", err)
	}

	var b bytes.Buffer

	b.WriteString("# Stellar Integrity Workbench — Replay Report\n\n")

	// 1. Report identity
	b.WriteString("## Report Identity\n\n")
	writeKV(&b, "Evidence Schema Version", manifest.SchemaVersion)
	writeKV(&b, "Run ID", manifest.RunID)
	writeKV(&b, "Case ID", manifest.CaseID)
	writeKV(&b, "Classification", string(manifest.Classification))
	writeKV(&b, "Source Kind", manifest.Provenance.Kind)
	if manifest.Provenance.URL != nil {
		writeKV(&b, "Source URL", *manifest.Provenance.URL)
	}
	if manifest.Provenance.RetrievedAt != nil {
		writeKV(&b, "Source Retrieved At", *manifest.Provenance.RetrievedAt)
	}
	writeKV(&b, "Source Notes", manifest.Provenance.Notes)
	b.WriteByte('\n')

	// 2. Execution
	b.WriteString("## Execution\n\n")
	writeKV(&b, "Execution State", string(manifest.Run.State))
	writeKV(&b, "Started At", manifest.Run.StartedAt)
	writeKV(&b, "Completed At", manifest.Run.CompletedAt)
	writeKV(&b, "Host Exit Code", formatOptInt(manifest.Run.HostExitCode))
	writeKV(&b, "Host Duration (ms)", formatOptInt64(manifest.Run.HostDurationMs))
	writeKV(&b, "Adapter Execution Status", formatOptString(manifest.Run.AdapterExecutionStatus))
	writeKV(&b, "Adapter Exit Code", formatOptInt(manifest.Run.AdapterExitCode))
	writeKV(&b, "Adapter Duration (ms)", formatOptInt64(manifest.Run.AdapterDurationMs))
	b.WriteByte('\n')

	// 3. Conformance
	b.WriteString("## Conformance\n\n")
	outcomeText, interp := describeConformance(manifest.Run.State, manifest.Run.Outcome)
	writeKV(&b, "Conformance Outcome", outcomeText)
	writeKV(&b, "Interpretation", interp)
	b.WriteByte('\n')

	// 4. Input integrity
	b.WriteString("## Input Integrity\n\n")
	writeKV(&b, "Format", manifest.Input.Format)
	writeKV(&b, "Encoding", manifest.Input.Encoding)
	writeKV(&b, "SHA-256", manifest.Input.SHA256)
	writeKV(&b, "Byte Length", strconv.FormatInt(manifest.Input.ByteLength, 10))
	writeKV(&b, "Corpus Object Path", manifest.Input.ObjectPath)
	writeKV(&b, "Detected Type", formatOptString(manifest.Input.DetectedType))
	writeKV(&b, "Network Name", manifest.Input.NetworkName)
	writeKV(&b, "Ledger Sequence", formatOptInt64(manifest.Input.LedgerSequence))
	writeKV(&b, "Protocol Version", formatOptInt(manifest.Input.ProtocolVersion))
	b.WriteByte('\n')

	// 5. Adapter and worker
	b.WriteString("## Adapter and Worker\n\n")
	writeKV(&b, "Adapter ID", manifest.Adapter.AdapterID)
	writeKV(&b, "Adapter Version", manifest.Adapter.AdapterVersion)
	writeKV(&b, "SDK Version", manifest.Adapter.SDKVersion)
	writeKV(&b, "Worker Version", manifest.Worker.Version)
	writeKV(&b, "Worker Commit SHA", formatOptString(manifest.Worker.CommitSHA))
	b.WriteByte('\n')

	// 6. Assertions
	if manifest.Run.State != domain.RunStateCompleted {
		b.WriteString("## Assertions (Partial Results — No Conformance Outcome Derived)\n\n")
	} else {
		b.WriteString("## Assertions\n\n")
	}
	if len(manifest.Assertions) == 0 {
		b.WriteString("No assertion results recorded.\n\n")
	} else {
		b.WriteString("| Assertion ID | Kind | Expected | Observed | Status | Explanation | Evidence References |\n")
		b.WriteString("|---|---|---|---|---|---|---|\n")
		for _, a := range manifest.Assertions {
			obs := "unavailable"
			if a.Observed != nil {
				obs = formatExactValue(a.Observed)
			}
			refs := "none"
			if len(a.EvidenceRefs) > 0 {
				refs = strings.Join(a.EvidenceRefs, ", ")
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n",
				escapeMarkdown(a.AssertionID),
				escapeMarkdown(string(a.Kind)),
				escapeMarkdown(formatExactValue(a.Expected)),
				escapeMarkdown(obs),
				escapeMarkdown(string(a.Status)),
				escapeMarkdown(a.Explanation),
				escapeMarkdown(refs),
			)
		}
		b.WriteByte('\n')
	}

	// 7. Diagnostics and limitations
	b.WriteString("## Diagnostics and Limitations\n\n")
	writeKV(&b, "Adapter Stderr Bytes Seen", formatOptInt64(manifest.Diagnostics.AdapterStderrBytesSeen))
	writeKV(&b, "Adapter Stderr Truncated", formatOptBool(manifest.Diagnostics.AdapterStderrTruncated))
	b.WriteByte('\n')

	b.WriteString("### Adapter Diagnostics\n\n")
	if len(manifest.Diagnostics.AdapterDiagnostics) == 0 {
		b.WriteString("None recorded.\n\n")
	} else {
		b.WriteString("| Stage | Code | Message |\n")
		b.WriteString("|---|---|---|\n")
		for _, d := range manifest.Diagnostics.AdapterDiagnostics {
			fmt.Fprintf(&b, "| %s | %s | %s |\n",
				escapeMarkdown(d.Stage),
				escapeMarkdown(d.Code),
				escapeMarkdown(d.Message),
			)
		}
		b.WriteByte('\n')
	}

	b.WriteString("### Known Limitations\n\n")
	if len(manifest.Diagnostics.Limitations) == 0 {
		b.WriteString("None recorded.\n\n")
	} else {
		for _, lim := range manifest.Diagnostics.Limitations {
			fmt.Fprintf(&b, "- %s\n", escapeMarkdown(lim))
		}
		b.WriteByte('\n')
	}

	// 8. Evidence artifacts
	b.WriteString("## Evidence Artifacts\n\n")
	if len(manifest.Artifacts) == 0 {
		b.WriteString("No artifacts recorded.\n")
	} else {
		b.WriteString("| Path | Role | Media Type | Byte Length | SHA-256 |\n")
		b.WriteString("|---|---|---|---|---|\n")
		for _, art := range manifest.Artifacts {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
				escapeMarkdown(art.Path),
				escapeMarkdown(art.Role),
				escapeMarkdown(art.MediaType),
				strconv.FormatInt(art.ByteLength, 10),
				escapeMarkdown(art.SHA256),
			)
		}
	}

	return b.Bytes(), nil
}

func describeConformance(state domain.RunState, outcome *domain.RunOutcome) (string, string) {
	if state == domain.RunStateFailed {
		return "none (no completed conformance outcome recorded)",
			"Execution failed; no completed conformance outcome is recorded."
	}
	if state == domain.RunStateCancelled {
		return "none (no completed conformance outcome recorded)",
			"Execution was cancelled; no completed conformance outcome is recorded."
	}
	if outcome == nil {
		return "none", "No conformance outcome recorded."
	}
	switch *outcome {
	case domain.RunOutcomePass:
		return string(*outcome), "Execution completed and assertions passed."
	case domain.RunOutcomeRegression:
		return string(*outcome), "Execution completed and one or more assertions failed."
	case domain.RunOutcomeInconclusive:
		return string(*outcome), "Execution completed but required assertions could not all be evaluated."
	case domain.RunOutcomeNotApplicable:
		return string(*outcome), "The required capability was not applicable."
	default:
		return string(*outcome), "Unknown conformance outcome."
	}
}

func writeKV(b *bytes.Buffer, label, value string) {
	fmt.Fprintf(b, "- **%s:** %s\n", label, escapeMarkdown(value))
}

func formatOptString(v *string) string {
	if v == nil {
		return "not recorded"
	}
	return *v
}

func formatOptInt(v *int) string {
	if v == nil {
		return "not recorded"
	}
	return strconv.Itoa(*v)
}

func formatOptInt64(v *int64) string {
	if v == nil {
		return "not recorded"
	}
	return strconv.FormatInt(*v, 10)
}

func formatOptBool(v *bool) string {
	if v == nil {
		return "not recorded"
	}
	if *v {
		return "true"
	}
	return "false"
}

func formatExactValue(v any) string {
	switch x := v.(type) {
	case json.Number:
		return x.String()
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case bool:
		return strconv.FormatBool(x)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// escapeMarkdown neutralizes newlines, control characters, pipes, backticks,
// and Markdown formatting tokens in untrusted manifest strings.
func escapeMarkdown(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\r', '\n', '\t':
			out.WriteByte(' ')
		case '\\', '`', '*', '_', '{', '}', '[', ']', '(', ')', '#', '+', '-', '!', '|', '<', '>', '~':
			out.WriteByte('\\')
			out.WriteRune(r)
		default:
			if r < 0x20 || r == 0x7f {
				continue
			}
			out.WriteRune(r)
		}
	}
	return out.String()
}
