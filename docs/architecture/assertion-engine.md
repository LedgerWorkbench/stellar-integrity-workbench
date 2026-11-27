# Assertion Evaluation Engine Architecture (Unit 6)

Package: `internal/assertion` (`internal/assertion/engine.go`), backed by domain types in `internal/domain`.

## 1. Registered Assertion Kinds

The engine evaluates exactly five typed assertion kinds (no generic expression language or user-defined scripts):

| Kind | Expected Value Type & Validation | Observed Source | Comparison Semantics |
|---|---|---|---|
| `ledger_sequence_equals` | Exact non-negative integer (`0..4294967295`); rejects `float64`, decimals, booleans, negative/out-of-range | `Projection.LedgerSequence` | Exact `int64` equality; `inconclusive` if nil |
| `transaction_count_equals` | Exact non-negative integer (`0..4294967295`); rejects `float64`, decimals, booleans, negative/out-of-range | `Projection.TransactionCount` | Exact `int64` equality; `inconclusive` if nil |
| `transaction_hash_present` | 64-character hexadecimal string (`[0-9a-fA-F]{64}`) | `Projection.TransactionHashes` + `TransactionHashesTruncated` | Case-insensitive hex lookup (see truncation rules below) |
| `decoded_type_equals` | Non-empty, non-whitespace string | `Projection.DecodedType` | Exact string equality; `inconclusive` if empty |
| `input_hash_equals` | 64-character lowercase hexadecimal SHA-256 (`[0-9a-f]{64}`) | `EvaluationInput.VerifiedInputSHA256` | Exact lowercase SHA-256 equality; `inconclusive` if empty |

## 2. Observed Value Mapping from Unit 5

`assertion.ProjectionFromGoIngest` maps `goingest.SemanticProjection` to `domain.SemanticProjection`:

| Unit 5 Adapter Field (`goingest`) | Domain Observation (`domain.SemanticProjection` / `EvaluationInput`) |
|---|---|
| `xdr_type` | `DecodedType` |
| `ledger.sequence` | `LedgerSequence` |
| `transactions.total` | `TransactionCount` |
| `transactions.hashes` | `TransactionHashes` (`nil` = unavailable; `[]string{}` = complete empty list) |
| `transactions.hashes_truncated` | `TransactionHashesTruncated` |
| Trusted verified input SHA-256 (e.g. `localfile.VerifiedInput.SHA256`) | `EvaluationInput.VerifiedInputSHA256` |

## 3. Result Statuses

- `pass`: Observed value is available, valid, and satisfies the expectation.
- `fail`: Observed value is available, valid, and does not satisfy the expectation.
- `skipped`: Assertion kind is valid, but the selected adapter did not declare support in `EvaluationInput.SupportedKinds`.
- `inconclusive`: Required observation is unavailable (e.g. nil projection, missing field, or hash absent from a truncated list).

Invalid expectations, duplicate/empty assertion IDs, invalid capability declarations, empty evidence references, or malformed observed values return a Go `error` rather than fabricating assertion results.

## 4. Transaction-Hash Truncation Behavior

Unit 5's `go-ingest` adapter bounds `transactions.hashes` to `MaxProjectedTxHashes = 256` and sets `hashes_truncated = true` when `total > 256`:

- Hash found in `TransactionHashes` -> `pass` (even when `TransactionHashesTruncated == true`).
- Hash not found and `TransactionHashesTruncated == false` -> `fail`.
- Hash not found and `TransactionHashesTruncated == true` -> `inconclusive` (never falsely reported as absent).
- `TransactionHashes == nil` -> `inconclusive`.

## 5. Input-Hash Trust Boundary

`input_hash_equals` evaluates against `EvaluationInput.VerifiedInputSHA256`, supplied by the trusted local input verification layer (`localfile.VerifiedInput.SHA256`), rather than relying solely on the adapter's echoed `Result.Input.SHA256`.

## 6. Evidence References & Deterministic Ordering

- `Evaluate` processes `expectations` in manifest slice order and returns `Results` in the exact same order.
- Supplied `EvaluationInput.EvidenceRefs` are validated (non-empty) and cloned into each `AssertionResult.EvidenceRefs`. If none are supplied, the field is omitted. Unit 7 will connect these logical references to the persisted evidence bundle.

## 7. Outcome Derivation & Separation from Execution State

`domain.DeriveOutcome` derives `domain.RunOutcome` from `Results`:

1. Any `fail` -> `regression` (takes precedence over `inconclusive` and `skipped`).
2. Any `inconclusive` (and no `fail`) -> `inconclusive`.
3. All `skipped` -> `not_applicable`.
4. All `pass` -> `pass`.
5. Mixed `pass` and `skipped` -> `inconclusive` (all v1 manifest expectations are treated as required; a skipped assertion never silently counts as passing).

Execution state (`completed`, `failed`, `cancelled` in `domain.RunState`) remains strictly separated from semantic conformance outcome (`domain.RunOutcome`). Operational errors, invalid XDR, or adapter crashes belong to execution state and are never converted into assertion failures.

## 8. Current Limitations & Deferred Work

- `go-ingest` is configured via `assertion.GoIngestSupportedKinds()` to support all five v1 assertion kinds; persistent per-adapter capability registries and full `workbench-worker` wiring are deferred to later units.
- Evidence bundle persistence and report generation are deferred to Unit 7+.
