package reporting

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/evidence"
)

// ErrUnsafeReportContent is returned when a report renderer detects
// recognizable sensitive content (such as local filesystem paths, credentials,
// tokens, PEM private keys, Stellar secret keys, sensitive object properties,
// or sensitive URL parameters) in the manifest. Renderers fail closed without
// emitting or mutating content.
var ErrUnsafeReportContent = errors.New("unsafe report content")

var (
	httpURLPattern     = regexp.MustCompile(`https?://[^\s"'<>]+`)
	fileURLPattern     = regexp.MustCompile(`(?i)\bfile://[^\s"'<>]*`)
	unixAbsPathPattern = regexp.MustCompile(`(^|[\s"'(=:])/(?:home|tmp|var|etc|usr|opt|mnt|media|root|Users|Volumes|private|proc|sys|dev|workspace|srv|run)(?:/[^\s"']*|$)`)
	winDrivePattern    = regexp.MustCompile(`(?i)(^|[\s"'(=])[a-z]:(?:\\|/(?:users|windows|program|temp|tmp|workspace))`)
	winUNCPattern      = regexp.MustCompile(`(^|[\s"'(=])\\\\[a-zA-Z0-9._-]+\\[^\s"']+`)
	bearerTokenPattern = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]{8,}`)
	credAssignPattern  = regexp.MustCompile(`(?i)\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|password|passwd|pwd|secret|private[_-]?key|client[_-]?secret|auth[_-]?token)\s*[:=]\s*["']?[^\s"']{4,}`)
	pemKeyPattern      = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)
	stellarSeedPattern = regexp.MustCompile(`\bS[A-Z2-7]{55}\b`)
)

var sensitiveQueryParams = map[string]struct{}{
	"accesstoken":       {},
	"token":             {},
	"apikey":            {},
	"key":               {},
	"secret":            {},
	"password":          {},
	"passwd":            {},
	"pwd":               {},
	"auth":              {},
	"authorization":     {},
	"sig":               {},
	"signature":         {},
	"xamzsignature":     {},
	"xamzcredential":    {},
	"xamzsecuritytoken": {},
	"xgoogsignature":    {},
	"xgoogcredential":   {},
	"sas":               {},
	"se":                {},
	"sp":                {},
	"sv":                {},
	"sr":                {},
	"st":                {},
	"refreshtoken":      {},
	"clientsecret":      {},
	"privatekey":        {},
	"authtoken":         {},
	"secretkey":         {},
	"signingkey":        {},
	"secretaccesskey":   {},
	"sessiontoken":      {},
	"bearertoken":       {},
}

var sensitivePropertyKeys = map[string]struct{}{
	"apikey":          {},
	"accesstoken":     {},
	"refreshtoken":    {},
	"password":        {},
	"passwd":          {},
	"pwd":             {},
	"secret":          {},
	"secretkey":       {},
	"privatekey":      {},
	"clientsecret":    {},
	"authtoken":       {},
	"authorization":   {},
	"bearer":          {},
	"bearertoken":     {},
	"token":           {},
	"signingkey":      {},
	"secretaccesskey": {},
	"sessiontoken":    {},
}

// validateReportSafety marshals manifest to its canonical JSON representation,
// decodes it into generic JSON values with UseNumber(), and recursively
// inspects every object key, nested array, and string value for recognizable
// sensitive content. It fails closed with ErrUnsafeReportContent, naming only
// the field path and category without echoing any secret value.
func validateReportSafety(manifest *evidence.Manifest) error {
	if manifest == nil {
		return nil
	}

	rawJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("%w: failed to serialize manifest for safety inspection", ErrUnsafeReportContent)
	}

	var generic any
	dec := json.NewDecoder(bytes.NewReader(rawJSON))
	dec.UseNumber()
	if err := dec.Decode(&generic); err != nil {
		return fmt.Errorf("%w: failed to decode manifest JSON for safety inspection", ErrUnsafeReportContent)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON data during safety inspection", ErrUnsafeReportContent)
	}

	return checkSafeJSONValue("$", generic)
}

func checkSafeJSONValue(path string, v any) error {
	switch x := v.(type) {
	case nil, bool, json.Number:
		return nil
	case string:
		return checkSafeString(path, x)
	case []any:
		for i, item := range x {
			if err := checkSafeJSONValue(fmt.Sprintf("%s[%d]", path, i), item); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for k, item := range x {
			if err := checkSafeString(path+".<key>", k); err != nil {
				return err
			}
			childPath := path + "." + k
			if isSensitivePropertyKey(k) && hasNonEmptyJSONValue(item) {
				return fmt.Errorf("%w: %s is a credential-like property with a non-empty value", ErrUnsafeReportContent, childPath)
			}
			if err := checkSafeJSONValue(childPath, item); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%w: %s has unsupported JSON value type %T", ErrUnsafeReportContent, path, v)
	}
}

func normalizeCredentialKey(key string) string {
	trimmed := strings.TrimSpace(key)
	var b strings.Builder
	b.Grow(len(trimmed))
	for _, r := range trimmed {
		if r == '_' || r == '-' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isSensitivePropertyKey(key string) bool {
	_, ok := sensitivePropertyKeys[normalizeCredentialKey(key)]
	return ok
}

func hasNonEmptyJSONValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
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
		if _, sensitive := sensitiveQueryParams[normalizeCredentialKey(k)]; sensitive {
			return fmt.Errorf("%w: %s contains URL with sensitive query parameter %q", ErrUnsafeReportContent, field, k)
		}
	}
	return nil
}
