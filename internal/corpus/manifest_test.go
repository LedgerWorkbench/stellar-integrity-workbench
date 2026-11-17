package corpus_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/corpus"
	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
)

func TestManifest_ToDomain(t *testing.T) {
	url := "https://example.com/archive/ledger-12345.xdr"
	retrievedAtStr := "2026-11-17T09:00:00Z"
	protocolVer := 21
	ledgerSeq := int64(12345)
	coreVer := "v21.1.0"
	indexerVer := "v1.2.3"

	m := &corpus.Manifest{
		SchemaVersion:  "1.0.0",
		CaseID:         "case_test_001",
		Title:          "Test Case",
		Summary:        "A test case summary",
		Classification: corpus.ClassificationSyntheticFixture,
		Source: corpus.Source{
			Kind:        "synthetic_construction",
			URL:         &url,
			RetrievedAt: &retrievedAtStr,
			Notes:       "Constructed for unit testing",
		},
		Network: corpus.Network{
			Name:            "Synthetic Standalone Network",
			ProtocolVersion: &protocolVer,
			LedgerSequence:  &ledgerSeq,
		},
		Input: corpus.Input{
			Format:     "LedgerCloseMeta",
			Encoding:   "xdr_binary",
			SHA256:     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			ByteLength: 100,
			ObjectPath: "fixtures/test.xdr",
		},
		Expectations: []corpus.Expectation{
			{
				AssertionID: "assertion_seq_check",
				Kind:        "ledger_sequence_equals",
				Expected:    12345,
				Description: "Verifies decoded ledger sequence",
			},
		},
		ImplementationContext: corpus.ImplementationContext{
			StellarCoreVersion: &coreVer,
			SDKVersions: map[string]string{
				"stellar-go-sdk": "v0.9.0",
			},
			IndexerVersion: &indexerVer,
		},
		Limitations: []string{"Synthetic input"},
	}

	c, err := m.ToDomain()
	if err != nil {
		t.Fatalf("ToDomain() failed: %v", err)
	}

	if c.CaseID != m.CaseID {
		t.Errorf("CaseID mismatch: got %q, want %q", c.CaseID, m.CaseID)
	}
	if c.Classification != domain.ClassificationSyntheticFixture {
		t.Errorf("Classification mismatch: got %q", c.Classification)
	}
	if c.Source.RetrievedAt == nil {
		t.Fatalf("expected non-nil RetrievedAt")
	}
	expectedTime, _ := time.Parse(time.RFC3339, retrievedAtStr)
	if !c.Source.RetrievedAt.Equal(expectedTime) {
		t.Errorf("RetrievedAt mismatch: got %v, want %v", c.Source.RetrievedAt, expectedTime)
	}
	if *c.Network.LedgerSequence != ledgerSeq {
		t.Errorf("LedgerSequence mismatch: got %d, want %d", *c.Network.LedgerSequence, ledgerSeq)
	}
	if len(c.Expectations) != 1 {
		t.Fatalf("expected 1 expectation, got %d", len(c.Expectations))
	}
	if c.Expectations[0].Kind != domain.AssertionKindLedgerSequenceEquals {
		t.Errorf("Expectation kind mismatch: got %q", c.Expectations[0].Kind)
	}
}

func TestManifest_ToDomain_NilHandling(t *testing.T) {
	m := &corpus.Manifest{
		SchemaVersion:  "1.0.0",
		CaseID:         "case_null_values",
		Title:          "Null values test",
		Summary:        "Testing nullable fields",
		Classification: corpus.ClassificationSyntheticFixture,
		Source: corpus.Source{
			Kind:  "synthetic_construction",
			Notes: "No timestamps or URLs",
		},
		Network: corpus.Network{
			Name: "Synthetic Standalone Network",
		},
		Input: corpus.Input{
			Format:     "LedgerCloseMeta",
			Encoding:   "xdr_binary",
			SHA256:     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			ByteLength: 0,
			ObjectPath: "fixtures/empty.xdr",
		},
		Expectations: []corpus.Expectation{
			{
				AssertionID: "assertion_type_check",
				Kind:        "decoded_type_equals",
				Expected:    "LedgerCloseMeta",
				Description: "Verifies type",
			},
		},
		ImplementationContext: corpus.ImplementationContext{
			SDKVersions: map[string]string{},
		},
		Limitations: []string{},
	}

	c, err := m.ToDomain()
	if err != nil {
		t.Fatalf("ToDomain() failed: %v", err)
	}

	if c.Source.RetrievedAt != nil {
		t.Errorf("expected nil RetrievedAt, got %v", c.Source.RetrievedAt)
	}
	if c.Network.ProtocolVersion != nil {
		t.Errorf("expected nil ProtocolVersion, got %v", c.Network.ProtocolVersion)
	}
	if c.Network.LedgerSequence != nil {
		t.Errorf("expected nil LedgerSequence, got %v", c.Network.LedgerSequence)
	}
	if c.ImplementationContext.StellarCoreVersion != nil {
		t.Errorf("expected nil StellarCoreVersion, got %v", c.ImplementationContext.StellarCoreVersion)
	}
}

