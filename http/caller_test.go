package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type callTestUser struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func newJSONServer(t *testing.T, status int, body string, headers map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		if _, ok := headers["Content-Type"]; !ok && body != "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestCall_TypedDest(t *testing.T) {
	srv := newJSONServer(t, 200, `{"name":"alice","age":30}`, nil)
	defer srv.Close()

	req, err := http.NewRequest("GET", srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	got, err := Call[callTestUser](context.Background(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got.Name != "alice" || got.Age != 30 {
		t.Errorf("decoded = %+v, want {alice 30}", got)
	}
}

func TestCall_MapDest(t *testing.T) {
	srv := newJSONServer(t, 200, `{"name":"bob","age":42}`, nil)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	got, err := Call[map[string]any](context.Background(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got["name"] != "bob" {
		t.Errorf("got[name] = %v, want bob", got["name"])
	}
	// JSON numbers decode as float64 into any
	if got["age"].(float64) != 42 {
		t.Errorf("got[age] = %v, want 42", got["age"])
	}
}

func TestCallVoid_204NoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	req, _ := http.NewRequest("DELETE", srv.URL, nil)
	if err := CallVoid(context.Background(), req); err != nil {
		t.Fatalf("CallVoid: %v", err)
	}
}

func TestCall_204ReturnsZeroValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	req, _ := http.NewRequest("DELETE", srv.URL, nil)
	got, err := Call[callTestUser](context.Background(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got != (callTestUser{}) {
		t.Errorf("got = %+v, want zero value", got)
	}
}

func TestCall_EmptyBodyOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	if err := CallVoid(context.Background(), req); err != nil {
		t.Fatalf("CallVoid: %v", err)
	}
}

func TestCall_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	_, err := Call[callTestUser](ctx, req)
	if err == nil {
		t.Fatal("expected error from cancel, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestHTTPError_BodyAndHeader(t *testing.T) {
	srv := newJSONServer(t, 429, `[{"error":"rate limited"}]`, map[string]string{
		"Retry-After": "30",
		"X-RateLimit": "1000",
	})
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL, nil)
	_, err := Call[callTestUser](context.Background(), req)
	if err == nil {
		t.Fatal("expected HTTPError, got nil")
	}
	herr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("err type = %T, want *HTTPError", err)
	}
	if herr.Code != 429 {
		t.Errorf("Code = %d, want 429", herr.Code)
	}
	if string(herr.Body) != `[{"error":"rate limited"}]` {
		t.Errorf("Body = %q, want raw JSON array", string(herr.Body))
	}
	if got := herr.Header.Get("Retry-After"); got != "30" {
		t.Errorf("Retry-After = %q, want 30", got)
	}
	if got := herr.Header.Get("X-RateLimit"); got != "1000" {
		t.Errorf("X-RateLimit = %q, want 1000", got)
	}
}

func TestHTTPError_ErrorString(t *testing.T) {
	herr := &HTTPError{
		Code: 500,
		Body: []byte("internal boom"),
	}
	s := herr.Error()
	if !strings.Contains(s, "500") {
		t.Errorf("Error() = %q, missing status", s)
	}
	if !strings.Contains(s, "internal boom") {
		t.Errorf("Error() = %q, missing body", s)
	}
}

func TestHTTPErrorCode_Unchanged(t *testing.T) {
	if got := HTTPErrorCode(&HTTPError{Code: 404}); got != 404 {
		t.Errorf("HTTPErrorCode = %d, want 404", got)
	}
	if got := HTTPErrorCode(errors.New("plain")); got != -1 {
		t.Errorf("HTTPErrorCode plain = %d, want -1", got)
	}
}

func TestDefaultHttpClient_VerifiesTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	_, err := Call[map[string]any](context.Background(), req)
	if err == nil {
		t.Fatal("expected TLS verification failure, got nil")
	}
	if !strings.Contains(err.Error(), "x509") && !strings.Contains(err.Error(), "certificate") {
		t.Errorf("err = %v, expected x509/certificate error", err)
	}
}

func TestCall_WithInsecureTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	got, err := Call[map[string]any](context.Background(), req, WithInsecureTLS())
	if err != nil {
		t.Fatalf("Call WithInsecureTLS: %v", err)
	}
	if got["ok"] != true {
		t.Errorf("got = %v, want ok=true", got)
	}
}

func TestCall_WithClient(t *testing.T) {
	srv := newJSONServer(t, 200, `{"name":"x","age":1}`, nil)
	defer srv.Close()

	calls := 0
	custom := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return http.DefaultTransport.RoundTrip(r)
		}),
	}
	req, _ := http.NewRequest("GET", srv.URL, nil)
	_, err := Call[callTestUser](context.Background(), req, WithClient(custom))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if calls != 1 {
		t.Errorf("custom client used %d times, want 1", calls)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
