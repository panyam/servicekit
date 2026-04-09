package http

import (
	"testing"
)

// TestGenerateSessionID_Length verifies that GenerateSessionID returns a
// 32-character string (hex encoding of 16 random bytes = 128 bits of entropy),
// matching the MCP Streamable HTTP session ID format.
func TestGenerateSessionID_Length(t *testing.T) {
	id := GenerateSessionID()
	if len(id) != 32 {
		t.Errorf("GenerateSessionID() length = %d, want 32", len(id))
	}
}

// TestGenerateSessionID_HexOnly verifies that GenerateSessionID returns only
// lowercase hexadecimal characters (0-9, a-f).
func TestGenerateSessionID_HexOnly(t *testing.T) {
	id := GenerateSessionID()
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("GenerateSessionID() contains non-hex char %q in %q", string(c), id)
			break
		}
	}
}

// TestGenerateSessionID_Unique verifies that two consecutive calls to
// GenerateSessionID return different values (with 128 bits of entropy,
// collision probability is astronomically low).
func TestGenerateSessionID_Unique(t *testing.T) {
	a := GenerateSessionID()
	b := GenerateSessionID()
	if a == b {
		t.Errorf("GenerateSessionID() returned same value twice: %q", a)
	}
}
