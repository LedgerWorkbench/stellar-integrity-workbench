package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func makeSyntheticBatchBytes(t *testing.T, sequences ...uint32) []byte {
	t.Helper()
	var metas []xdr.LedgerCloseMeta
	for _, seq := range sequences {
		metas = append(metas, xdr.LedgerCloseMeta{
			V: 1,
			V1: &xdr.LedgerCloseMetaV1{
				LedgerHeader: xdr.LedgerHeaderHistoryEntry{
					Header: xdr.LedgerHeader{
						LedgerVersion: 21,
						LedgerSeq:     xdr.Uint32(seq),
						ScpValue: xdr.StellarValue{
							CloseTime: xdr.TimePoint(1700000000 + uint64(seq)),
						},
					},
				},
				TxSet: xdr.GeneralizedTransactionSet{
					V: 1,
					V1TxSet: &xdr.TransactionSetV1{
						Phases: []xdr.TransactionPhase{},
					},
				},
			},
		})
	}

	var startSeq, endSeq xdr.Uint32
	if len(sequences) > 0 {
		startSeq = xdr.Uint32(sequences[0])
		endSeq = xdr.Uint32(sequences[len(sequences)-1])
	}

	batch := xdr.LedgerCloseMetaBatch{
		StartSequence:    startSeq,
		EndSequence:      endSeq,
		LedgerCloseMetas: metas,
	}

	var buf bytes.Buffer
	if _, err := xdr.Marshal(&buf, batch); err != nil {
		t.Fatalf("marshal synthetic batch: %v", err)
	}
	return buf.Bytes()
}

func TestRun_Success(t *testing.T) {
	dir := t.TempDir()
	batchPath := filepath.Join(dir, "batch.xdr")
	outPath := filepath.Join(dir, "lcm_65695102.xdr")

	batchBytes := makeSyntheticBatchBytes(t, 65695101, 65695102, 65695103)
	if err := os.WriteFile(batchPath, batchBytes, 0o600); err != nil {
		t.Fatalf("write batch: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--input", batchPath, "--ledger", "65695102", "--output", outPath}, &stdout)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var decoded xdr.LedgerCloseMeta
	if err := xdr.SafeUnmarshal(outBytes, &decoded); err != nil {
		t.Fatalf("SafeUnmarshal output: %v", err)
	}
	if decoded.LedgerSequence() != 65695102 {
		t.Errorf("decoded LedgerSequence() = %d, want 65695102", decoded.LedgerSequence())
	}

	sum := sha256.Sum256(outBytes)
	wantPrefix := "sha256=" + hex.EncodeToString(sum[:])
	if !strings.Contains(stdout.String(), wantPrefix) {
		t.Errorf("stdout %q missing %q", stdout.String(), wantPrefix)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(outPath)
		if err != nil {
			t.Fatalf("stat output: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o400 {
			t.Errorf("output permissions = %o, want 0400", perm)
		}
	}
}

func TestRun_Errors(t *testing.T) {
	dir := t.TempDir()
	batchPath := filepath.Join(dir, "batch.xdr")
	if err := os.WriteFile(batchPath, makeSyntheticBatchBytes(t, 100, 101), 0o600); err != nil {
		t.Fatalf("write batch: %v", err)
	}

	badPath := filepath.Join(dir, "bad.xdr")
	if err := os.WriteFile(badPath, []byte("not-valid-xdr"), 0o600); err != nil {
		t.Fatalf("write bad batch: %v", err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing_ledger_in_batch",
			args: []string{"--input", batchPath, "--ledger", "999", "--output", filepath.Join(dir, "out1.xdr")},
		},
		{
			name: "malformed_batch_xdr",
			args: []string{"--input", badPath, "--ledger", "100", "--output", filepath.Join(dir, "out2.xdr")},
		},
		{
			name: "missing_input_file",
			args: []string{"--input", filepath.Join(dir, "missing.xdr"), "--ledger", "100", "--output", filepath.Join(dir, "out3.xdr")},
		},
		{
			name: "missing_required_flags",
			args: []string{"--ledger", "100"},
		},
		{
			name: "zero_ledger_sequence",
			args: []string{"--input", batchPath, "--ledger", "0", "--output", filepath.Join(dir, "out4.xdr")},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := run(tc.args, &stdout); err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestRun_RefusesExistingOutputFile(t *testing.T) {
	dir := t.TempDir()
	batchPath := filepath.Join(dir, "batch.xdr")
	outPath := filepath.Join(dir, "existing.xdr")

	if err := os.WriteFile(batchPath, makeSyntheticBatchBytes(t, 65695102), 0o600); err != nil {
		t.Fatalf("write batch: %v", err)
	}

	originalContent := []byte("preserve-existing-content")
	if err := os.WriteFile(outPath, originalContent, 0o640); err != nil {
		t.Fatalf("write existing output: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--input", batchPath, "--ledger", "65695102", "--output", outPath}, &stdout)
	if err == nil {
		t.Fatal("expected error when output file already exists, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected clear 'already exists' error, got: %v", err)
	}

	gotContent, readErr := os.ReadFile(outPath)
	if readErr != nil {
		t.Fatalf("read existing output: %v", readErr)
	}
	if !bytes.Equal(gotContent, originalContent) {
		t.Errorf("existing output was modified: got %q, want %q", gotContent, originalContent)
	}

	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(outPath)
		if statErr != nil {
			t.Fatalf("stat existing output: %v", statErr)
		}
		if perm := info.Mode().Perm(); perm != 0o640 {
			t.Errorf("existing output permissions changed: got %o, want 0640", perm)
		}
	}
}

func TestRun_RefusesExistingOutputSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	dir := t.TempDir()
	batchPath := filepath.Join(dir, "batch.xdr")
	targetPath := filepath.Join(dir, "target.xdr")
	linkPath := filepath.Join(dir, "link.xdr")

	if err := os.WriteFile(batchPath, makeSyntheticBatchBytes(t, 65695102), 0o600); err != nil {
		t.Fatalf("write batch: %v", err)
	}
	targetContent := []byte("do-not-overwrite-target")
	if err := os.WriteFile(targetPath, targetContent, 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--input", batchPath, "--ledger", "65695102", "--output", linkPath}, &stdout)
	if err == nil {
		t.Fatal("expected error when output path is an existing symlink, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected clear 'already exists' error for symlink, got: %v", err)
	}

	gotTarget, readErr := os.ReadFile(targetPath)
	if readErr != nil {
		t.Fatalf("read symlink target: %v", readErr)
	}
	if !bytes.Equal(gotTarget, targetContent) {
		t.Errorf("symlink target was modified: got %q, want %q", gotTarget, targetContent)
	}
}
