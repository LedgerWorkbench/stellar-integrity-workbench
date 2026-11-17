//go:build windows

package localfile

import (
	"os"
)

// openFileSafe opens a file relative to Root on Windows platforms.
func openFileSafe(r *os.Root, name string) (*os.File, error) {
	return r.OpenFile(name, os.O_RDONLY, 0)
}
