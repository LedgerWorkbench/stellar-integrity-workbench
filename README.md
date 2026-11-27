# Stellar Integrity Workbench

Stellar Integrity Workbench is a local-first conformance and regression replay tool for already-produced Stellar ledger-close metadata (`LedgerCloseMeta` XDR). It verifies immutable local inputs, runs an allowlisted ingestion adapter in a bounded subprocess, evaluates typed assertions, writes and independently verifies an immutable evidence bundle, and renders schema-validated JSON and Markdown reports.

> **Ingestion Replay Boundary:** This workbench replays already-produced Stellar ledger metadata against pinned SDK decoders (`github.com/stellar/go-stellar-sdk v0.9.0`). It is not SCP consensus replay, a replacement for Stellar Core, a general-purpose indexer, or a Soroban smart contract application.

## Status

- **Phase 6 Core Engine:** Units 1–9 implemented; Unit 10 (CI/documentation completion) pending.
- **Go Toolchain:** Go 1.25+
- **Pinned Stellar Go SDK:** `github.com/stellar/go-stellar-sdk v0.9.0`

## Building

Build both binaries into the same directory (the worker resolves `go-ingest-adapter` from its own executable directory):

```bash
mkdir -p bin artifacts
go build -o ./bin/go-ingest-adapter ./cmd/go-ingest-adapter
go build -o ./bin/workbench-worker ./cmd/workbench-worker
```

Verify the build:

```bash
./bin/workbench-worker help
./bin/workbench-worker version
```

## Preparing Local Corpus Input and Manifest

1. Place the immutable `LedgerCloseMeta` XDR binary at a safe corpus-relative path beneath your trusted corpus root (for example, `./corpus/objects/sha256/<digest>`).
2. Create a corpus manifest JSON conforming to `schemas/corpus-manifest.schema.json` (`1.0.0`) whose `input.object_path`, `input.sha256`, and `input.byte_length` match the local file. (See `docs/validation/ledger-sample-65695102.md` for reproducible extraction of an authentic Pubnet sample using `tools/extract-ledger-meta`.)

## Running a Replay Job

```bash
./bin/workbench-worker run \
  --manifest ./corpus/manifests/case.json \
  --input-root ./corpus \
  --input objects/sha256/<digest> \
  --output-dir ./artifacts
```

### Flags

| Flag | Description |
|---|---|
| `--manifest` | Path to the local corpus manifest JSON file (max 1 MiB, required) |
| `--input-root` | Trusted local corpus-object root directory (required) |
| `--input` | Canonical corpus-relative object path matching `input.object_path` (required) |
| `--output-dir` | Existing output directory for evidence bundles and reports (required) |
| `--json` | Emit a single JSON report (or sanitized JSON error envelope) to stdout |
| `--timeout-ms` | Adapter execution timeout in milliseconds (default: `30000`) |
| `--max-input-bytes` | Maximum allowed input bytes (default: `67108864` / 64 MiB) |
| `--max-output-bytes` | Maximum allowed adapter stdout bytes (default: `10485760` / 10 MiB) |

Diagnostics and structured logs are written to `stderr`. In `--json` mode, `stdout` receives exactly one JSON document.

## Output Layout

For each run, `workbench-worker` generates a unique cryptographically random `<run-id>` and writes:

```text
<output-dir>/
├── <run-id>/
│   ├── manifest.json
│   └── artifacts/
│       ├── adapter-stdout.json
│       ├── semantic-projection.json
│       └── assertions.json
├── <run-id>.json
└── <run-id>.md
```

Existing run bundles and report files are never overwritten.

## Exit Codes

| Exit Code | Meaning |
|---:|---|
| `0` | Completed run with `pass` |
| `1` | Invalid command, missing required flags, or usage error |
| `2` | Invalid manifest, missing/unsafe input, input too large, byte-length mismatch, or SHA-256 mismatch |
| `3` | Adapter unavailable, timeout, process failure, unsupported format/operation, or invalid adapter response |
| `4` | Completed run with `regression` |
| `5` | Internal failure, evidence write/verification failure, or report-generation failure |
| `6` | Completed run with `inconclusive` outcome |
| `7` | Cancelled run |
| `8` | Completed run with `not_applicable` outcome |

## Running Tests

```bash
go test ./... -count=1
go test -race ./...
go vet ./...
go mod verify
```

## Known Limitations

- Local-only execution; remote manifest/input fetching, object storage, and digital signatures are not implemented.
- Only the built-in `go-ingest` adapter (`v0.9.0`) is registered.
- The authentic Pubnet `65695102` sample binary is intentionally not committed to the repository; local end-to-end acceptance was verified against the locally extracted `lcm_65695102.xdr` sample in the developer environment.
