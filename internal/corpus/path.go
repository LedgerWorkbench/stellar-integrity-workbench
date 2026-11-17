package corpus

import (
	"fmt"
	"path"
	"strings"
)

// ValidateObjectPath validates that an input object_path is a safe, portable,
// canonical relative path suitable for local directories and object storage.
//
// It rejects:
//   - Empty or whitespace-only paths
//   - Absolute filesystem paths (starting with '/' or Windows drive letters)
//   - Backslash path separators ('\')
//   - Parent directory traversal segments ('..')
//   - Current directory segments ('.')
//   - Empty path segments (such as double slashes '//' or trailing slashes)
//   - Non-canonical paths where path.Clean differs from the input
func ValidateObjectPath(p string) error {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return fmt.Errorf("object path must not be empty or whitespace")
	}

	if strings.Contains(p, "\\") {
		return fmt.Errorf("object path %q contains invalid path separator '\\'; must use forward slashes '/'", p)
	}

	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("object path %q must be relative, not absolute", p)
	}

	// Reject Windows drive prefix (e.g. "C:", "d:")
	if len(p) >= 2 && p[1] == ':' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) {
		return fmt.Errorf("object path %q contains Windows drive specification", p)
	}

	segments := strings.Split(p, "/")
	for _, seg := range segments {
		if seg == "" {
			return fmt.Errorf("object path %q contains empty path segment (double slash or trailing slash)", p)
		}
		if seg == "." {
			return fmt.Errorf("object path %q contains current directory segment '.'", p)
		}
		if seg == ".." {
			return fmt.Errorf("object path %q contains parent directory traversal segment '..'", p)
		}
	}

	cleaned := path.Clean(p)
	if cleaned != p {
		return fmt.Errorf("object path %q is not canonical (cleaned: %q)", p, cleaned)
	}

	return nil
}
