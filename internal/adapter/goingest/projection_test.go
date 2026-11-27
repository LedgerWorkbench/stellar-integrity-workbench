package goingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestBuildSemanticProjection_FieldsAndDeterminism(t *testing.T) {
	lcm, raw := makeSyntheticLCMv1(t, []bool{true, false, true})
	decoded, err := DecodeLedgerCloseMeta(raw)
	if err != nil {
		t.Fatalf("DecodeLedgerCloseMeta failed: %v", err)
	}

	p1, err := BuildSemanticProjection(decoded)
	if err != nil {
		t.Fatalf("BuildSemanticProjection failed: %v", err)
	}
	p2, err := BuildSemanticProjection(decoded)
	if err != nil {
		t.Fatalf("second BuildSemanticProjection failed: %v", err)
	}

	if p1.ProjectionVersion != ProjectionVersion {
		t.Errorf("ProjectionVersion = %q, want %q", p1.ProjectionVersion, ProjectionVersion)
	}
	if p1.XDRType != "LedgerCloseMeta" {
		t.Errorf("XDRType = %q, want %q", p1.XDRType, "LedgerCloseMeta")
	}
	if p1.OuterVariant != 1 {
		t.Errorf("OuterVariant = %d, want 1", p1.OuterVariant)
	}
	if p1.Ledger.Sequence != 200 {
		t.Errorf("Sequence = %d, want 200", p1.Ledger.Sequence)
	}
	if p1.Ledger.ProtocolVersion != 21 {
		t.Errorf("ProtocolVersion = %d, want 21", p1.Ledger.ProtocolVersion)
	}
	if p1.Ledger.CloseTimeUnix != 1700000200 {
		t.Errorf("CloseTimeUnix = %d, want 1700000200", p1.Ledger.CloseTimeUnix)
	}
	if want := lcm.LedgerHash().HexString(); p1.Ledger.LedgerHash != want {
		t.Errorf("LedgerHash = %q, want %q", p1.Ledger.LedgerHash, want)
	}
	if want := lcm.PreviousLedgerHash().HexString(); p1.Ledger.PreviousLedgerHash != want {
		t.Errorf("PreviousLedgerHash = %q, want %q", p1.Ledger.PreviousLedgerHash, want)
	}

	if p1.Transactions.Total != 3 {
		t.Errorf("Total = %d, want 3", p1.Transactions.Total)
	}
	if p1.Transactions.Successful != 2 {
		t.Errorf("Successful = %d, want 2", p1.Transactions.Successful)
	}
	if p1.Transactions.Failed != 1 {
		t.Errorf("Failed = %d, want 1", p1.Transactions.Failed)
	}
	if p1.Transactions.HashesTruncated {
		t.Error("HashesTruncated = true, want false")
	}
	if len(p1.Transactions.Hashes) != 3 {
		t.Fatalf("len(Hashes) = %d, want 3", len(p1.Transactions.Hashes))
	}
	for i := 0; i < 3; i++ {
		if want := lcm.TransactionHash(i).HexString(); p1.Transactions.Hashes[i] != want {
			t.Errorf("Hashes[%d] = %q, want %q", i, p1.Transactions.Hashes[i], want)
		}
	}

	// Deterministic JSON output
	j1, err := json.Marshal(p1)
	if err != nil {
		t.Fatalf("marshal p1: %v", err)
	}
	j2, err := json.Marshal(p2)
	if err != nil {
		t.Fatalf("marshal p2: %v", err)
	}
	if !bytes.Equal(j1, j2) {
		t.Errorf("projection JSON not deterministic:\n%s\nvs\n%s", j1, j2)
	}
}

func TestBuildSemanticProjection_HashTruncation(t *testing.T) {
	_, raw := makeSyntheticLCMv2(t, []bool{true, false, true, false, true})
	decoded, err := DecodeLedgerCloseMeta(raw)
	if err != nil {
		t.Fatalf("DecodeLedgerCloseMeta failed: %v", err)
	}

	proj, err := BuildSemanticProjectionWithLimit(decoded, 2)
	if err != nil {
		t.Fatalf("BuildSemanticProjectionWithLimit failed: %v", err)
	}
	if proj.Transactions.Total != 5 {
		t.Errorf("Total = %d, want 5", proj.Transactions.Total)
	}
	if proj.Transactions.Successful != 3 {
		t.Errorf("Successful = %d, want 3", proj.Transactions.Successful)
	}
	if proj.Transactions.Failed != 2 {
		t.Errorf("Failed = %d, want 2", proj.Transactions.Failed)
	}
	if !proj.Transactions.HashesTruncated {
		t.Error("HashesTruncated = false, want true")
	}
	if len(proj.Transactions.Hashes) != 2 {
		t.Errorf("len(Hashes) = %d, want 2", len(proj.Transactions.Hashes))
	}
}

func TestBuildSemanticProjection_ZeroTransactions(t *testing.T) {
	_, raw := makeSyntheticLCMv1(t, nil)
	decoded, err := DecodeLedgerCloseMeta(raw)
	if err != nil {
		t.Fatalf("DecodeLedgerCloseMeta failed: %v", err)
	}

	proj, err := BuildSemanticProjection(decoded)
	if err != nil {
		t.Fatalf("BuildSemanticProjection failed: %v", err)
	}
	if proj.Transactions.Total != 0 || proj.Transactions.Successful != 0 || proj.Transactions.Failed != 0 {
		t.Errorf("unexpected counts for 0-tx ledger: %+v", proj.Transactions)
	}
	if proj.Transactions.Hashes == nil || len(proj.Transactions.Hashes) != 0 {
		t.Errorf("expected non-nil empty Hashes slice, got %#v", proj.Transactions.Hashes)
	}
	if proj.Transactions.HashesTruncated {
		t.Error("HashesTruncated = true, want false")
	}
}

func TestBuildSemanticProjection_InvalidInputs(t *testing.T) {
	if _, err := BuildSemanticProjection(nil); !errors.Is(err, ErrProjectionFailed) {
		t.Errorf("expected ErrProjectionFailed for nil input, got %v", err)
	}

	_, raw := makeSyntheticLCMv0(t)
	decoded, err := DecodeLedgerCloseMeta(raw)
	if err != nil {
		t.Fatalf("DecodeLedgerCloseMeta failed: %v", err)
	}
	if _, err := BuildSemanticProjectionWithLimit(decoded, -1); !errors.Is(err, ErrProjectionFailed) {
		t.Errorf("expected ErrProjectionFailed for negative limit, got %v", err)
	}
}
