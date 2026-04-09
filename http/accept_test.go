package http

import (
	"testing"
)

// TestParseAcceptTypes_JSON verifies that "application/json" is correctly
// detected in the Accept header.
func TestParseAcceptTypes_JSON(t *testing.T) {
	json, sse := ParseAcceptTypes("application/json")
	if !json {
		t.Error("expected acceptsJSON=true for application/json")
	}
	if sse {
		t.Error("expected acceptsSSE=false for application/json")
	}
}

// TestParseAcceptTypes_SSE verifies that "text/event-stream" is correctly
// detected in the Accept header.
func TestParseAcceptTypes_SSE(t *testing.T) {
	json, sse := ParseAcceptTypes("text/event-stream")
	if json {
		t.Error("expected acceptsJSON=false for text/event-stream")
	}
	if !sse {
		t.Error("expected acceptsSSE=true for text/event-stream")
	}
}

// TestParseAcceptTypes_Both verifies that both JSON and SSE are detected
// when the Accept header contains both types (common in MCP Streamable HTTP).
func TestParseAcceptTypes_Both(t *testing.T) {
	json, sse := ParseAcceptTypes("application/json, text/event-stream")
	if !json {
		t.Error("expected acceptsJSON=true")
	}
	if !sse {
		t.Error("expected acceptsSSE=true")
	}
}

// TestParseAcceptTypes_Wildcard verifies that "*/*" matches both JSON and SSE,
// per RFC 7231 §5.3.2 (wildcard matches any media type).
func TestParseAcceptTypes_Wildcard(t *testing.T) {
	json, sse := ParseAcceptTypes("*/*")
	if !json {
		t.Error("expected acceptsJSON=true for */*")
	}
	if !sse {
		t.Error("expected acceptsSSE=true for */*")
	}
}

// TestParseAcceptTypes_WithQuality verifies that quality values (q=) in the
// Accept header are stripped correctly per RFC 7231 §5.3.2. The presence of
// a quality value should not prevent type matching.
func TestParseAcceptTypes_WithQuality(t *testing.T) {
	json, sse := ParseAcceptTypes("text/event-stream;q=0.9, application/json;q=1.0")
	if !json {
		t.Error("expected acceptsJSON=true with quality values")
	}
	if !sse {
		t.Error("expected acceptsSSE=true with quality values")
	}
}

// TestParseAcceptTypes_Empty verifies that an empty Accept header returns
// false for both types (no content types accepted).
func TestParseAcceptTypes_Empty(t *testing.T) {
	json, sse := ParseAcceptTypes("")
	if json {
		t.Error("expected acceptsJSON=false for empty header")
	}
	if sse {
		t.Error("expected acceptsSSE=false for empty header")
	}
}

// TestParseAcceptTypes_Unknown verifies that unrecognized media types are
// ignored and don't affect JSON/SSE detection.
func TestParseAcceptTypes_Unknown(t *testing.T) {
	json, sse := ParseAcceptTypes("text/html, image/png")
	if json {
		t.Error("expected acceptsJSON=false for unknown types")
	}
	if sse {
		t.Error("expected acceptsSSE=false for unknown types")
	}
}
