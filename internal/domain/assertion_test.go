package domain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LedgerWorkbench/stellar-integrity-workbench/internal/domain"
)

func TestAssertionKind_Validation(t *testing.T) {
	kinds := domain.AllAssertionKinds()
	if len(kinds) != 5 {
		t.Fatalf("expected 5 assertion kinds, got %d", len(kinds))
	}

	for _, k := range kinds {
		if !k.IsValid() {
			t.Errorf("expected kind %q to be valid", k)
		}
		if err := k.Validate(); err != nil {
			t.Errorf("expected Validate() nil for %q, got: %v", k, err)
		}
	}

	invalid := domain.AssertionKind("custom_expression")
	if invalid.IsValid() {
		t.Errorf("expected %q to be invalid", invalid)
	}
	if err := invalid.Validate(); err == nil {
		t.Errorf("expected Validate() error for %q", invalid)
	}
}

func TestAssertionStatus_Validation(t *testing.T) {
	statuses := domain.AllAssertionStatuses()
	if len(statuses) != 4 {
		t.Fatalf("expected 4 assertion statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if !s.IsValid() {
			t.Errorf("expected status %q to be valid", s)
		}
		if err := s.Validate(); err != nil {
			t.Errorf("expected Validate() nil for %q, got: %v", s, err)
		}
	}

	invalid := domain.AssertionStatus("errored")
	if invalid.IsValid() {
		t.Errorf("expected %q to be invalid", invalid)
	}
	if err := invalid.Validate(); err == nil {
		t.Errorf("expected Validate() error for %q", invalid)
	}
}

func TestParseExactInteger(t *testing.T) {
	t.Run("exact standard integer", func(t *testing.T) {
		n, err := domain.ParseExactInteger(json.Number("42"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 42 {
			t.Errorf("got %d, want 42", n)
		}
	})

	t.Run("exact integer larger than 2^53", func(t *testing.T) {
		// 2^53 + 1 = 9007199254740993 cannot be represented accurately in IEEE 754 float64
		// (float64 would round it to 9007199254740992).
		const largeIntStr = "9007199254740993"
		const expectedVal int64 = 9007199254740993

		n, err := domain.ParseExactInteger(json.Number(largeIntStr))
		if err != nil {
			t.Fatalf("unexpected error parsing 2^53+1: %v", err)
		}
		if n != expectedVal {
			t.Errorf("precision loss detected! got %d, want %d", n, expectedVal)
		}
	})

	t.Run("exact max int64", func(t *testing.T) {
		const maxInt64Str = "9223372036854775807"
		const expectedVal int64 = 9223372036854775807

		n, err := domain.ParseExactInteger(json.Number(maxInt64Str))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != expectedVal {
			t.Errorf("got %d, want %d", n, expectedVal)
		}
	})

	t.Run("reject decimal float in json.Number", func(t *testing.T) {
		_, err := domain.ParseExactInteger(json.Number("100.5"))
		if err == nil {
			t.Fatalf("expected error for decimal float, got nil")
		}
		if !strings.Contains(err.Error(), "decimal/float") {
			t.Errorf("expected error mentioning decimal/float, got: %v", err)
		}
	})

	t.Run("reject native float64", func(t *testing.T) {
		_, err := domain.ParseExactInteger(float64(100.0))
		if err == nil {
			t.Fatalf("expected error for float64, got nil")
		}
		if !strings.Contains(err.Error(), "floating point value") {
			t.Errorf("expected error mentioning floating point, got: %v", err)
		}
	})

	t.Run("reject string", func(t *testing.T) {
		_, err := domain.ParseExactInteger("not_a_number")
		if err == nil {
			t.Fatalf("expected error for string, got nil")
		}
	})

	t.Run("reject overflow beyond int64", func(t *testing.T) {
		_, err := domain.ParseExactInteger(json.Number("999999999999999999999999999999999999"))
		if err == nil {
			t.Fatalf("expected error for overflow beyond int64, got nil")
		}
		if !strings.Contains(err.Error(), "out of int64 range") {
			t.Errorf("expected error mentioning out of int64 range, got: %v", err)
		}
	})
}

func TestAssertionExpectation_TypePreservation(t *testing.T) {
	exp := domain.AssertionExpectation{
		AssertionID: "assertion_seq",
		Kind:        domain.AssertionKindLedgerSequenceEquals,
		Expected:    int64(50000000),
		Description: "Check sequence",
	}

	data, err := json.Marshal(exp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Verify that 50000000 is serialized as an unquoted JSON integer
	if !strings.Contains(string(data), `"expected":50000000`) {
		t.Errorf("expected unquoted integer in JSON, got: %s", string(data))
	}

	// Deserialize with UseNumber
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()

	var deserialized domain.AssertionExpectation
	if err := decoder.Decode(&deserialized); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	n, err := deserialized.ExpectedInt64()
	if err != nil {
		t.Fatalf("ExpectedInt64() failed: %v", err)
	}
	if n != 50000000 {
		t.Errorf("got %d, want 50000000", n)
	}
}
