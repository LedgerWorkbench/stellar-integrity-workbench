# Authentic Ledger Sample Assertion Evaluation: Pubnet Ledger 65695102

This document records the Unit 6 assertion evaluation results for the authentic Stellar Public Network `LedgerCloseMeta` sample (`65695102`) documented in `docs/validation/ledger-sample-65695102.md`.

## 1. Provenance and Observation Boundaries

- **Values actually read from the authentic adapter output (`go run ./cmd/go-ingest-adapter`):**
  - `xdr_type`: `"LedgerCloseMeta"`
  - `outer_variant`: `2`
  - `ledger.sequence`: `65695102`
  - `ledger.protocol_version`: `30`
  - `transactions.total`: `221`
  - `transactions.hashes_truncated`: `false`
  - `transactions.hashes[0]`: `142ada46a60fdaaefaeeb389a9b6869e5f205a453d5e59811db5b8aac97646e1` (221 hashes total)
- **Values independently cross-checked in the provenance record (`docs/validation/ledger-sample-65695102.md`):**
  - Extracted `LedgerCloseMeta` SHA-256: `0316872e732b0e341f69f92af0c589a1432309b6c4ef19860ee16423f738e9a3` (`1653776` bytes)
  - Horizon (`https://horizon.stellar.org/ledgers/65695102`) cross-checked sequence (`65695102`), protocol version (`30`), close time (`1795761582`), and transaction count (`221`).
  - Canonical `semantic_projection` SHA-256: `ff238af52d50b2a816382ef7f1e09565b9afee306027aa576a5e3092e3582111`
- **Logical content references supplied to the evaluator:**
  - `logical://input/sha256/0316872e732b0e341f69f92af0c589a1432309b6c4ef19860ee16423f738e9a3`
  - `logical://adapter/go-ingest/semantic-projection/v1`
  - (Unit 7 will connect these logical references to the persisted evidence bundle.)
- **Values unavailable in this environment:**
  - Live S3/Horizon re-fetch was not performed during this evaluation pass; evaluation used the previously retrieved and locally verified `repro_lcm_65695102.xdr` sample and a fresh `go-ingest-adapter` execution.

## 2. Assertion Evaluation Results

Evaluated using `assertion.Evaluate` (`internal/assertion/engine.go`) with `SupportedKinds = assertion.GoIngestSupportedKinds()` (all 5 kinds):

| Assertion ID | Kind | Expected | Observed | Status | Explanation |
|---|---|---|---|---|---|
| `assertion_seq_65695102` | `ledger_sequence_equals` | `65695102` | `65695102` | `pass` | `observed ledger sequence 65695102 matches expected 65695102` |
| `assertion_txcount_65695102` | `transaction_count_equals` | `221` | `221` | `pass` | `observed transaction count 221 matches expected 221` |
| `assertion_txhash_65695102` | `transaction_hash_present` | `142ada46a60fdaaefaeeb389a9b6869e5f205a453d5e59811db5b8aac97646e1` | `142ada46a60fdaaefaeeb389a9b6869e5f205a453d5e59811db5b8aac97646e1` | `pass` | `expected transaction hash 142ada46a60fdaaefaeeb389a9b6869e5f205a453d5e59811db5b8aac97646e1 is present in observed hashes` |
| `assertion_type_65695102` | `decoded_type_equals` | `"LedgerCloseMeta"` | `"LedgerCloseMeta"` | `pass` | `observed decoded type "LedgerCloseMeta" matches expected "LedgerCloseMeta"` |
| `assertion_sha256_65695102` | `input_hash_equals` | `0316872e732b0e341f69f92af0c589a1432309b6c4ef19860ee16423f738e9a3` | `0316872e732b0e341f69f92af0c589a1432309b6c4ef19860ee16423f738e9a3` | `pass` | `trusted verified input SHA-256 0316872e732b0e341f69f92af0c589a1432309b6c4ef19860ee16423f738e9a3 matches expected` |

**Derived Run Outcome:** `pass` (`domain.RunOutcomePass`).
