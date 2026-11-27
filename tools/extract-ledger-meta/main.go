// Command extract-ledger-meta decodes an XDR-encoded LedgerCloseMetaBatch
// using the pinned official Stellar Go SDK (v0.9.0), extracts a single
// LedgerCloseMeta by ledger sequence, verifies its sequence, and serializes
// the decoded LedgerCloseMeta value to a read-only output file.
//
// Note: This helper extracts and serializes the decoded LedgerCloseMeta value
// via xdr.Marshal rather than slicing raw bytes from the compressed source
// object.
//
// Usage:
//
//	go run ./tools/extract-ledger-meta --input FC159281--65695102.xdr --ledger 65695102 --output lcm_65695102.xdr
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "extract-ledger-meta: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("extract-ledger-meta", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		inputPath  string
		outputPath string
		ledgerSeq  uint64
	)
	fs.StringVar(&inputPath, "input", "", "path to decompressed LedgerCloseMetaBatch XDR file")
	fs.Uint64Var(&ledgerSeq, "ledger", 0, "ledger sequence number to extract")
	fs.StringVar(&outputPath, "output", "", "path to write serialized LedgerCloseMeta XDR file")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if inputPath == "" {
		return errors.New("--input is required")
	}
	if outputPath == "" {
		return errors.New("--output is required")
	}
	if ledgerSeq == 0 || ledgerSeq > uint64(^uint32(0)) {
		return fmt.Errorf("--ledger must be a positive uint32 sequence, got %d", ledgerSeq)
	}

	n, sumHex, err := extractLedgerCloseMeta(inputPath, uint32(ledgerSeq), outputPath)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(stdout, "len=%d sha256=%s output=%s\n", n, sumHex, outputPath)
	return err
}

// extractLedgerCloseMeta decodes a LedgerCloseMetaBatch XDR file with
// xdr.SafeUnmarshal, extracts the requested ledger via batch.GetLedger,
// verifies its sequence, and serializes the decoded LedgerCloseMeta value
// using xdr.Marshal to outputPath with 0400 permissions.
func extractLedgerCloseMeta(inputPath string, ledgerSeq uint32, outputPath string) (int, string, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return 0, "", fmt.Errorf("read input %q: %w", inputPath, err)
	}

	var batch xdr.LedgerCloseMetaBatch
	if err := xdr.SafeUnmarshal(data, &batch); err != nil {
		return 0, "", fmt.Errorf("unmarshal LedgerCloseMetaBatch: %w", err)
	}

	lcm, err := batch.GetLedger(ledgerSeq)
	if err != nil {
		return 0, "", fmt.Errorf("extract ledger %d from batch: %w", ledgerSeq, err)
	}
	if got := lcm.LedgerSequence(); got != ledgerSeq {
		return 0, "", fmt.Errorf("extracted ledger sequence mismatch: got %d, want %d", got, ledgerSeq)
	}

	var out bytes.Buffer
	if _, err := xdr.Marshal(&out, lcm); err != nil {
		return 0, "", fmt.Errorf("marshal extracted LedgerCloseMeta: %w", err)
	}

	if err := writeExclusiveFile(outputPath, out.Bytes(), 0o400); err != nil {
		return 0, "", err
	}

	sum := sha256.Sum256(out.Bytes())
	return out.Len(), hex.EncodeToString(sum[:]), nil
}

// writeExclusiveFile creates outputPath exclusively (refusing existing files
// and symlinks) with the given mode, writes all bytes, syncs, and closes the
// file. If writing, syncing, or closing fails, the partially created file is
// removed.
func writeExclusiveFile(outputPath string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output path %q already exists (refusing to overwrite): %w", outputPath, err)
		}
		return fmt.Errorf("create output %q: %w", outputPath, err)
	}

	cleanup := func(cause error) error {
		if rmErr := os.Remove(outputPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return fmt.Errorf("%w (also failed to remove partial output %q: %v)", cause, outputPath, rmErr)
		}
		return cause
	}

	written := 0
	for written < len(data) {
		n, wErr := f.Write(data[written:])
		written += n
		if wErr != nil {
			_ = f.Close()
			return cleanup(fmt.Errorf("write output %q: %w", outputPath, wErr))
		}
		if n == 0 {
			_ = f.Close()
			return cleanup(fmt.Errorf("write output %q: %w", outputPath, io.ErrShortWrite))
		}
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		return cleanup(fmt.Errorf("sync output %q: %w", outputPath, err))
	}
	if err := f.Close(); err != nil {
		return cleanup(fmt.Errorf("close output %q: %w", outputPath, err))
	}
	return nil
}
