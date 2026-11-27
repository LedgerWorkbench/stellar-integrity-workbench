package reporting_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/evidence"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/reporting"
)

func cloneManifestForCompare(t *testing.T, m *evidence.Manifest) *evidence.Manifest {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal clone: %v", err)
	}
	var out evidence.Manifest
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("decode clone: %v", err)
	}
	return &out
}

func TestRenderSafety_RejectsSensitiveContentInBothRenderers(t *testing.T) {
	pass := domain.RunOutcomePass
	baseManifest := func() *evidence.Manifest {
		return makeSyntheticManifest(domain.RunStateCompleted, &pass, []domain.AssertionResult{
			{
				AssertionID: "assertion_seq",
				Kind:        domain.AssertionKindLedgerSequenceEquals,
				Expected:    json.Number("100"),
				Observed:    json.Number("100"),
				Status:      domain.AssertionStatusPass,
				Explanation: "matched",
			},
		})
	}

	fakeStellarSeed := "S" + strings.Repeat("A", 55)

	tests := []struct {
		name   string
		secret string
		mutate func(m *evidence.Manifest)
	}{
		{
			name:   "unix_home_path_in_provenance_notes",
			secret: "/home/alice/secrets/key.txt",
			mutate: func(m *evidence.Manifest) {
				m.Provenance.Notes = "loaded from /home/alice/secrets/key.txt"
			},
		},
		{
			name:   "windows_drive_path_in_assertion_explanation",
			secret: `C:\Users\bob\ledger.xdr`,
			mutate: func(m *evidence.Manifest) {
				m.Assertions[0].Explanation = `failed reading C:\Users\bob\ledger.xdr`
			},
		},
		{
			name:   "windows_unc_path_in_limitation",
			secret: `\\fileserver\share\secret.txt`,
			mutate: func(m *evidence.Manifest) {
				m.Diagnostics.Limitations = []string{`read from \\fileserver\share\secret.txt`}
			},
		},
		{
			name:   "file_uri_local_path_in_notes",
			secret: "file:///etc/passwd",
			mutate: func(m *evidence.Manifest) {
				m.Provenance.Notes = "source file:///etc/passwd"
			},
		},
		{
			name:   "bearer_token_in_diagnostic_message",
			secret: "Bearer tok_1234567890abcdef",
			mutate: func(m *evidence.Manifest) {
				m.Diagnostics.AdapterDiagnostics[0].Message = "header Authorization: Bearer tok_1234567890abcdef"
			},
		},
		{
			name:   "credential_assignment_in_limitation",
			secret: "sk_test_supersecretvalue",
			mutate: func(m *evidence.Manifest) {
				m.Diagnostics.Limitations = []string{"api_key=sk_test_supersecretvalue"}
			},
		},
		{
			name:   "pem_private_key_header_in_free_text",
			secret: "-----BEGIN RSA PRIVATE KEY-----",
			mutate: func(m *evidence.Manifest) {
				m.Provenance.Notes = "saw -----BEGIN RSA PRIVATE KEY----- in log"
			},
		},
		{
			name:   "stellar_secret_key_encoding",
			secret: fakeStellarSeed,
			mutate: func(m *evidence.Manifest) {
				m.Assertions[0].Observed = fakeStellarSeed
			},
		},
		{
			name:   "source_url_sensitive_query_parameter",
			secret: "sig_abc123xyz",
			mutate: func(m *evidence.Manifest) {
				u := "https://example.invalid/ledger.xdr?X-Amz-Signature=sig_abc123xyz"
				m.Provenance.URL = &u
			},
		},
		{
			name:   "source_url_userinfo_credentials",
			secret: "hunter2password",
			mutate: func(m *evidence.Manifest) {
				u := "https://alice:hunter2password@example.invalid/ledger.xdr"
				m.Provenance.URL = &u
			},
		},
		{
			name:   "typed_map_string_string_bearer_in_expected",
			secret: "Bearer nested_typed_secret_987654",
			mutate: func(m *evidence.Manifest) {
				m.Assertions[0].Expected = map[string]string{
					"header": "Bearer nested_typed_secret_987654",
				}
			},
		},
		{
			name:   "typed_slice_map_string_string_path_in_observed",
			secret: "/tmp/private/observed.xdr",
			mutate: func(m *evidence.Manifest) {
				m.Assertions[0].Observed = []map[string]string{
					{"path": "/tmp/private/observed.xdr"},
				}
			},
		},
		{
			name:   "generic_nested_map_and_array_in_observed",
			secret: "secret_token_in_array_xyz",
			mutate: func(m *evidence.Manifest) {
				m.Assertions[0].Observed = map[string]any{
					"items": []any{
						map[string]any{"note": "password=secret_token_in_array_xyz"},
					},
				}
			},
		},
		{
			name:   "credential_property_api_key_with_opaque_value",
			secret: "opaque-value-1234",
			mutate: func(m *evidence.Manifest) {
				m.Assertions[0].Observed = map[string]string{
					"api_key": "opaque-value-1234",
				}
			},
		},
		{
			name:   "camelCase_accessToken_property",
			secret: "opaque-access-token-val",
			mutate: func(m *evidence.Manifest) {
				m.Assertions[0].Observed = map[string]string{
					"accessToken": "opaque-access-token-val",
				}
			},
		},
		{
			name:   "camelCase_clientSecret_property",
			secret: "opaque-client-secret-val",
			mutate: func(m *evidence.Manifest) {
				m.Assertions[0].Expected = map[string]string{
					"clientSecret": "opaque-client-secret-val",
				}
			},
		},
		{
			name:   "camelCase_privateKey_property",
			secret: "opaque-private-key-val",
			mutate: func(m *evidence.Manifest) {
				m.Assertions[0].Observed = map[string]string{
					"privateKey": "opaque-private-key-val",
				}
			},
		},
		{
			name:   "camelCase_secretAccessKey_property",
			secret: "opaque-aws-secret-val",
			mutate: func(m *evidence.Manifest) {
				m.Assertions[0].Observed = map[string]string{
					"secretAccessKey": "opaque-aws-secret-val",
				}
			},
		},
		{
			name:   "camelCase_sessionToken_property",
			secret: "opaque-session-token-val",
			mutate: func(m *evidence.Manifest) {
				m.Assertions[0].Observed = map[string]string{
					"sessionToken": "opaque-session-token-val",
				}
			},
		},
		{
			name:   "camelCase_url_query_accessToken",
			secret: "tok-query-val-123",
			mutate: func(m *evidence.Manifest) {
				u := "https://example.invalid/ledger.xdr?accessToken=tok-query-val-123"
				m.Provenance.URL = &u
			},
		},
		{
			name:   "camelCase_url_query_apiKey",
			secret: "apikey-query-val-456",
			mutate: func(m *evidence.Manifest) {
				u := "https://example.invalid/ledger.xdr?apiKey=apikey-query-val-456"
				m.Provenance.URL = &u
			},
		},
		{
			name:   "invalid_timestamp_containing_local_path_does_not_leak",
			secret: "/home/alice/leak-in-timestamp",
			mutate: func(m *evidence.Manifest) {
				m.Run.StartedAt = "/home/alice/leak-in-timestamp"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := baseManifest()
			tc.mutate(m)
			snapshot := cloneManifestForCompare(t, m)

			_, errJSON := reporting.RenderJSON(m)
			if !errors.Is(errJSON, reporting.ErrUnsafeReportContent) {
				t.Fatalf("RenderJSON error = %v, want ErrUnsafeReportContent", errJSON)
			}
			if strings.Contains(errJSON.Error(), tc.secret) {
				t.Errorf("RenderJSON error leaked sensitive value %q: %v", tc.secret, errJSON)
			}

			_, errMD := reporting.RenderMarkdown(m)
			if !errors.Is(errMD, reporting.ErrUnsafeReportContent) {
				t.Fatalf("RenderMarkdown error = %v, want ErrUnsafeReportContent", errMD)
			}
			if strings.Contains(errMD.Error(), tc.secret) {
				t.Errorf("RenderMarkdown error leaked sensitive value %q: %v", tc.secret, errMD)
			}

			// Full deep comparison: manifest must not be mutated during rejected render.
			after := cloneManifestForCompare(t, m)
			if !reflect.DeepEqual(snapshot, after) {
				t.Errorf("manifest was mutated during rejected render")
			}
		})
	}

	t.Run("safe_public_https_url_and_corpus_relative_path_accepted", func(t *testing.T) {
		m := baseManifest()
		u := "https://aws-public-blockchain.s3.amazonaws.com/v1.1/stellar/ledgers/pubnet/FC159281--65695102.xdr.zst"
		m.Provenance.URL = &u
		m.Input.ObjectPath = "pubnet/ledgers/65695102.xdr"

		if _, err := reporting.RenderJSON(m); err != nil {
			t.Fatalf("RenderJSON rejected safe HTTPS URL / relative path: %v", err)
		}
		if _, err := reporting.RenderMarkdown(m); err != nil {
			t.Fatalf("RenderMarkdown rejected safe HTTPS URL / relative path: %v", err)
		}
	})
}

