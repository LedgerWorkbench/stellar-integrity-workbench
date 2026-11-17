package corpus

import (
	"fmt"
	"path"
	"strings"
)

// ValidateObjectPath validates that an input object_path is a safe, portable,
// canonical relative path suitable for both local filesystem storage and object storage.
//
// It rejects:
//   - Empty paths or paths with leading/trailing whitespace
//   - Absolute filesystem paths (starting with '/' or Windows drive letters)
//   - Backslash path separators ('\')
//   - Directory traversal segments ('..') or current directory segments ('.')
//   - Empty path segments (double slashes '//' or trailing slashes)
//   - Control characters or unsupported path characters (allowed: [a-zA-Z0-9_.-])
//   - Non-canonical paths where path.Clean differs from the input
func ValidateObjectPath(p string) error {
	if p == "" {
		return fmt.Errorf("object path must not be empty")
	}

	if strings.TrimSpace(p) != p {
		return fmt.Errorf("object path %q contains leading or trailing whitespace", p)
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

		// Each segment must start with alphanumeric, underscore, or hyphen
		first := seg[0]
		if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || (first >= '0' && first <= '9') || first == '_' || first == '-') {
			return fmt.Errorf("object path segment %q must start with alphanumeric, underscore, or hyphen", seg)
		}

		// All characters in segment must be in [a-zA-Z0-9_.-]
		for i := 0; i < len(seg); i++ {
			c := seg[i]
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
				return fmt.Errorf("object path %q contains unsupported or control character %q", p, c)
			}
		}
	}

	cleaned := path.Clean(p)
	if cleaned != p {
		return fmt.Errorf("object path %q is not canonical (cleaned: %q)", p, cleaned)
	}

	return nil
}
