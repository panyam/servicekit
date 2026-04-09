package http

import (
	"testing"
)

// TestResolveURL_AbsoluteURL verifies that an absolute URL reference is
// returned unchanged, per RFC 3986 §5.2.2 — an absolute URI overrides the base.
func TestResolveURL_AbsoluteURL(t *testing.T) {
	got, err := ResolveURL("http://localhost:8080/sse", "http://other:9090/path?q=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://other:9090/path?q=1" {
		t.Errorf("got %q, want %q", got, "http://other:9090/path?q=1")
	}
}

// TestResolveURL_AbsolutePath verifies that an absolute path reference
// inherits scheme+host from the base URL but replaces the path.
func TestResolveURL_AbsolutePath(t *testing.T) {
	got, err := ResolveURL("http://localhost:8080/mcp/sse", "/mcp/message?sessionId=abc")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://localhost:8080/mcp/message?sessionId=abc" {
		t.Errorf("got %q, want %q", got, "http://localhost:8080/mcp/message?sessionId=abc")
	}
}

// TestResolveURL_RelativePath verifies that a relative path reference
// inherits scheme+host+directory from the base URL, resolving against the
// base path's directory.
func TestResolveURL_RelativePath(t *testing.T) {
	got, err := ResolveURL("http://localhost:8080/mcp/sse", "message?sessionId=abc")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://localhost:8080/mcp/message?sessionId=abc" {
		t.Errorf("got %q, want %q", got, "http://localhost:8080/mcp/message?sessionId=abc")
	}
}

// TestResolveURL_DifferentHost verifies that a reference with a different
// host is used as-is (absolute URL takes precedence).
func TestResolveURL_DifferentHost(t *testing.T) {
	got, err := ResolveURL("http://localhost:8080/sse", "https://proxy.example.com/mcp")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://proxy.example.com/mcp" {
		t.Errorf("got %q, want %q", got, "https://proxy.example.com/mcp")
	}
}

// TestResolveURL_PreservesQueryParams verifies that query parameters in the
// reference URL are preserved through resolution.
func TestResolveURL_PreservesQueryParams(t *testing.T) {
	got, err := ResolveURL("http://host/base", "/path?a=1&b=2")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://host/path?a=1&b=2" {
		t.Errorf("got %q, want %q", got, "http://host/path?a=1&b=2")
	}
}

// TestResolveURL_InvalidBase verifies that an unparseable base URL returns
// an error rather than panicking.
func TestResolveURL_InvalidBase(t *testing.T) {
	_, err := ResolveURL("://bad", "/path")
	if err == nil {
		t.Fatal("expected error for invalid base URL")
	}
}

// TestResolveURL_InvalidRef verifies that an unparseable reference URL returns
// an error rather than panicking.
func TestResolveURL_InvalidRef(t *testing.T) {
	_, err := ResolveURL("http://host/base", "://bad")
	if err == nil {
		t.Fatal("expected error for invalid reference URL")
	}
}
