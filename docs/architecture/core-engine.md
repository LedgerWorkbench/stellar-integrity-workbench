# Core Engine Architecture (Phase 6)

**Status:** Units 1–5 implemented; Units 6–10 pending

## 1. Implemented Components

### Unit 1: Command Skeletons
- `cmd/workbench-worker/main.go`, `cmd/go-ingest-adapter/main.go`

### Unit 2: Corpus Manifest & Domain Model
- `schemas/corpus-manifest.schema.json`, `internal/corpus/`, `internal/domain/`
- Strict Draft 2020-12 manifest validation, evidence classification, path safety (`ValidateObjectPath`), and run/assertion domain types.

### Unit 3: Local File Source & Integrity Verification
- `internal/source/localfile/`
- Root-confined corpus access via Go 1.25 `os.Root`, non-blocking regular-file verification, bounded streaming read (`DefaultMaxBytes = 64 MiB`), SHA-256 verification, and immutable `VerifiedInput`.

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

## 5. Unit 5: Go Ingestion Adapter (`internal/adapter/goingest`, `cmd/go-ingest-adapter`)

- **Pinned SDK:** `github.com/stellar/go-stellar-sdk v0.9.0` (resolves to tag `v0.9.0`, commit `5410d3410e212331399d1518cc13150279969dac`).
- **Input integrity:** `goingest.LoadAndVerifyInput` rejects missing files, symlinks, directories, and non-regular files via `os.Lstat` and `f.Stat`, enforces `limits.max_input_bytes`, reads exact raw bytes without normalization, computes SHA-256, and rejects any mismatch against `input_sha256` before invoking XDR decoding.
- **XDR decoder API:** Pre-checks the outer union discriminant using `xdr.LedgerCloseMetaView(data).V()` (official SDK zero-copy view API) before calling `xdr.SafeUnmarshal(data, &lcm)` into `xdr.LedgerCloseMeta`. Truncated discriminants (< 4 bytes) return `ErrXDRDecodeFailed`; outer discriminants outside `{0, 1, 2}` return `ErrUnsupportedXDRVariant` (`decode.status = "unsupported_type"`); `SafeUnmarshal` rejects truncated payloads, malformed XDR, and trailing unconsumed bytes with `ErrXDRDecodeFailed` (`decode.status = "invalid_input"`).
- **Supported outer `LedgerCloseMeta` variants:** `V0` (`LedgerCloseMetaV0`), `V1` (`LedgerCloseMetaV1`), and `V2` (`LedgerCloseMetaV2`), validated structurally (including `GeneralizedTransactionSet.V == 1` and `TransactionPhase.V` `0`/`1`, plus `TxSet`/`TxProcessing` count alignment) before calling SDK accessors. Unknown or unsupported variants return structured `ErrUnsupportedXDRVariant` / `ErrXDRDecodeFailed` errors without panicking.
- **Inner variant classification limitation:** After `SafeUnmarshal` succeeds for a supported outer variant (`0`, `1`, or `2`), `validateLedgerCloseMetaStructure` checks inner union variants (`GeneralizedTransactionSet.V`, `TransactionPhase.V`). However, the pinned SDK `v0.9.0` cannot distinguish a malformed inner structure from a future inner variant at the XDR level — `SafeUnmarshal` either succeeds or fails as a whole. Where the SDK cannot make this distinction, the adapter conservatively treats decoded inner variants outside the validated set as `ErrUnsupportedXDRVariant`.
- **SDK accessors & ingest helpers used:**
  - `xdr.LedgerCloseMeta.LedgerSequence()`
  - `xdr.LedgerCloseMeta.ProtocolVersion()`
  - `xdr.LedgerCloseMeta.LedgerCloseTime()`
  - `xdr.LedgerCloseMeta.LedgerHash().HexString()`
  - `xdr.LedgerCloseMeta.PreviousLedgerHash().HexString()`
  - `xdr.LedgerCloseMeta.CountTransactions()`
  - `xdr.LedgerCloseMeta.TransactionHash(i).HexString()`
  - `ingest/ledger.TransactionCounts(lcm)` (`github.com/stellar/go-stellar-sdk/ingest/ledger`)
- **Semantic projection (`goingest.SemanticProjection`, `projection_version: "1"`):**
  - `xdr_type`: `"LedgerCloseMeta"`, `outer_variant`: `0 | 1 | 2`.
  - `ledger`: `sequence`, `protocol_version`, `close_time_unix`, `ledger_hash`, `previous_ledger_hash`.
  - `transactions`: `total`, `successful`, `failed`, `hashes` (in processing order, bounded to `MaxProjectedTxHashes = 256`), and `hashes_truncated` (`true` when `total > 256`).
- **Network passphrase limitation:** `ingest.NewLedgerTransactionReaderFromLedgerCloseMeta` requires a `networkPassphrase` argument, which is not part of Adapter Protocol v1. The Unit 5 adapter does not hardcode or infer Public/Testnet passphrases and instead uses `xdr.LedgerCloseMeta` accessors and `ingest/ledger.TransactionCounts`. Full per-transaction envelope/operation extraction via `LedgerTransactionReader` is deferred until an explicit protocol extension supplies network identity or passphrase.

## 6. Remaining Work (Units 6–10)

- **Units 6–10:** Assertion evaluation engine, evidence bundle generation, and full `workbench-worker` run command wiring.
