// Package hash computes content hashes for integrity verification and dedup.
//
// It uses XXH3 (128-bit) — a non-cryptographic hash that is an order of
// magnitude faster than SHA-256, which matters across a multi-terabyte media
// library. 128 bits gives ample collision resistance for identifying files.
package hash

import (
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"

	"github.com/zeebo/xxh3"
)

// Algo is the hash algorithm identifier stored alongside hashes.
const Algo = "xxh3-128"

// File streams the file at path through the hasher and returns the hex digest.
func File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return Reader(f)
}

// Reader streams r through the hasher and returns the hex digest.
func Reader(r io.Reader) (string, error) {
	h := xxh3.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return Format(h.Sum128()), nil
}

// Format renders a 128-bit XXH3 sum as a 32-character hex string (big-endian).
func Format(u xxh3.Uint128) string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[0:8], u.Hi)
	binary.BigEndian.PutUint64(b[8:16], u.Lo)
	return hex.EncodeToString(b[:])
}

// Hasher computes an XXH3-128 digest incrementally — e.g. as an io.Writer in a
// MultiWriter so a file can be copied and hashed in a single read pass.
type Hasher struct{ h *xxh3.Hasher }

// New returns a fresh incremental hasher.
func New() *Hasher { return &Hasher{h: xxh3.New()} }

// Write feeds bytes into the digest.
func (w *Hasher) Write(p []byte) (int, error) { return w.h.Write(p) }

// Hex returns the current digest as a 32-character hex string.
func (w *Hasher) Hex() string { return Format(w.h.Sum128()) }