func TestManifest_ToDomain_InvalidKind(t *testing.T) {
	m := &corpus.Manifest{
		SchemaVersion:  "1.0.0",
		CaseID:         "case_invalid_kind",
		Title:          "Invalid Kind Test",
		Summary:        "Summary",
		Classification: corpus.ClassificationSyntheticFixture,
		Source: corpus.Source{
			Kind:  "synthetic_construction",
			Notes: "Notes",
		},
		Network: corpus.Network{Name: "Synthetic Standalone Network"},
		Input: corpus.Input{
			Format:     "LedgerCloseMeta",
			Encoding:   "xdr_binary",
			SHA256:     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			ByteLength: 10,
			ObjectPath: "path",
		},
		Expectations: []corpus.Expectation{
			{
				AssertionID: "assertion_invalid",
				Kind:        "unsupported_custom_kind",
				Expected:    123,
				Description: "Invalid",
			},
		},
		ImplementationContext: corpus.ImplementationContext{SDKVersions: map[string]string{}},
	}

	_, err := m.ToDomain()
	if err == nil {
		t.Fatalf("expected error for unsupported assertion kind")
	}
	if !strings.Contains(err.Error(), "unsupported assertion kind") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestManifest_DeserializationRejectsUnknownFields(t *testing.T) {
	jsonWithUnknown := `{
		"schema_version": "1.0.0",
		"case_id": "case_unknown_field",
		"title": "Title",
		"summary": "Summary",
		"classification": "synthetic_fixture",
		"unknown_field": "disallowed",
		"source": {"kind": "synthetic_construction", "notes": "note"},
		"network": {"name": "Synthetic Standalone Network", "protocol_version": null, "ledger_sequence": null},
		"input": {
			"format": "LedgerCloseMeta",
			"encoding": "xdr_binary",
			"sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"byte_length": 0,
			"object_path": "path"
		},
		"expectations": [
			{"assertion_id": "assertion_01", "kind": "decoded_type_equals", "expected": "LedgerCloseMeta", "description": "desc"}
		],
		"implementation_context": {"stellar_core_version": null, "sdk_versions": {}, "indexer_version": null},
		"limitations": []
	}`

	decoder := json.NewDecoder(strings.NewReader(jsonWithUnknown))
	decoder.DisallowUnknownFields()
	var m corpus.Manifest
	err := decoder.Decode(&m)
	if err == nil {
		t.Fatalf("expected error decoding unknown field with DisallowUnknownFields")
	}
	if !strings.Contains(err.Error(), "unknown_field") {
		t.Errorf("expected error mentioning unknown field, got: %v", err)
	}
}

func TestValidateSyntheticNetworkName(t *testing.T) {
	validNames := []string{
		"Synthetic Standalone Network",
		"synthetic test network",
		"Standalone Test Cluster",
		"Local Synthetic Sandbox",
		"SYNTHETIC",
		"standalone",
	}

	for _, name := range validNames {
		t.Run("valid_"+name, func(t *testing.T) {
			if err := corpus.ValidateSyntheticNetworkName(name); err != nil {
				t.Errorf("ValidateSyntheticNetworkName(%q) returned unexpected error: %v", name, err)
			}
		})
	}

	invalidNames := []struct {
		name        string
		expectedErr string
	}{
		{"", "must not be empty"},
		{"   ", "must not be empty"},
		{"Mainnet", "misleading public or shared network identity"},
		{"Stellar Mainnet", "misleading public or shared network identity"},
		{"Public Global Stellar Network", "misleading public or shared network identity"},
		{"Testnet", "misleading public or shared network identity"},
		{"Stellar Testnet", "misleading public or shared network identity"},
		{"Futurenet", "misleading public or shared network identity"},
		{"Stellar Futurenet", "misleading public or shared network identity"},
		{"Synthetic Mainnet", "misleading public or shared network identity"},
		{"Standalone Testnet", "misleading public or shared network identity"},
		{"Custom Private Network", "must positively identify as 'synthetic' or 'standalone'"},
		{"Stellar Local Core", "must positively identify as 'synthetic' or 'standalone'"},
	}

	for _, tc := range invalidNames {
		t.Run("invalid_"+tc.name, func(t *testing.T) {
			err := corpus.ValidateSyntheticNetworkName(tc.name)
			if err == nil {
				t.Errorf("ValidateSyntheticNetworkName(%q) expected error containing %q, got nil", tc.name, tc.expectedErr)
			} else if !strings.Contains(err.Error(), tc.expectedErr) {
				t.Errorf("ValidateSyntheticNetworkName(%q) expected error containing %q, got %v", tc.name, tc.expectedErr, err)
			}
		})
	}
}
