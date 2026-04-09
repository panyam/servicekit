package http

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateSessionID returns a cryptographically random 32-character hex string
// (128 bits of entropy) suitable for HTTP session identifiers.
//
// Uses crypto/rand for security — the output is unpredictable and safe for
// use as session tokens, CSRF tokens, or any context where an attacker
// should not be able to guess the value.
//
// Panics if the system's cryptographic random number generator fails,
// which indicates a serious system-level problem.
func GenerateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
