package localfile

import (
	"bytes"
	"io"
)

// VerifiedInput represents an immutable, verified corpus input payload.
// It encapsulates the raw byte payload so that callers cannot mutate the backing slice.
type VerifiedInput struct {
	objectPath string
	byteLength int64
	sha256Hex  string
	rawBytes   []byte // private unexported backing slice
}

// ObjectPath returns the relative object path of the verified input.
func (v *VerifiedInput) ObjectPath() string {
	return v.objectPath
}

// ByteLength returns the verified byte length of the input.
func (v *VerifiedInput) ByteLength() int64 {
	return v.byteLength
}

// SHA256 returns the verified lowercase hexadecimal SHA-256 digest of the input.
func (v *VerifiedInput) SHA256() string {
	return v.sha256Hex
}

// Len returns the byte length as an integer.
func (v *VerifiedInput) Len() int {
	return len(v.rawBytes)
}

// NewReader returns a new io.Reader reading from the immutable input bytes.
// Each call returns an independent reader starting at offset 0.
func (v *VerifiedInput) NewReader() io.Reader {
	return bytes.NewReader(v.rawBytes)
}

// BytesCopy returns a defensive copy of the raw input bytes.
// Mutating the returned slice has no effect on the VerifiedInput's internal buffer.
func (v *VerifiedInput) BytesCopy() []byte {
	cp := make([]byte, len(v.rawBytes))
	copy(cp, v.rawBytes)
	return cp
}
