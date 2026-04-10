package http

import (
	"errors"
	"net/http"
	"testing"
)

// TestHTTPStatusError_Error verifies the error string formatting for
// HTTPStatusError with and without a body.
func TestHTTPStatusError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  HTTPStatusError
		want string
	}{
		{"with body", HTTPStatusError{StatusCode: 503, Body: "service unavailable"}, "HTTP 503: service unavailable"},
		{"without body", HTTPStatusError{StatusCode: 500, Body: ""}, "HTTP 500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestIsHTTPTransient_5xx verifies that 5xx status codes are classified
// as transient (retriable) errors — the server experienced a temporary
// failure that may resolve on retry (overload, gateway timeout, etc.).
func TestIsHTTPTransient_5xx(t *testing.T) {
	for _, code := range []int{500, 502, 503, 504} {
		if !IsHTTPTransient(code) {
			t.Errorf("IsHTTPTransient(%d) = false, want true", code)
		}
	}
}

// TestIsHTTPTransient_4xx verifies that 4xx status codes are classified
// as terminal (non-retriable) errors — the client request was invalid
// and retrying won't help.
func TestIsHTTPTransient_4xx(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404, 409, 429} {
		if IsHTTPTransient(code) {
			t.Errorf("IsHTTPTransient(%d) = true, want false", code)
		}
	}
}

// TestIsHTTPTransient_2xx verifies that 2xx status codes are not classified
// as transient errors (they're successes, not errors at all).
func TestIsHTTPTransient_2xx(t *testing.T) {
	for _, code := range []int{200, 201, 202, 204} {
		if IsHTTPTransient(code) {
			t.Errorf("IsHTTPTransient(%d) = true, want false", code)
		}
	}
}

// TestHTTPStatusError_IsTransient verifies that HTTPStatusError integrates
// correctly with IsHTTPTransient — 5xx errors are transient, 4xx are not.
func TestHTTPStatusError_IsTransient(t *testing.T) {
	err5xx := &HTTPStatusError{StatusCode: 503}
	if !IsHTTPTransient(err5xx.StatusCode) {
		t.Error("503 should be transient")
	}

	err4xx := &HTTPStatusError{StatusCode: 404}
	if IsHTTPTransient(err4xx.StatusCode) {
		t.Error("404 should not be transient")
	}
}

// TestHTTPStatusError_HeaderAccessible verifies that the Header field on
// HTTPStatusError is populated and accessible via errors.As. This is important
// for callers that need to inspect response headers (e.g., Retry-After,
// X-Request-Id) from non-2xx error responses for diagnostics or retry logic.
func TestHTTPStatusError_HeaderAccessible(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-Id", "abc123")
	h.Set("Retry-After", "5")

	err := error(&HTTPStatusError{
		StatusCode: 503,
		Header:     h,
		Body:       "service unavailable",
	})

	var target *HTTPStatusError
	if !errors.As(err, &target) {
		t.Fatal("errors.As failed to match HTTPStatusError")
	}
	if got := target.Header.Get("X-Request-Id"); got != "abc123" {
		t.Errorf("Header X-Request-Id = %q, want %q", got, "abc123")
	}
	if got := target.Header.Get("Retry-After"); got != "5" {
		t.Errorf("Header Retry-After = %q, want %q", got, "5")
	}
}

// TestMaxErrorBodySize_Value verifies the MaxErrorBodySize constant is 16 KB.
// This constant caps error response body reads to prevent memory exhaustion
// from oversized error payloads.
func TestMaxErrorBodySize_Value(t *testing.T) {
	if MaxErrorBodySize != 16*1024 {
		t.Errorf("MaxErrorBodySize = %d, want %d", MaxErrorBodySize, 16*1024)
	}
}
