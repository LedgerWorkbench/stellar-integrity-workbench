# Adapter Compatibility Matrix

**Last Updated:** Phase 6, Unit 4

> **Note:** No Stellar XDR decoding or SDK compatibility claim is made in Unit 4. Actual `LedgerCloseMeta` decoding and semantic projection begin in Unit 5.

## 1. Registered Production Adapters

| Adapter ID | Binary | Language | Pinned SDK Dependency | Supported Operations | Status (Unit 4) |
|---|---|---|---|---|---|
| `go-ingest` | `go-ingest-adapter` | Go 1.25 | `github.com/stellar/go-stellar-sdk v0.9.0` | `decode_ledger_meta` | Protocol v1 wired; XDR decoding pending Unit 5 |

## 2. Planned Adapters (Not Yet Implemented)

- JavaScript XDR/SDK adapter (`@stellar/stellar-sdk`) — planned for later units.
- Rust XDR adapter (`stellar-xdr`) — planned for later units.

## 3. Test-Only Helpers

The `internal/adapter` test suite builds an isolated, test-only helper binary inside `TestMain` solely to verify `ProcessRunner` behavior (success, failure, malformed JSON, trailing JSON, identity/hash mismatches, timeout, cancellation, oversized stdout, and oversized stderr).

- This helper exists only in `internal/adapter/process_runner_test.go`.
- It is **never** registered in `NewProductionRegistry`.
- It is **not** a Stellar SDK or XDR conformance adapter.
