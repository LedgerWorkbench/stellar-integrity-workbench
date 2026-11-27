package reporting

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/evidence"
)

// ErrUnsafeReportContent is returned when a report renderer detects
// recognizable sensitive content (such as local filesystem paths, credentials,
// tokens, PEM private keys, Stellar secret keys, or sensitive URL parameters)
// in the manifest. Renderers fail closed without emitting or mutating content.
var ErrUnsafeReportContent = errors.New("unsafe report content")

var (
	httpURLPattern     = regexp.MustCompile(`https?://[^\s"'<>]+`)
	fileURLPattern     = regexp.MustCompile(`(?i)\bfile://[^\s"'<>]*`)
	unixAbsPathPattern = regexp.MustCompile(`(^|[\s"'(=:])/(?:home|tmp|var|etc|usr|opt|mnt|media|root|Users|Volumes|private|proc|sys|dev|workspace|srv|run)(?:/[^\s"']*|$)`)
	winDrivePattern    = regexp.MustCompile(`(?i)(^|[\s"'(=])[a-z]:(?:\\|/(?:users|windows|program|temp|tmp|workspace))`)
	winUNCPattern      = regexp.MustCompile(`(^|[\s"'(=])\\\\[a-zA-Z0-9._-]+\\[^\s"']+`)
	bearerTokenPattern = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]{8,}`)
	credAssignPattern  = regexp.MustCompile(`(?i)\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|password|passwd|secret|private[_-]?key|client[_-]?secret|auth[_-]?token)\s*[:=]\s*["']?[^\s"']{4,}`)
	pemKeyPattern      = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)
	stellarSeedPattern = regexp.MustCompile(`\bS[A-Z2-7]{55}\b`)
)

var sensitiveQueryParams = map[string]struct{}{
	"access_token":         {},
	"token":                {},
	"api_key":              {},
	"apikey":               {},
	"key":                  {},
	"secret":               {},
	"password":             {},
	"passwd":               {},
	"pwd":                  {},
	"auth":                 {},
	"authorization":        {},
	"sig":                  {},
	"signature":            {},
	"x-amz-signature":      {},
	"x-amz-credential":     {},
	"x-amz-security-token": {},
	"x-goog-signature":     {},
	"x-goog-credential":    {},
	"sas":                  {},
	"se":                   {},
	"sp":                   {},
	"sv":                   {},
	"sr":                   {},
	"st":                   {},
	"refresh_token":        {},
	"client_secret":        {},
	"private_key":          {},
}

// validateReportSafety inspects all string-bearing fields in manifest for
// recognizable sensitive content and fails closed with ErrUnsafeReportContent.
// Error messages identify only the field path and category, never the secret value.
func validateReportSafety(manifest *evidence.Manifest) error {
	if manifest == nil {
		return nil
	}

	checks := []struct {
		field string
		value string
	}{
		{"schema_version", manifest.SchemaVersion},
		{"run_id", manifest.RunID},
		{"case_id", manifest.CaseID},
		{"classification", string(manifest.Classification)},
		{"provenance.kind", manifest.Provenance.Kind},
		{"provenance.notes", manifest.Provenance.Notes},
		{"input.sha256", manifest.Input.SHA256},
		{"input.format", manifest.Input.Format},
		{"input.encoding", manifest.Input.Encoding},
		{"input.object_path", manifest.Input.ObjectPath},
		{"input.network_name", manifest.Input.NetworkName},
		{"worker.version", manifest.Worker.Version},
		{"adapter.adapter_id", manifest.Adapter.AdapterID},
		{"adapter.adapter_version", manifest.Adapter.AdapterVersion},
		{"adapter.sdk_version", manifest.Adapter.SDKVersion},
		{"run.started_at", manifest.Run.StartedAt},
		{"run.completed_at", manifest.Run.CompletedAt},
		{"run.state", string(manifest.Run.State)},
	}
	for _, c := range checks {
		if err := checkSafeString(c.field, c.value); err != nil {
			return err
		}
	}

	optChecks := []struct {
		field string
		value *string
	}{
		{"provenance.url", manifest.Provenance.URL},
		{"provenance.repository", manifest.Provenance.Repository},
		{"provenance.retrieved_at", manifest.Provenance.RetrievedAt},
		{"input.detected_type", manifest.Input.DetectedType},
		{"worker.commit_sha", manifest.Worker.CommitSHA},
		{"run.adapter_execution_status", manifest.Run.AdapterExecutionStatus},
	}
	for _, c := range optChecks {
		if c.value != nil {
			if err := checkSafeString(c.field, *c.value); err != nil {
				return err
			}
		}
	}

	for i, a := range manifest.Assertions {
		prefix := fmt.Sprintf("assertions[%d]", i)
		if err := checkSafeString(prefix+".assertion_id", a.AssertionID); err != nil {
			return err
		}
		if err := checkSafeString(prefix+".kind", string(a.Kind)); err != nil {
			return err
		}
		if err := checkSafeValue(prefix+".expected", a.Expected); err != nil {
			return err
		}
		if err := checkSafeValue(prefix+".observed", a.Observed); err != nil {
			return err
		}
		if err := checkSafeString(prefix+".status", string(a.Status)); err != nil {
			return err
		}
		if err := checkSafeString(prefix+".explanation", a.Explanation); err != nil {
			return err
		}
		for j, ref := range a.EvidenceRefs {
			if err := checkSafeString(fmt.Sprintf("%s.evidence_refs[%d]", prefix, j), ref); err != nil {
				return err
			}
		}
	}

	for i, d := range manifest.Diagnostics.AdapterDiagnostics {
		prefix := fmt.Sprintf("diagnostics.adapter_diagnostics[%d]", i)
		if err := checkSafeString(prefix+".stage", d.Stage); err != nil {
			return err
		}
		if err := checkSafeString(prefix+".code", d.Code); err != nil {
			return err
		}
		if err := checkSafeString(prefix+".message", d.Message); err != nil {
			return err
		}
	}
	for i, lim := range manifest.Diagnostics.Limitations {
		if err := checkSafeString(fmt.Sprintf("diagnostics.limitations[%d]", i), lim); err != nil {
			return err
		}
	}
	for i, art := range manifest.Artifacts {
		prefix := fmt.Sprintf("artifacts[%d]", i)
		if err := checkSafeString(prefix+".path", art.Path); err != nil {
			return err
		}
		if err := checkSafeString(prefix+".role", art.Role); err != nil {
			return err
		}
		if err := checkSafeString(prefix+".media_type", art.MediaType); err != nil {
			return err
		}
		if err := checkSafeString(prefix+".sha256", art.SHA256); err != nil {
			return err
		}
	}

	return nil
}

