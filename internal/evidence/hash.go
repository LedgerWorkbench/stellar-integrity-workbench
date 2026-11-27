package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// ComputeSHA256 returns the lowercase hexadecimal SHA-256 digest and exact byte
// length of data.
func ComputeSHA256(data []byte) (string, int64) {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), int64(len(data))
}

// HashReader streams r through SHA-256 and returns the lowercase hexadecimal
// digest and exact byte count read.
func HashReader(r io.Reader) (string, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// IsValidSHA256 reports whether s is a 64-character lowercase hexadecimal string.
func IsValidSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isDigit := c >= '0' && c <= '9'
		isLower := c >= 'a' && c <= 'f'
		if !isDigit && !isLower {
			return false
		}
	}
	return true
}
