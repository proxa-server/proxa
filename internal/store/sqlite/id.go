package sqlite

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// newID returns a sortable, opaque ID with the given prefix.
// Format: prefix_<12 hex chars seconds><12 hex chars rand>
// e.g. "svc_017c8b3d4e2a8f00a3b4c5d6"
//
// Not a KSUID per se, but lexically sortable by time and unique
// with overwhelming probability (96 bits random).
func newID(prefix string) string {
	now := time.Now().UTC().UnixNano() / 1e6 // ms
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	return prefix + "_" + hexBytes(uint64(now), 6) + hex.EncodeToString(buf)
}

func hexBytes(v uint64, n int) string {
	b := make([]byte, n)
	for i := n - 1; i >= 0; i-- {
		b[i] = byte(v & 0xff)
		v >>= 8
	}
	return hex.EncodeToString(b)
}