func checkSafeValue(field string, v any) error {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		return checkSafeString(field, x)
	case []string:
		for i, s := range x {
			if err := checkSafeString(fmt.Sprintf("%s[%d]", field, i), s); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range x {
			if err := checkSafeValue(fmt.Sprintf("%s[%d]", field, i), item); err != nil {
				return err
			}
		}
	case map[string]any:
		for k, item := range x {
			if err := checkSafeString(field+".<key>", k); err != nil {
				return err
			}
			if err := checkSafeValue(field+"."+k, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkSafeString(field, s string) error {
	if s == "" {
		return nil
	}

	// Inspect any http(s) URLs for credentials or sensitive query parameters,
	// then strip safe http(s) URLs before checking local Unix filesystem paths.
	urls := httpURLPattern.FindAllString(s, -1)
	for _, rawURL := range urls {
		if err := checkSafeHTTPURL(field, rawURL); err != nil {
			return err
		}
	}
	nonHTTP := httpURLPattern.ReplaceAllString(s, " ")

	if fileURLPattern.MatchString(s) {
		return fmt.Errorf("%w: %s contains a file:// local path URI", ErrUnsafeReportContent, field)
	}
	if unixAbsPathPattern.MatchString(nonHTTP) {
		return fmt.Errorf("%w: %s contains a Unix absolute filesystem path", ErrUnsafeReportContent, field)
	}
	if winDrivePattern.MatchString(nonHTTP) || winUNCPattern.MatchString(nonHTTP) {
		return fmt.Errorf("%w: %s contains a Windows absolute or UNC filesystem path", ErrUnsafeReportContent, field)
	}
	if bearerTokenPattern.MatchString(s) {
		return fmt.Errorf("%w: %s contains a bearer authorization credential", ErrUnsafeReportContent, field)
	}
	if credAssignPattern.MatchString(s) {
		return fmt.Errorf("%w: %s contains a credential assignment", ErrUnsafeReportContent, field)
	}
	if pemKeyPattern.MatchString(s) {
		return fmt.Errorf("%w: %s contains a PEM private-key header", ErrUnsafeReportContent, field)
	}
	if stellarSeedPattern.MatchString(s) {
		return fmt.Errorf("%w: %s contains a Stellar secret-key-shaped value", ErrUnsafeReportContent, field)
	}

	return nil
}

func checkSafeHTTPURL(field, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %s contains an unparseable URL", ErrUnsafeReportContent, field)
	}
	if u.User != nil {
		return fmt.Errorf("%w: %s contains URL userinfo credentials", ErrUnsafeReportContent, field)
	}
	q := u.Query()
	for k := range q {
		if _, sensitive := sensitiveQueryParams[strings.ToLower(k)]; sensitive {
			return fmt.Errorf("%w: %s contains URL with sensitive query parameter %q", ErrUnsafeReportContent, field, k)
		}
	}
	return nil
}
