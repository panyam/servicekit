package http

import (
	"fmt"
	"net/http"
)

// MaxErrorBodySize is the maximum number of bytes read from an HTTP error
// response body. Prevents memory exhaustion from malicious or misconfigured
// servers returning oversized error payloads. Shared by DoWithAuthRetry and
// any other error-body readers in this package.
const MaxErrorBodySize = 16 << 10 // 16 KB

// HTTPStatusError represents an HTTP response with a non-2xx status code.
// It captures the status code, response headers, and response body for error
// reporting, classification (e.g., 5xx errors are typically transient/retriable),
// and programmatic inspection (e.g., reading WWW-Authenticate or Retry-After).
type HTTPStatusError struct {
	StatusCode int
	Header     http.Header
	Body       string
}

func (e *HTTPStatusError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

// IsHTTPTransient returns true if the HTTP status code indicates a transient
// server error (5xx) that may succeed on retry. Client errors (4xx) and
// successes (2xx) return false.
//
// Use this to decide whether to retry a failed HTTP request:
//   - 500, 502, 503, 504: transient (server overload, gateway timeout)
//   - 400, 401, 403, 404: terminal (client error, won't change on retry)
func IsHTTPTransient(statusCode int) bool {
	return statusCode >= 500
}
