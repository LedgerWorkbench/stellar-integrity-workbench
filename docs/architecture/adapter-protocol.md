# Adapter Process Protocol (Version 1)

**Status:** Implemented in Phase 6, Unit 4
**Protocol Version:** `"1"`
**Schema Standard:** JSON Schema Draft 2020-12

## 1. Overview

The Stellar Integrity Workbench worker communicates with approved adapter processes over a strict, versioned JSON protocol using `stdin` and `stdout`. Each process invocation handles **exactly one** request and produces **exactly one** JSON result.

- **Request:** Written as a single JSON document to the adapter's `stdin`.
- **Result:** Written as a single JSON document to the adapter's `stdout`.
- **Diagnostics:** Written exclusively to `stderr` (structured logs or diagnostic messages). `stdout` must contain no banners, progress messages, or log output.

## 2. Schema Definitions

The canonical schemas are embedded in `schemas/schemas.go`:

- Request schema: [`schemas/adapter-request.schema.json`](../../schemas/adapter-request.schema.json)
- Result schema: [`schemas/adapter-result.schema.json`](../../schemas/adapter-result.schema.json)

Both schemas are compiled and validated against JSON Schema Draft 2020-12 using `github.com/santhosh-tekuri/jsonschema/v6`.

### 2.1 Request Structure

```json
{
  "schema_version": "1",
  "run_id": "run_example",
  "case_id": "case_example",
  "operation": "decode_ledger_meta",
  "input_path": "/worker-created/run-directory/input.xdr",
  "input_format": "LedgerCloseMeta",
  "input_sha256": "<64-char-lowercase-hex-sha256>",
  "target": {
    "adapter_id": "go-ingest",
    "adapter_version": "dev",
    "sdk_version": "v0.9.0"
  },
  "limits": {
    "timeout_ms": 30000,
    "max_input_bytes": 67108864,
    "max_output_bytes": 10485760
  }
}
```

#### Request Validation Rules

- `schema_version`: Must be `"1"`. Unsupported versions are rejected before processing.
- `run_id`, `case_id`: Non-empty strings (1–256 chars).
- `operation`: Currently `"decode_ledger_meta"` only.
- `input_path`: Non-empty path to the worker-staged, read-only input file inside the per-run temporary directory. Never accepted from untrusted manifests or network requests.
- `input_format`: Currently `"LedgerCloseMeta"` only.
- `input_sha256`: Exactly 64 lowercase hexadecimal characters (`^[0-9a-f]{64}$`).
- `target`: Non-empty `adapter_id`, `adapter_version`, and `sdk_version` (max 128 chars each).
- `limits`: Positive integers within bounds (`timeout_ms`: 1–3,600,000; `max_input_bytes` and `max_output_bytes`: 1–1,073,741,824).
- Unknown fields (`additionalProperties: false`) and trailing JSON documents are strictly rejected.
- Requests cannot specify an executable path, shell command, arbitrary arguments, environment variables, or working directory.

### 2.2 Result Structure

```json
{
  "schema_version": "1",
  "run_id": "run_example",
  "case_id": "case_example",
  "adapter_id": "go-ingest",
  "adapter_version": "dev",
  "sdk_version": "v0.9.0",
  "execution": {
    "status": "completed",
    "exit_code": 0,
    "duration_ms": 15
  },
  "input": {
    "sha256": "<64-char-lowercase-hex-sha256>"
  },
  "decode": {
    "status": "not_attempted",
    "detected_type": null
  },
  "semantic_projection": null,
  "diagnostics": [],
  "capabilities_used": []
}
```

#### Status Enums

- `execution.status`: `"completed"`, `"failed"`, `"cancelled"`
- `decode.status`: `"not_attempted"`, `"success"`, `"invalid_input"`, `"unsupported_type"`, `"unsupported_protocol"`

This separation ensures a process-level exit (`execution`) is never conflated with Stellar XDR decoding (`decode`). Unavailable fields (such as `detected_type` or `semantic_projection` when decoding has not succeeded) use `null` rather than fabricated defaults.

#### Response Cross-Validation

After execution, the worker's `ProcessRunner` verifies:
1. Output did not exceed `max_output_bytes` (checked before timeout/exit errors).
2. `stdout` decodes as exactly one JSON object with no unknown fields or trailing data.
3. Raw JSON validates against `adapter-result.schema.json`.
4. `schema_version` is `"1"`.
5. `run_id` and `case_id` match the request.
6. `adapter_id`, `adapter_version`, and `sdk_version` match the requested target and registry entry.
7. `input.sha256` matches the verified input hash.
8. `execution.exit_code` and `execution.status` are consistent with the actual OS process exit code.

## 3. Reconciliation with Technical Architecture

In `docs/architecture/technical-architecture.md` (Section 6.3), the high-level `AdapterRunner` contract sketched nested `input` (`format`, `encoding`, `sha256`, `object_ref`), `limits.memory_mb`, and `semantic_output_ref` / `raw_output_ref`.

In Phase 6, Unit 4, the internal worker-to-adapter process protocol uses worker-staged `input_path`, `input_format`, and `input_sha256` at the top level of the request, and inline `semantic_projection` (or `null`) in the result. This is an internal process boundary between `workbench-worker` and allowlisted local adapter binaries, not a public API for external clients.

## 4. Boundary Between Unit 4 and Unit 5

Unit 4 establishes the protocol contract, schema validation, registry, and bounded process runner. **Unit 4 does not implement Stellar XDR decoding or semantic projection.**

Until Unit 5 is implemented, `cmd/go-ingest-adapter` validates incoming protocol v1 requests and returns an explicit non-success response with `execution.status: "failed"`, `execution.exit_code: 3`, `decode.status: "not_attempted"`, `semantic_projection: null`, and a structured diagnostic (`code: "decoder_not_implemented"`).
