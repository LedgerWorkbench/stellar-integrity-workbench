# Authentic Ledger Sample Verification: Pubnet Ledger 65695102

This document records the provenance, retrieval, reproducible extraction, Horizon cross-check, and repeated-run determinism verification for an authentic Stellar Public Network `LedgerCloseMeta` sample decoded by `go-ingest-adapter` (Unit 5).

> **Note:** The raw binary ledger file is intentionally **not** committed to this repository. Any developer can reproduce the exact bytes and verification results using the repository helper and commands below.

## 1. Source

- **Registry Source:** AWS Open Data Registry — AWS Public Blockchain (`https://registry.opendata.aws/aws-public-blockchain/`)
- **Registry License Reference:** `https://github.com/aws-samples/digital-assets-examples/blob/main/LICENSE` (this is the license reference listed by the AWS Public Blockchain Registry)
- **Dataset / Network:** Stellar Public Network (`pubnet`), `v1.1/stellar/ledgers/pubnet`
- **S3 Object URL:** `https://aws-public-blockchain.s3.amazonaws.com/v1.1/stellar/ledgers/pubnet/FC160BFF--65664000-65727999/FC159281--65695102.xdr.zst`
- **Dataset Configuration URL:** `https://aws-public-blockchain.s3.amazonaws.com/v1.1/stellar/ledgers/pubnet/.config.json`
  - Recorded configuration: `networkPassphrase = "Public Global Stellar Network ; September 2015"`, `compression = "zstd"`, `ledgersPerBatch = 1`
- **Compression Format:** Zstandard (`zstd`)
- **Retrieval Timestamp (UTC):** `2026-11-27T06:40:01Z` (recorded during the initial developer-environment retrieval; not re-fetched during the reproducibility pass)

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

3. **Extract individual `LedgerCloseMeta` using the repository helper (`tools/extract-ledger-meta/main.go`):**
   ```bash
   rm -f lcm_65695102.xdr
   go run ./tools/extract-ledger-meta \
     --input FC159281--65695102.xdr \
     --ledger 65695102 \
     --output lcm_65695102.xdr
   ```
   - Uses pinned `github.com/stellar/go-stellar-sdk v0.9.0`.
   - Decodes `FC159281--65695102.xdr` as `xdr.LedgerCloseMetaBatch` via `xdr.SafeUnmarshal`, extracts ledger `65695102` via `batch.GetLedger(65695102)`, and verifies `lcm.LedgerSequence() == 65695102`.
   - **Exclusive output requirement:** The helper opens `--output` with `O_WRONLY|O_CREATE|O_EXCL` (`0400` permissions) and refuses to overwrite an existing file or follow a symlink, so the output path must not already exist (remove any previous `lcm_65695102.xdr` with `rm -f lcm_65695102.xdr` before repeating extraction).
   - **Serialization note:** The helper extracts and serializes the decoded `xdr.LedgerCloseMeta` value using `xdr.Marshal`; the output is a re-serialized `LedgerCloseMeta` XDR value rather than a direct byte slice from the compressed source object.

## 3. Hashes and Sizes

| Artifact | Size (bytes) | SHA-256 |
|---|---|---|
| `FC159281--65695102.xdr.zst` (compressed) | `175086` | `41db25fdc6dee9d255d5015db60ba9e7757abb5c090a04f0fe3544fd79b40fe5` |
| `FC159281--65695102.xdr` (decompressed batch) | `1653788` | `2caa44f52f2343d3cc0cdda8d9e6468531abf6709b8f1f1ac036cbc80b75012c` |
| `lcm_65695102.xdr` (extracted `LedgerCloseMeta`) | `1653776` | `0316872e732b0e341f69f92af0c589a1432309b6c4ef19860ee16423f738e9a3` |

Verify all three byte sizes and SHA-256 digests:
```bash
wc -c FC159281--65695102.xdr.zst FC159281--65695102.xdr lcm_65695102.xdr
sha256sum FC159281--65695102.xdr.zst FC159281--65695102.xdr lcm_65695102.xdr
```

## 4. Horizon Cross-Check

Cross-checked against Stellar Horizon (`https://horizon.stellar.org/ledgers/65695102`) during the initial verification session:

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

1. **Generate the Adapter Protocol v1 requests and run `go-ingest-adapter` twice against `lcm_65695102.xdr`:**
   ```bash
   INPUT_SHA256=$(sha256sum lcm_65695102.xdr | awk '{print $1}')

   python3 -c "import json; print(json.dumps({'schema_version':'1','run_id':'run_repro_1','case_id':'pubnet_65695102','operation':'decode_ledger_meta','input_path':'lcm_65695102.xdr','input_format':'LedgerCloseMeta','input_sha256':'$INPUT_SHA256','target':{'adapter_id':'go-ingest','adapter_version':'dev','sdk_version':'v0.9.0'},'limits':{'timeout_ms':10000,'max_input_bytes':10485760,'max_output_bytes':1048576}}))" \
     | go run ./cmd/go-ingest-adapter > run1.json 2>/dev/null

   python3 -c "import json; print(json.dumps({'schema_version':'1','run_id':'run_repro_2','case_id':'pubnet_65695102','operation':'decode_ledger_meta','input_path':'lcm_65695102.xdr','input_format':'LedgerCloseMeta','input_sha256':'$INPUT_SHA256','target':{'adapter_id':'go-ingest','adapter_version':'dev','sdk_version':'v0.9.0'},'limits':{'timeout_ms':10000,'max_input_bytes':10485760,'max_output_bytes':1048576}}))" \
     | go run ./cmd/go-ingest-adapter > run2.json 2>/dev/null
   ```

2. **Canonicalize `semantic_projection` (excluding volatile fields such as `execution.duration_ms`):**
   Each run's `semantic_projection` object is extracted and serialized as compact JSON with sorted keys (`sort_keys=True`, `separators=(",", ":")`) before hashing:
   ```bash
   python3 -c 'import json; r1=json.load(open("run1.json")); r2=json.load(open("run2.json")); open("proj1.json","w").write(json.dumps(r1["semantic_projection"], sort_keys=True, separators=(",", ":"))); open("proj2.json","w").write(json.dumps(r2["semantic_projection"], sort_keys=True, separators=(",", ":")))'
   ```

3. **Hash and compare the two canonical `semantic_projection` outputs:**
   ```bash
   sha256sum proj1.json proj2.json
   cmp -s proj1.json proj2.json && echo "Canonical semantic_projection files are byte-for-byte identical"
   ```

4. **Observed Results:**
   - **Run 1:** Exit code `0`, `execution.status = "completed"`, `decode.status = "success"`
   - **Run 2:** Exit code `0`, `execution.status = "completed"`, `decode.status = "success"`
   - **Canonical `semantic_projection` SHA-256:**
     - `proj1.json`: `ff238af52d50b2a816382ef7f1e09565b9afee306027aa576a5e3092e3582111`
     - `proj2.json`: `ff238af52d50b2a816382ef7f1e09565b9afee306027aa576a5e3092e3582111`
   - **Comparison (`cmp -s`):** Exit code `0` (byte-for-byte identical `semantic_projection` across both runs).
