# Evidence Bundle Architecture (Unit 7)

Package: `internal/evidence`
Schema: `schemas/evidence-bundle.schema.json` (JSON Schema Draft 2020-12, manifest `schema_version: "1.0.0"`)

## 1. Evidence Manifest Version and Fields

The manifest (`manifest.json`) uses `schema_version: "1.0.0"` (`^1\.[0-9]+\.[0-9]+$`) with `additionalProperties: false` and strict single-document JSON decoding (`DisallowUnknownFields()`, `UseNumber()`, trailing-data rejection).

| Section | Fields | Source |
|---|---|---|
| Identity | `schema_version`, `run_id`, `case_id`, `classification` | `evidence.SchemaVersion`, `domain.Run`, `domain.Case` |
| Provenance | `kind`, `url`, `repository`, `issue_number`, `pull_request_number`, `retrieved_at` (UTC RFC 3339), `notes` | `domain.Case.Source` |
| Input Integrity | `sha256`, `byte_length`, `format`, `encoding`, `object_path`, `detected_type`, `network_name`, `ledger_sequence`, `protocol_version` | `localfile.VerifiedInput` cross-checked against `domain.Case.Input`, `domain.Case.Network`, and `adapter.Result.Decode.DetectedType` |
| Worker Metadata | `version`, `commit_sha` (`null` when unavailable) | Trusted `evidence.WorkerMetadata` |
| Adapter Metadata | `adapter_id`, `adapter_version`, `sdk_version` | `adapter.RequestTarget` |
| Run Metadata | `started_at`, `completed_at` (UTC RFC 3339), `state`, `adapter_execution_status`, `adapter_exit_code`, `adapter_duration_ms` (adapter-reported), `host_exit_code` (host-observed), `host_duration_ms` (host-measured), `outcome` (`null` for failed/cancelled runs) | `domain.Run`, `adapter.Result.Execution`, `adapter.RunOutput` |
| Assertions | `assertion_id`, `kind`, `expected`, `observed`, `status`, `explanation`, `evidence_refs` | Unit 6 `[]domain.AssertionResult` |
| Diagnostics | `adapter_diagnostics`, `limitations`, `adapter_stderr_bytes_seen`, `adapter_stderr_truncated` | `adapter.Result.Diagnostics`, `domain.Case.Limitations`, `adapter.RunOutput` |
| Artifacts | `path`, `role`, `media_type`, `byte_length`, `sha256` | Computed from exact stored bytes |

## 2. Bundle Directory Layout

```text
<evidence-root>/
└── <run-id>/
    ├── manifest.json
    └── artifacts/
        ├── adapter-stdout.json
        ├── semantic-projection.json
        └── assertions.json
```

- `artifacts/adapter-stdout.json` (`role: adapter_stdout`): exact validated JSON bytes captured from adapter stdout via `adapter.RunOutput.StdoutCopy()`, never reconstructed via `json.Marshal`.
- `artifacts/semantic-projection.json` (`role: semantic_projection`): exact JSON object bytes from `adapter.Result.SemanticProjection` when non-null.
- `artifacts/assertions.json` (`role: assertions`): ordered JSON array of Unit 6 `domain.AssertionResult` values when present.

## 3. Immutable Write Behaviour

`evidence.WriteBundle(evidenceRoot, input)`:
1. Validates all cross-entity invariants:
   - Terminal run state, outcome rules, `VerifiedInput` vs `Case.Input`, `AdapterOutput` identity/exit-code/hash vs `Run`/`VerifiedInput`, UTC RFC 3339 timestamp ordering.
   - For `completed` runs: requires non-empty `Case.Expectations`, 1-to-1 ordered `Assertions` matching expectation IDs, kinds, and exact expected values, `domain.DeriveOutcome(Assertions) == *Run.Outcome`, non-nil `AdapterOutput`/`Result` with `Execution.Status == "completed"`, `ActualExitCode == 0`, and non-empty `StdoutCopy()` (`adapter_stdout` and `assertions` artifacts are mandatory; `semantic_projection` is stored when non-null).
   - For `failed` or `cancelled` runs: requires `Run.Outcome == nil` and permits pre-adapter failures without `AdapterOutput`.
2. Confines filesystem operations to `evidenceRoot` via Go 1.25 `os.Root`.
3. Refuses to overwrite an existing `<run-id>` directory (`ErrBundleExists`).
4. Writes each artifact exclusively (`O_WRONLY|O_CREATE|O_EXCL`, `0400` permissions), computing SHA-256 and exact byte length from the written bytes.
5. Writes `manifest.json` exclusively (`0400`) only after all artifacts succeed.
6. Runs `VerifyBundle` on the finished bundle; cleans up only the partial `<run-id>` directory via `root.RemoveAll(runID)` if writing or verification fails.

## 4. Verification Procedure

`evidence.VerifyBundle(evidenceRoot, runID)` is strictly read-only:
1. Confines all filesystem access and directory enumeration via `os.Root` and `runRoot.FS()`, rejecting symlinks (`Lstat` + `os.SameFile` post-open check) and non-regular files.
2. Enforces `MaxManifestBytes` (1 MiB) before decoding `manifest.json`, enables JSON Schema format assertions (`AssertFormat()`), enforces canonical UTC `'Z'` RFC 3339 timestamps (`provenance.retrieved_at`, `run.started_at`, `run.completed_at`), and verifies `manifest.run_id == runID`.
3. Validates assertion ID uniqueness, outcome consistency (`domain.DeriveOutcome`), artifact path and role uniqueness (requiring `adapter_stdout` and `assertions` for `completed` runs), and cross-checks `artifacts/assertions.json` against `manifest.assertions`.
4. Recomputes exact byte length and SHA-256 for every listed artifact, reporting exact mismatches (`ErrArtifactMissing`, `ErrByteLengthMismatch`, `ErrHashMismatch`).
5. Scans the bundle directory and `artifacts/` via `runRoot.FS()` to reject any unlisted extra files or subdirectories (`ErrExtraArtifact`).

## 5. Original Input Reference & Timestamp Separation

- Original binary inputs remain in the immutable corpus store and are referenced via `input.object_path`, `input.sha256`, and `input.byte_length` (never absolute machine paths).
- Run timestamps (`started_at`, `completed_at`) and adapter duration live in `manifest.json` (`run` section) and `adapter-stdout.json`; `semantic-projection.json` and `assertions.json` contain no run timestamps, so identical semantic inputs yield identical artifact hashes across runs.

## 6. Trust Boundaries & Deferred Work

- **What verification proves:** Internal consistency, schema compliance, absence of tampering, missing files, or unlisted files in the stored local bundle, and exact SHA-256/byte-length agreement with `manifest.json`.
- **What verification does not prove:** Cryptographic authenticity against an external signer or remote transparency log.
- **Deferred work:** Remote object storage, digital signatures, and full `workbench-worker` orchestration (Unit 9).
