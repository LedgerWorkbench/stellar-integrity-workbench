//go:build unix || linux || darwin

package localfile

import (
	"os"
	"syscall"
)

// openFileSafe opens a file relative to Root with O_NONBLOCK to guarantee that
// opening special files such as FIFOs will not block indefinitely waiting for a writer.
func openFileSafe(r *os.Root, name string) (*os.File, error) {
	return r.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
