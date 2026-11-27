# Adapter Compatibility Matrix

**Last Updated:** Phase 6, Unit 5

> **Note:** Claims below reflect only behaviour verified by the Unit 5 implementation and its test suite. Compatibility with all Stellar protocol versions or arbitrary future XDR variants is not claimed.

## 1. Registered Production Adapters

| Adapter ID | Binary | Language | Pinned SDK Dependency | Supported Operations | Status (Unit 5) |
|---|---|---|---|---|---|
| `go-ingest` | `go-ingest-adapter` | Go 1.25 | `github.com/stellar/go-stellar-sdk v0.9.0` | `decode_ledger_meta` | Implemented: `LedgerCloseMeta` V0/V1/V2 XDR decoding (`xdr.SafeUnmarshal`) and `projection_version: "1"` |

### `go-ingest` Verified Scope & Limitations
- **Verified:** Local regular-file/size/SHA-256 checks, `xdr.SafeUnmarshal` into `xdr.LedgerCloseMeta` (V0, V1, V2), `ingest/ledger.TransactionCounts`, and bounded semantic projection (up to 256 processing-order transaction hashes).
- **Limitations:** Does not invoke `ingest.NewLedgerTransactionReaderFromLedgerCloseMeta` because Protocol v1 does not carry a network passphrase. Unsupported outer/inner XDR variants are rejected with structured errors.

## 2. Planned Adapters (Not Yet Implemented)

- JavaScript XDR/SDK adapter (`@stellar/stellar-sdk`) — planned for later units.
- Rust XDR adapter (`stellar-xdr`) — planned for later units.

## 3. Test-Only Helpers

The `internal/adapter` test suite builds an isolated, test-only helper binary inside `TestMain` solely to verify `ProcessRunner` behavior (success, failure, malformed JSON, trailing JSON, identity/hash mismatches, timeout, cancellation, oversized stdout, and oversized stderr).

- This helper exists only in `internal/adapter/process_runner_test.go`.
- It is **never** registered in `NewProductionRegistry`.
- It is **not** a Stellar SDK or XDR conformance adapter.
