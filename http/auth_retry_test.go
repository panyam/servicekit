package http

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestDoWithAuthRetry_Success verifies that a successful (200) response is
// returned directly with no retries.
func TestDoWithAuthRetry_Success(t *testing.T) {
	calls := 0
	resp, err := DoWithAuthRetry(
		nil, // no auth config
		func() (*http.Request, error) {
			return http.NewRequest("POST", "http://example.com/api", nil)
		},
		func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if calls != 1 {
		t.Errorf("do called %d times, want 1", calls)
	}
}

// TestDoWithAuthRetry_401_RefreshRetry verifies that a 401 response triggers
// OnUnauthorized, then retries the request. The second attempt should use
// the refreshed credentials (via SetAuth being called again on the new request).
func TestDoWithAuthRetry_401_RefreshRetry(t *testing.T) {
	attempt := 0
	resp, err := DoWithAuthRetry(
		&AuthRetryConfig{
			SetAuth: func(req *http.Request) error {
				if attempt == 0 {
					req.Header.Set("Authorization", "Bearer old")
				} else {
					req.Header.Set("Authorization", "Bearer new")
				}
				return nil
			},
			OnUnauthorized: func(resp *http.Response) error {
				return nil // signal: retry
			},
		},
		func() (*http.Request, error) {
			return http.NewRequest("POST", "http://example.com/api", nil)
		},
		func(req *http.Request) (*http.Response, error) {
			attempt++
			if req.Header.Get("Authorization") == "Bearer new" {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
			}
			return &http.Response{
				StatusCode: 401,
				Header:     http.Header{"Www-Authenticate": []string{`Bearer realm="test"`}},
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200 after retry", resp.StatusCode)
	}
	if attempt != 2 {
		t.Errorf("attempts = %d, want 2", attempt)
	}
}

// TestDoWithAuthRetry_401_GivesUp verifies that two consecutive 401 responses
// result in an error (max 1 retry for 401).
func TestDoWithAuthRetry_401_GivesUp(t *testing.T) {
	_, err := DoWithAuthRetry(
		&AuthRetryConfig{
			SetAuth:        func(req *http.Request) error { return nil },
			OnUnauthorized: func(resp *http.Response) error { return nil },
		},
		func() (*http.Request, error) {
			return http.NewRequest("POST", "http://example.com/api", nil)
		},
		func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 401,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
			}, nil
		},
	)
	if err == nil {
		t.Fatal("expected error after two 401s")
	}
}

// TestDoWithAuthRetry_403_ScopeStepUp verifies that a 403 response triggers
// OnForbidden with the required scopes parsed from WWW-Authenticate, then
// retries the request with stepped-up credentials.
func TestDoWithAuthRetry_403_ScopeStepUp(t *testing.T) {
	attempt := 0
	var receivedScopes []string
	resp, err := DoWithAuthRetry(
		&AuthRetryConfig{
			SetAuth: func(req *http.Request) error {
				req.Header.Set("Authorization", "Bearer token")
				return nil
			},
			OnForbidden: func(resp *http.Response) error {
				wwa := resp.Header.Get("Www-Authenticate")
				_, scopes, _ := ParseWWWAuthenticate(wwa)
				receivedScopes = scopes
				return nil // signal: retry
			},
		},
		func() (*http.Request, error) {
			return http.NewRequest("POST", "http://example.com/api", nil)
		},
		func(req *http.Request) (*http.Response, error) {
			attempt++
			if attempt > 1 {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
			}
			return &http.Response{
				StatusCode: 403,
				Header:     http.Header{"Www-Authenticate": []string{`Bearer scope="read write"`}},
				Body:       io.NopCloser(strings.NewReader("forbidden")),
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200 after step-up", resp.StatusCode)
	}
	if len(receivedScopes) != 2 || receivedScopes[0] != "read" || receivedScopes[1] != "write" {
		t.Errorf("scopes = %v, want [read write]", receivedScopes)
	}
}

// TestDoWithAuthRetry_403_NoHandler verifies that a 403 with no OnForbidden
// handler returns an error immediately (no retry possible).
func TestDoWithAuthRetry_403_NoHandler(t *testing.T) {
	_, err := DoWithAuthRetry(
		&AuthRetryConfig{
			SetAuth: func(req *http.Request) error { return nil },
			// OnForbidden intentionally nil
		},
		func() (*http.Request, error) {
			return http.NewRequest("POST", "http://example.com/api", nil)
		},
		func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 403,
				Body:       io.NopCloser(strings.NewReader("forbidden")),
			}, nil
		},
	)
	if err == nil {
		t.Fatal("expected error when OnForbidden is nil")
	}
}

// TestDoWithAuthRetry_401Then403 verifies the retry budget allows one 401
// retry AND one 403 retry in the same request lifecycle.
func TestDoWithAuthRetry_401Then403(t *testing.T) {
	attempt := 0
	resp, err := DoWithAuthRetry(
		&AuthRetryConfig{
			SetAuth: func(req *http.Request) error {
				req.Header.Set("Authorization", fmt.Sprintf("Bearer token-%d", attempt))
				return nil
			},
			OnUnauthorized: func(resp *http.Response) error { return nil },
			OnForbidden:    func(resp *http.Response) error { return nil },
		},
		func() (*http.Request, error) {
			return http.NewRequest("POST", "http://example.com/api", nil)
		},
		func(req *http.Request) (*http.Response, error) {
			attempt++
			switch attempt {
			case 1:
				return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(""))}, nil
			case 2:
				return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("")),
					Header: http.Header{"Www-Authenticate": []string{`Bearer scope="admin"`}}}, nil
			default:
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
			}
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if attempt != 3 {
		t.Errorf("attempts = %d, want 3", attempt)
	}
}

// TestDoWithAuthRetry_NilConfig verifies that nil AuthRetryConfig means no
// auth handling — requests pass through as-is, and 401/403 are returned
// directly as errors.
func TestDoWithAuthRetry_NilConfig(t *testing.T) {
	_, err := DoWithAuthRetry(
		nil,
		func() (*http.Request, error) {
			return http.NewRequest("POST", "http://example.com/api", nil)
		},
		func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 401,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
			}, nil
		},
	)
	if err == nil {
		t.Fatal("expected error for 401 with nil config")
	}
}
