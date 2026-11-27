# Authentic Ledger Sample Verification: Pubnet Ledger 65695102

This document records the provenance, retrieval, extraction, Horizon cross-check, and repeated-run determinism verification for an authentic Stellar Public Network `LedgerCloseMeta` sample decoded by `go-ingest-adapter` (Unit 5).

> **Note:** The raw binary ledger file is intentionally **not** committed to this repository. Any developer can reproduce the exact bytes and verification results using the commands below.

## 1. Source

- **Dataset / Network:** Stellar Public Network (`pubnet`), AWS Open Data / AWS Public Blockchain Dataset (`v1.1/stellar/ledgers/pubnet`)
- **S3 Object URL:** `https://aws-public-blockchain.s3.amazonaws.com/v1.1/stellar/ledgers/pubnet/FC160BFF--65664000-65727999/FC159281--65695102.xdr.zst`
- **Dataset Configuration URL:** `https://aws-public-blockchain.s3.amazonaws.com/v1.1/stellar/ledgers/pubnet/.config.json`
  - Recorded configuration: `networkPassphrase = "Public Global Stellar Network ; September 2015"`, `compression = "zstd"`, `ledgersPerBatch = 1`
- **Compression Format:** Zstandard (`zstd`)
- **Retrieval Timestamp (UTC):** `2026-11-27T06:40:01Z`

## 2. Retrieval and Extraction

1. **Download compressed batch object:**
   ```bash
   curl -fsSL -o FC159281--65695102.xdr.zst \
     https://aws-public-blockchain.s3.amazonaws.com/v1.1/stellar/ledgers/pubnet/FC160BFF--65664000-65727999/FC159281--65695102.xdr.zst
   ```

2. **Decompress with `zstd`:**
   ```bash
   zstd -d -f FC159281--65695102.xdr.zst -o FC159281--65695102.xdr
   ```

3. **Extract individual `LedgerCloseMeta` using pinned SDK `v0.9.0`:**
   - Decoded `FC159281--65695102.xdr` as `xdr.LedgerCloseMetaBatch` via `xdr.SafeUnmarshal(data, &batch)`.
   - Extracted ledger `65695102` via `batch.GetLedger(65695102)`.
   - Preserved the extracted `xdr.LedgerCloseMeta` bytes to `lcm_65695102.xdr` (`0400` permissions) via `xdr.Marshal(&out, lcm)` without modifying ledger contents.

## 3. Hashes and Sizes

| Artifact | Size (bytes) | SHA-256 |
|---|---|---|
| `FC159281--65695102.xdr.zst` (compressed) | `175086` | `41db25fdc6dee9d255d5015db60ba9e7757abb5c090a04f0fe3544fd79b40fe5` |
| `FC159281--65695102.xdr` (decompressed batch) | `1653788` | `2caa44f52f2343d3cc0cdda8d9e6468531abf6709b8f1f1ac036cbc80b75012c` |
| `lcm_65695102.xdr` (extracted `LedgerCloseMeta`) | `1653776` | `0316872e732b0e341f69f92af0c589a1432309b6c4ef19860ee16423f738e9a3` |

## 4. Horizon Cross-Check

Cross-checked against Stellar Horizon (`https://horizon.stellar.org/ledgers/65695102`):

| Field | Horizon Value | Adapter `semantic_projection` Value | Match |
|---|---|---|---|
| `sequence` | `65695102` | `65695102` | Yes |
| `protocol_version` | `30` | `30` | Yes |
| `closed_at` / `close_time_unix` | `2026-11-27T06:39:42Z` | `1795761582` (`2026-11-27T06:39:42Z`) | Yes |
| `hash` / `ledger_hash` | `84970562651affde59710e669003362a7714e62fa8f3d8d4b4134efdabe17013` | `84970562651affde59710e669003362a7714e62fa8f3d8d4b4134efdabe17013` | Yes |
| `prev_hash` / `previous_ledger_hash` | `26d72118d1ac64ee4f470954ce1d0269acfb2d9e5983e83d6cba70581ad29209` | `26d72118d1ac64ee4f470954ce1d0269acfb2d9e5983e83d6cba70581ad29209` | Yes |
| `successful_transaction_count` | `197` | `197` | Yes |
| `failed_transaction_count` | `24` | `24` | Yes |
| Total transactions | `221` | `221` (`hashes_truncated: false`, `221` hashes) | Yes |

Additional decoded metadata:
- `xdr_type`: `"LedgerCloseMeta"`
- `outer_variant`: `2` (`LedgerCloseMetaV2`)

## 5. Repeated-Run Semantic Stability

Executed `go run ./cmd/go-ingest-adapter` twice against `lcm_65695102.xdr` using identical protocol requests (`schema_version: "1"`, `adapter_id: "go-ingest"`, `adapter_version: "dev"`, `sdk_version: "v0.9.0"`):

- **Run 1:** Exit code `0`, `execution.status = "completed"`, `decode.status = "success"`
- **Run 2:** Exit code `0`, `execution.status = "completed"`, `decode.status = "success"`
- **Canonical `semantic_projection` SHA-256 (sorted keys, compact JSON):**
  - Run 1: `ff238af52d50b2a816382ef7f1e09565b9afee306027aa576a5e3092e3582111`
  - Run 2: `ff238af52d50b2a816382ef7f1e09565b9afee306027aa576a5e3092e3582111`
- **Result:** Byte-for-byte identical `semantic_projection` across both runs (`Equal semantic_projection: True`).
