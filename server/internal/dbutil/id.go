package dbutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewHexID returns a random 128-bit identifier as 32 lowercase hex characters.
// It panics if the system entropy source fails, so an ID is never derived from
// predictable bytes.
func NewHexID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("secure random unavailable for ID generation: %w", err))
	}
	return hex.EncodeToString(b)
}
