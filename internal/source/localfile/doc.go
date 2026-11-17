// Package localfile provides secure, root-confined local filesystem storage access,
// bounded input reading, and cryptographic integrity verification for corpus input objects.
//
// # Trusted Storage Root Configuration
//
// The corpus object storage root is supplied exclusively through trusted application
// configuration (e.g. CLI flags or server environment configuration), never from an
// untrusted corpus manifest or API request. Manifests specify relative object paths
// (keys) within the trusted root directory.
//
// # Relative Object Path Handling and Confinement
//
// Before any filesystem operation is performed, the object path is validated using
// corpus.ValidateObjectPath. All absolute paths, Windows drive identifiers, directory
// traversals (".."), current directory segments ("."), empty segments, backslashes,
// and control characters are rejected.
//
// Filesystem operations are confined to the configured storage root using Go 1.25's
// os.OpenRoot API. The os.Root handle ensures that relative file access cannot escape
// above the designated root directory via parent traversals ("..") or external symlinks.
//
// Confinement Boundaries and Limitations:
//   - os.Root is not a complete operating system sandbox or container isolation boundary.
//   - It does not prohibit crossing filesystem mount boundaries or traversing into subtrees
//     mounted inside the root (e.g. bind mounts, loop mounts, or virtual filesystems such as
//     /proc or /sys if mounted within the root path).
//   - It does not prohibit opening special files (such as character devices, block devices,
//     or FIFOs) present within the directory tree; therefore, post-open regular-file
//     verification (info.Mode().IsRegular()) remains strictly mandatory.
//   - The storage root directory itself must be an administrator-controlled, trusted
//     filesystem location containing only authentic corpus data.
//
// # Symlink Policy
//
// Symbolic links within the storage root are handled deterministically:
//   - Symlink Escapes: Any symbolic link that references a location outside the configured
//     storage root (via parent directory traversal or absolute target) is strictly rejected
//     by os.Root confinement.
//   - Internal Symlinks: Symbolic links that remain strictly within the storage root and
//     resolve to regular files are permitted, provided the resolved target satisfies all
//     regularity, size, and integrity requirements.
//   - Non-Regular Targets: Any symlink resolving to a directory, FIFO, device, socket, or
//     non-existent file is rejected.
//
// # Accepted File Types and Special File Handling
//
// Only regular files are accepted for replay processing. Directories, FIFOs (named pipes),
// Unix domain sockets, and character/block device nodes are strictly rejected.
//
// To prevent denial-of-service hangs when encountering a FIFO or named pipe, files are
// opened using non-blocking I/O (O_NONBLOCK on POSIX platforms). This guarantees that opening
// a FIFO without an active writer returns immediately rather than blocking worker goroutines
// indefinitely. Once opened, the file mode is checked via Stat; if the file is not regular,
// it is immediately closed and rejected with ErrNotRegularFile.
//
// # Bounded Input Size Enforcement
//
// Unbounded reads of untrusted inputs are strictly forbidden. Input sizes are enforced in
// two distinct stages:
//
//  1. Pre-Read Size Check: The reported size from file.Stat() is checked before reading.
//     If the file size exceeds the configured maximum, the file is rejected immediately
//     without allocating memory for its contents.
//  2. Bounded Streaming Read: File contents are read using an io.LimitReader bounded to
//     maxBytes + 1. If reading encounters more than maxBytes, the file grew after its
//     initial stat, and it is rejected immediately.
//
// The default maximum file size is 64 MiB (DefaultMaxBytes = 67,108,864 bytes).
// The 64 MiB default is an initial operational safety limit selected to bound
// individual input loading and memory consumption. It is not a protocol-derived
// maximum, and it is not based on measured maximum sizes across historical
// Stellar ledger-close metadata. Larger inputs require an explicit, reviewed
// configuration change and remain subject to aggregate worker memory and
// concurrency controls.
//
// Invalid size configurations (zero, negative limits, or values exceeding platform-safe
// integer bounds MaxAllowedBytes) are rejected during Store initialization with ErrInvalidConfig.
// Limits cannot be silently disabled.
//
// # Integrity Verification
//
// Verification is performed over the exact, unparsed, unnormalized original bytes:
//   - Byte Length: The total bytes read must match the manifest's byte_length.
//   - SHA-256 Digest: The SHA-256 digest computed during streaming must match the
//     manifest's sha256 (64 lowercase hex characters).
//
// Expected manifest values are treated as claims to be verified, never as proof of authenticity.
// Verification must succeed in full before the payload can be passed to downstream consumers.
//
// # Immutable Input Preservation
//
// The original file on disk is never modified. The verified in-memory payload is encapsulated
// in a VerifiedInput struct. The private backing byte buffer is not exposed as a mutable slice.
// Callers obtain data via NewReader() (returning an independent io.Reader) or BytesCopy()
// (returning a defensive copy).
//
// # Scope Limitation
//
// This package is solely responsible for storage confinement, file loading, and integrity
// verification. It does not perform Stellar XDR decoding, adapter invocation, or ledger
// assertion evaluation. Those responsibilities belong to subsequent pipeline stages.
package localfile
