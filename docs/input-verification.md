# Stellar Integrity Workbench — Immutable Local Input & Integrity Verification Policy

**Version:** 1.0.0
**Status:** Authoritative Specification
**Applies to:** Core Engine, Worker Ingestion Pipeline, Local Storage Adapter

---

## 1. Overview & Architectural Boundaries

Stellar Integrity Workbench executes deterministic replay analysis over already-produced Stellar ledger data. The integrity of findings depends on untampered, immutable raw inputs.

This specification defines the contract for local-file input loading and cryptographic integrity verification implemented in `internal/source/localfile`.

> [!IMPORTANT]
> **Scope Boundary:** This unit enforces secure filesystem confinement, bounded input reading, and exact SHA-256 / byte-length integrity verification. It deliberately does **not** decode Stellar XDR, execute SDK/indexer adapters, or evaluate ledger assertions. Those operations occur in downstream pipeline units. No real or production Stellar ledger data is claimed to have passed this unit; all test fixtures are strictly synthetic.

---

## 2. Trusted Storage Root Configuration

1. **Trusted Origin:** The storage root directory (`RootDir`) is established solely through trusted application configuration (such as worker CLI flags or secure server environment configuration).
2. **Untrusted Input Restrictions:** Manifests and API requests are **strictly forbidden** from selecting or overriding the storage root. Manifests provide only a relative object path (`object_path`) within the configured root.
3. **Validation on Initialization:** The store verifies that `RootDir` exists and is a directory. Empty, whitespace, or non-existent paths are rejected immediately with `ErrInvalidConfig`.

---

## 3. Relative Object Path Handling & Filesystem Confinement

### 3.1 Object Path Validation
Before any filesystem operation occurs, `input.object_path` is validated via `corpus.ValidateObjectPath`. The following are strictly rejected:
- Absolute filesystem paths (`/etc/passwd`, `/...`)
- Windows drive identifiers (`C:file.xdr`) or backslashes (`\`)
- Parent directory traversal segments (`..`)
- Current directory segments (`.`)
- Empty segments (`//`) or trailing slashes (`dir/`)
- Control characters (such as null bytes `\x00` or newlines `\n`)

### 3.2 Confinement via `os.OpenRoot` (Go 1.25)
File access is confined to the storage root using Go 1.25's `os.OpenRoot` API. All path resolutions are scoped beneath the open `os.Root` directory descriptor.

### 3.3 Symbolic Link Policy
- **Symlink Escapes:** Any symbolic link attempting to reference a target outside the storage root (via parent traversals or absolute targets) is rejected by root confinement.
- **Internal Symlinks:** Symbolic links whose targets remain strictly within the storage root and resolve to regular files are followed, provided the target satisfies all size and integrity checks.
- **Non-Regular Targets:** Symlinks resolving to directories, FIFOs, device nodes, or sockets are rejected.

---

## 4. Accepted File Types & Safe Opening Strategy

1. **Regular Files Only:** Only regular files (`os.ModeType == 0`) are accepted. Directories, FIFOs (named pipes), Unix domain sockets, and character/block devices are rejected with `ErrNotRegularFile`.
2. **Non-Blocking FIFO Opening:** To prevent worker goroutines from hanging indefinitely when encountering a FIFO without an active writer, files are opened using non-blocking I/O (`O_NONBLOCK` on POSIX platforms). Once opened, `file.Stat()` verifies `info.Mode().IsRegular()`. If not regular, the descriptor is immediately closed and rejected, ensuring zero worker deadlocks.

---

## 5. Bounded Input Size Enforcement

Unbounded input reads present a denial-of-service vector and are prohibited. File sizes are enforced through two complementary stages:

1. **Pre-Read Size Check:** `file.Stat()` inspects the reported file size prior to reading. If `info.Size()` exceeds the configured `MaxBytes`, the file is rejected immediately without allocating heap memory.
2. **Bounded Streaming Read:** File contents are read using `io.LimitReader` bounded to `MaxBytes + 1`. If more than `MaxBytes` are read, the file grew after its initial stat, and it is rejected immediately with `ErrFileTooLarge`.
3. **Configurable Default Maximum (64 MiB):**
   - The default limit is `64 MiB` (`DefaultMaxBytes = 67,108,864` bytes).
   - **Rationale:** In Stellar Mainnet (Protocol 20-22), `LedgerCloseMeta` envelope sizes are bounded by protocol ledger operation limits. Real ledger closes measure under several megabytes even during high transaction volumes. A 64 MiB limit provides ample headroom for historical envelopes while bounding memory consumption.
   - **Validation:** Zero and negative limit values are strictly rejected on Store initialization with `ErrInvalidConfig`. The limit cannot be silently disabled.

---

## 6. Cryptographic Integrity Verification

Integrity verification is calculated over the exact, unparsed, unnormalized original bytes:
1. **Byte Length:** The total bytes read must match `manifest.byte_length` exactly.
2. **SHA-256 Digest:** The SHA-256 digest computed during the streaming read must match `manifest.sha256` (64 lowercase hexadecimal characters).

Manifest values are treated as claims to be verified, never as trusted proof. If either the length or the hash does not match, the input is rejected with `ErrByteLengthMismatch` or `ErrSHA256Mismatch`.

Verification must succeed completely before an input can be passed to downstream consumers.

---

## 7. Immutable Input Preservation

1. **On-Disk Immutability:** The source file on disk is opened read-only and is never modified.
2. **In-Memory Encapsulation:** Verified data is encapsulated in a `VerifiedInput` structure. Its private backing byte buffer is unexported. Downstream callers receive independent readers via `NewReader() io.Reader` or defensive copies via `BytesCopy() []byte`. External mutations of returned slices cannot alter the verified buffer.
