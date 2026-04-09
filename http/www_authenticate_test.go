package http

import (
	"testing"
)

// TestParseWWWAuthenticate_Scopes verifies extraction of the "scope" parameter
// from a WWW-Authenticate: Bearer header, per RFC 6750 §3.
// Multiple space-separated scopes should be returned as a string slice.
func TestParseWWWAuthenticate_Scopes(t *testing.T) {
	_, scopes, err := ParseWWWAuthenticate(`Bearer scope="read write admin"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopes) != 3 || scopes[0] != "read" || scopes[1] != "write" || scopes[2] != "admin" {
		t.Errorf("scopes = %v, want [read write admin]", scopes)
	}
}

// TestParseWWWAuthenticate_ResourceMetadata verifies extraction of the
// "resource_metadata" parameter from a WWW-Authenticate header. This is
// the MCP PRM (Protected Resource Metadata) discovery URL.
func TestParseWWWAuthenticate_ResourceMetadata(t *testing.T) {
	rm, _, err := ParseWWWAuthenticate(`Bearer resource_metadata="https://example.com/.well-known/oauth-protected-resource"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rm != "https://example.com/.well-known/oauth-protected-resource" {
		t.Errorf("resource_metadata = %q, want PRM URL", rm)
	}
}

// TestParseWWWAuthenticate_Both verifies extraction of both resource_metadata
// and scope parameters from the same header.
func TestParseWWWAuthenticate_Both(t *testing.T) {
	rm, scopes, err := ParseWWWAuthenticate(`Bearer resource_metadata="https://example.com/prm", scope="read write"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rm != "https://example.com/prm" {
		t.Errorf("resource_metadata = %q", rm)
	}
	if len(scopes) != 2 {
		t.Errorf("scopes = %v, want [read write]", scopes)
	}
}

// TestParseWWWAuthenticate_Empty verifies that an empty header returns
// empty values without error.
func TestParseWWWAuthenticate_Empty(t *testing.T) {
	rm, scopes, err := ParseWWWAuthenticate("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rm != "" || len(scopes) != 0 {
		t.Errorf("expected empty results, got rm=%q scopes=%v", rm, scopes)
	}
}

// TestParseWWWAuthenticate_NoBearer verifies that a header without the
// "Bearer " prefix is still parsed (some servers omit it).
func TestParseWWWAuthenticate_NoBearer(t *testing.T) {
	_, scopes, err := ParseWWWAuthenticate(`scope="openid profile"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopes) != 2 || scopes[0] != "openid" {
		t.Errorf("scopes = %v, want [openid profile]", scopes)
	}
}

// TestParseWWWAuthenticate_UnquotedScope verifies that unquoted scope
// values are handled (single scope without quotes).
func TestParseWWWAuthenticate_UnquotedScope(t *testing.T) {
	_, scopes, err := ParseWWWAuthenticate(`Bearer scope=read`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopes) != 1 || scopes[0] != "read" {
		t.Errorf("scopes = %v, want [read]", scopes)
	}
}