func TestRenderMarkdown_CompleteProvenanceFields(t *testing.T) {
	pass := domain.RunOutcomePass

	t.Run("present_repository_issue_and_pr", func(t *testing.T) {
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
		repo := "stellar/go-stellar-sdk"
		issue := 5123
		pr := 5199
		m.Provenance.Repository = &repo
		m.Provenance.IssueNumber = &issue
		m.Provenance.PullRequestNumber = &pr

		md, err := reporting.RenderMarkdown(m)
		if err != nil {
			t.Fatalf("RenderMarkdown failed: %v", err)
		}
		s := string(md)
		for _, want := range []string{
			"Source Repository:** stellar/go\\-stellar\\-sdk",
			"Source Issue Number:** 5123",
			"Source Pull Request Number:** 5199",
		} {
			if !strings.Contains(s, want) {
				t.Errorf("Markdown missing %q in:\n%s", want, s)
			}
		}
	})

	t.Run("absent_optional_provenance_rendered_as_not_recorded", func(t *testing.T) {
		m := makeSyntheticManifest(domain.RunStateFailed, nil, []domain.AssertionResult{})
		m.Provenance.URL = nil
		m.Provenance.Repository = nil
		m.Provenance.IssueNumber = nil
		m.Provenance.PullRequestNumber = nil
		m.Provenance.RetrievedAt = nil

		md, err := reporting.RenderMarkdown(m)
		if err != nil {
			t.Fatalf("RenderMarkdown failed: %v", err)
		}
		s := string(md)
		for _, want := range []string{
			"Source URL:** not recorded",
			"Source Repository:** not recorded",
			"Source Issue Number:** not recorded",
			"Source Pull Request Number:** not recorded",
			"Source Retrieved At:** not recorded",
		} {
			if !strings.Contains(s, want) {
				t.Errorf("Markdown missing %q", want)
			}
		}
	})
}
