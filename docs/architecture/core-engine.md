# Core Engine Architecture (Phase 6)

**Status:** Units 1–4 implemented; Units 5–9 pending

## 1. Implemented Components

### Unit 1: Corpus Manifest & Domain Model
- `schemas/corpus-manifest.schema.json`, `internal/corpus/`, `internal/domain/`
- Strict Draft 2020-12 manifest validation, evidence classification, path safety (`ValidateObjectPath`), and run/assertion domain types.

### Unit 2: Local File Source & Integrity Verification
- `internal/source/localfile/`
- Root-confined corpus access via Go 1.25 `os.Root`, non-blocking regular-file verification, bounded streaming read (`DefaultMaxBytes = 64 MiB`), SHA-256 verification, and immutable `VerifiedInput`.

### Unit 3: Command Skeletons
- `cmd/workbench-worker/main.go`, `cmd/go-ingest-adapter/main.go`

### Unit 4: Adapter Protocol & Allowlisted Process Runner
- `schemas/adapter-request.schema.json`, `schemas/adapter-result.schema.json`, `schemas/schemas.go`
- `internal/adapter/`

## 2. Production Adapter Registry

`adapter.NewProductionRegistry` maintains an allowlist of project-owned adapter binaries configured via trusted application configuration (`ProductionRegistryConfig`).

- In Unit 4, the production registry contains **only** `go-ingest`.
- Adapter IDs, versions, and pinned SDK versions (`v0.9.0` for `go-ingest`) are verified against the registry before process launch.
- `verifyExecutableFile` ensures the resolved binary exists, is a regular file, and has executable permissions on POSIX platforms.
- Request payloads and corpus manifests can never supply executable paths, shell commands, or CLI arguments.
- Test-only adapter helpers exist strictly in `*_test.go` files and are never registered in production registries.

## 3. Process Runner Lifecycle & Security Controls

`adapter.ProcessRunner.Run` accepts trusted `RunParams` and an already-verified `*localfile.VerifiedInput`:

1. **Pre-launch checks:** Validates limits and rejects inputs exceeding `max_input_bytes` (`ErrInputTooLarge`) before creating temp directories or launching processes.
2. **Registry resolution:** Resolves the allowlisted executable and verifies version/operation compatibility.
3. **Per-run temporary workspace:**
   - Creates a unique temporary directory (`siw-run-*`) with `0700` permissions.
   - Stages the verified bytes into `input.xdr` with `0400` (read-only) permissions without modifying the original corpus source.
   - Sets `cmd.Dir` to the per-run temporary directory.
   - Removes the temporary directory via `defer` on completion, timeout, cancellation, or error.
4. **Minimal environment & direct execution:**
   - Invokes `exec.CommandContext(runCtx, spec.BinaryPath)` directly with no shell (`sh -c` / `bash -c` is never used) and no user-supplied arguments.
   - Uses a stripped environment (`LC_ALL=C`, `LANG=C`, plus `SYSTEMROOT` on Windows only) so cloud credentials, GitHub tokens, and host secrets are not inherited.
5. **Bounded output capture:**
   - `stdout` is captured through `boundedStdoutWriter` limited to `limits.max_output_bytes`. Excess writes are not buffered; exceeding the limit cancels the process context and returns `ErrAdapterOutputTooLarge` (never misclassified as a timeout).
   - `stderr` is captured separately through `boundedStderrWriter` limited to `MaxStderrBytes` (default 1 MiB). Excess stderr is drained and counted (`StderrTruncated`, `StderrBytesSeen`) without unbounded memory growth and never mixed into `stdout`.
6. **Timeout & cancellation:**
   - Enforces `limits.timeout_ms` via `context.WithTimeout` bound to the caller's context.
   - Distinguishes parent cancellation (`ErrAdapterCancelled`) from execution timeout (`ErrAdapterTimeout`).
7. **Strict response validation:**
   - Decodes `stdout` with `DisallowUnknownFields()` and trailing-data rejection, validates against `adapter-result.schema.json`, checks identity/hash consistency, and verifies `execution.exit_code` and `execution.status` against the actual process exit code.

## 4. Known OS Isolation Limitations

`ProcessRunner` is a bounded process-isolation layer, not a full OS sandbox or container runtime:
- **Memory limits:** Portable OS-level memory cgroups or `RLIMIT_AS` enforcement are not applied by `ProcessRunner`; deployments must enforce container memory limits externally.
- **Network isolation:** `ProcessRunner` does not create Linux network namespaces; production deployments must restrict outbound network access at the container/host firewall level.
- **Process-tree termination:** `exec.CommandContext` terminates the direct child process on timeout or cancellation. Grandchildren spawned by an adapter are not guaranteed to be reaped without OS-specific process-group or cgroup supervision.

## 5. Remaining Work (Units 5–9)

- **Unit 5:** Implement Stellar XDR `LedgerCloseMeta` decoding and canonical semantic projection in `go-ingest-adapter` using `github.com/stellar/go-stellar-sdk v0.9.0`.
- **Units 6–9:** Assertion evaluation engine, evidence bundle generation, and full `workbench-worker` run command wiring.
