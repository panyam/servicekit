package http

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AuthRetryConfig configures automatic retry on HTTP 401 (Unauthorized)
// and 403 (Forbidden) responses. This implements the standard OAuth 2.0
// Bearer token retry pattern: refresh on 401, scope step-up on 403.
//
// All fields are optional. If nil, the corresponding retry is skipped.
type AuthRetryConfig struct {
	// SetAuth injects authentication credentials into a request.
	// Typically sets the Authorization: Bearer header.
	// Called before each request attempt (including retries).
	SetAuth func(req *http.Request) error

	// OnUnauthorized is called when the server returns 401.
	// Should refresh the token so the next SetAuth call uses fresh credentials.
	// Return nil to retry, non-nil error to give up.
	OnUnauthorized func(resp *http.Response) error

	// OnForbidden is called when the server returns 403.
	// The response is provided so the handler can parse WWW-Authenticate
	// for required scopes. Return nil to retry, non-nil error to give up.
	OnForbidden func(resp *http.Response) error
}

// AuthRetryError is returned when authentication retry fails (401/403
// exhausted or callback returned an error). Captures the HTTP response
// metadata for diagnostic and programmatic use.
//
// RequiredScopes is automatically parsed from the WWW-Authenticate header's
// "scope" parameter per RFC 6750 §3, so callers don't need to parse it
// themselves.
type AuthRetryError struct {
	// StatusCode is the HTTP status (401 or 403).
	StatusCode int
	// Message describes the failure (response body or callback error message).
	Message string
	// WWWAuthenticate is the raw WWW-Authenticate header from the server response.
	WWWAuthenticate string
	// RequiredScopes are the scopes parsed from WWW-Authenticate per RFC 6750.
	// Populated automatically from the "scope" parameter. Empty if not present.
	RequiredScopes []string
	// Cause is the underlying error from the callback, if the failure was due
	// to a callback error rather than retry exhaustion. Nil when retries were
	// simply exhausted.
	Cause error
}

func (e *AuthRetryError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("auth error %d: %s: %v", e.StatusCode, e.Message, e.Cause)
	}
	return fmt.Sprintf("auth error %d: %s", e.StatusCode, e.Message)
}

func (e *AuthRetryError) Unwrap() error {
	return e.Cause
}

// newAuthRetryError builds an AuthRetryError from an HTTP response,
// automatically parsing scopes from WWW-Authenticate per RFC 6750.
func newAuthRetryError(statusCode int, body string, wwa string, cause error) *AuthRetryError {
	var scopes []string
	if wwa != "" {
		_, scopes, _ = ParseWWWAuthenticate(wwa)
	}
	return &AuthRetryError{
		StatusCode:      statusCode,
		Message:         body,
		WWWAuthenticate: wwa,
		RequiredScopes:  scopes,
		Cause:           cause,
	}
}

// DoWithAuthRetry executes an HTTP request with automatic retry on 401/403.
//
// Retry budget: max 1 retry for 401 (token refresh), max 1 retry for 403
// (scope step-up). Total max 2 retries per request.
//
// If cfg is nil, no auth handling is performed — 401/403 responses are
// returned as AuthRetryError.
//
// buildReq must create a new *http.Request each call (the body may be consumed
// on the previous attempt). do is typically httpClient.Do.
func DoWithAuthRetry(
	cfg *AuthRetryConfig,
	buildReq func() (*http.Request, error),
	do func(*http.Request) (*http.Response, error),
) (*http.Response, error) {
	var tried401, tried403 bool

	for {
		req, err := buildReq()
		if err != nil {
			return nil, err
		}

		if cfg != nil && cfg.SetAuth != nil {
			if err := cfg.SetAuth(req); err != nil {
				return nil, fmt.Errorf("auth: %w", err)
			}
		}

		resp, err := do(req)
		if err != nil {
			return nil, err
		}

		switch resp.StatusCode {
		case http.StatusUnauthorized: // 401
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			wwa := resp.Header.Get("Www-Authenticate")
			msg := strings.TrimSpace(string(body))

			if tried401 || cfg == nil || cfg.OnUnauthorized == nil {
				return nil, newAuthRetryError(401, msg, wwa, nil)
			}
			tried401 = true

			if err := cfg.OnUnauthorized(resp); err != nil {
				return nil, newAuthRetryError(401, msg, wwa, err)
			}
			continue

		case http.StatusForbidden: // 403
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			wwa := resp.Header.Get("Www-Authenticate")
			msg := strings.TrimSpace(string(body))

			if tried403 || cfg == nil || cfg.OnForbidden == nil {
				return nil, newAuthRetryError(403, msg, wwa, nil)
			}
			tried403 = true

			if err := cfg.OnForbidden(resp); err != nil {
				return nil, newAuthRetryError(403, msg, wwa, err)
			}
			continue

		default:
			return resp, nil
		}
	}
}
