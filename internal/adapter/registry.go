package adapter

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// AdapterIDGoIngest is the canonical identifier for the built-in Go ingestion adapter.
	AdapterIDGoIngest = "go-ingest"

	// PinnedGoSDKVersion is the pinned official Stellar Go SDK version for this build slice.
	PinnedGoSDKVersion = "v0.9.0"

	// DefaultGoAdapterVersion is the default version string when built without -ldflags.
	DefaultGoAdapterVersion = "dev"
)

// AdapterSpec defines a trusted, allowlisted adapter entry in the registry.
// Executable paths and versions come exclusively from trusted configuration,
// never from a request or corpus manifest.
type AdapterSpec struct {
	ID         string
	Version    string
	SDKVersion string
	BinaryPath string
	Operations []string
}

// SupportsOperation reports whether the adapter supports the given protocol operation.
func (s AdapterSpec) SupportsOperation(op string) bool {
	for _, supported := range s.Operations {
		if supported == op {
			return true
		}
	}
	return false
}

// Registry holds the allowlisted set of approved adapter executables.
// Production registries contain only project-owned, explicitly configured adapters.
type Registry struct {
	adapters map[string]AdapterSpec
}

// ProductionRegistryConfig configures the built-in production adapter registry.
type ProductionRegistryConfig struct {
	// GoIngestBinaryPath is the trusted path to the compiled go-ingest-adapter binary.
	// Must be non-empty and point to an existing regular executable file.
	GoIngestBinaryPath string

	// GoIngestVersion is the expected adapter version (defaults to DefaultGoAdapterVersion if empty).
	GoIngestVersion string

	// GoSDKVersion is the pinned SDK version (defaults to PinnedGoSDKVersion if empty).
	GoSDKVersion string
}

// NewProductionRegistry constructs the production adapter registry containing
// exactly one allowed adapter: go-ingest.
//
// No test adapters or arbitrary user-supplied executables are ever registered here.
func NewProductionRegistry(cfg ProductionRegistryConfig) (*Registry, error) {
	if strings.TrimSpace(cfg.GoIngestBinaryPath) == "" {
		return nil, fmt.Errorf("%w: go-ingest binary path must not be empty", ErrAdapterUnavailable)
	}

	cleanPath := filepath.Clean(cfg.GoIngestBinaryPath)
	if !filepath.IsAbs(cleanPath) {
		absPath, err := filepath.Abs(cleanPath)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to resolve binary path %q: %v", ErrAdapterUnavailable, cfg.GoIngestBinaryPath, err)
		}
		cleanPath = absPath
	}

	adapterVersion := strings.TrimSpace(cfg.GoIngestVersion)
	if adapterVersion == "" {
		adapterVersion = DefaultGoAdapterVersion
	}

	sdkVersion := strings.TrimSpace(cfg.GoSDKVersion)
	if sdkVersion == "" {
		sdkVersion = PinnedGoSDKVersion
	}

	spec := AdapterSpec{
		ID:         AdapterIDGoIngest,
		Version:    adapterVersion,
		SDKVersion: sdkVersion,
		BinaryPath: cleanPath,
		Operations: []string{OperationDecodeLedgerMeta},
	}

	return &Registry{
		adapters: map[string]AdapterSpec{
			AdapterIDGoIngest: spec,
		},
	}, nil
}

// ListIDs returns the sorted list of registered adapter IDs.
func (r *Registry) ListIDs() []string {
	if r == nil {
		return nil
	}
	ids := make([]string, 0, len(r.adapters))
	for id := range r.adapters {
		ids = append(ids, id)
	}
	return ids
}

// Resolve looks up an adapter by target identity, verifies version and SDK
// compatibility, checks operation support, and verifies that the resolved
// binary exists and is an executable regular file.
func (r *Registry) Resolve(target RequestTarget, operation string) (AdapterSpec, error) {
	if r == nil {
		return AdapterSpec{}, fmt.Errorf("%w: registry is nil", ErrAdapterUnavailable)
	}

	spec, ok := r.adapters[target.AdapterID]
	if !ok {
		return AdapterSpec{}, fmt.Errorf("%w: adapter %q is not in the allowlisted registry", ErrAdapterUnavailable, target.AdapterID)
	}

	if target.AdapterVersion != spec.Version {
		return AdapterSpec{}, fmt.Errorf("%w: requested adapter_version %q does not match registered %q",
			ErrAdapterIdentityMismatch, target.AdapterVersion, spec.Version)
	}

	if target.SDKVersion != spec.SDKVersion {
		return AdapterSpec{}, fmt.Errorf("%w: requested sdk_version %q does not match registered %q",
			ErrAdapterIdentityMismatch, target.SDKVersion, spec.SDKVersion)
	}

	if !spec.SupportsOperation(operation) {
		return AdapterSpec{}, fmt.Errorf("%w: adapter %q does not support operation %q",
			ErrInvalidRequest, spec.ID, operation)
	}

	if err := verifyExecutableFile(spec.BinaryPath); err != nil {
		return AdapterSpec{}, err
	}

	return spec, nil
}

// verifyExecutableFile ensures the binary path exists, is a regular file (after
// resolving symlinks), and has executable permissions on POSIX platforms.
func verifyExecutableFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: adapter binary %q does not exist", ErrAdapterUnavailable, path)
		}
		return fmt.Errorf("%w: failed to stat adapter binary %q: %v", ErrAdapterUnavailable, path, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: adapter binary %q is not a regular file (mode %s)", ErrAdapterUnavailable, path, info.Mode().Type())
	}

	if runtime.GOOS != "windows" {
		if info.Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("%w: adapter binary %q is not executable (mode %s)", ErrAdapterUnavailable, path, info.Mode().Perm())
		}
	}

	return nil
}
