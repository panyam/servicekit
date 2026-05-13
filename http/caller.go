package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	gut "github.com/panyam/goutils/utils"
)

// Default HTTP clients with TLS verification enabled. Suitable for calls to
// public APIs or any endpoint with a trusted certificate.
//
// For internal endpoints with self-signed certificates, either pass a custom
// *http.Client via WithClient or use the WithInsecureTLS option on Call/CallVoid.
var (
	DefaultHttpClient   *http.Client
	LowQPSHttpClient    *http.Client
	MediumQPSHttpClient *http.Client
	HighQPSHttpClient   *http.Client
)

// insecureDefaultHttpClient is the shared client used by WithInsecureTLS.
// Lazily initialized to avoid paying for a tls.Config when no caller asks for it.
var (
	insecureDefaultHttpClient     *http.Client
	insecureDefaultHttpClientOnce sync.Once
)

func init() {
	DefaultHttpClient = &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{},
	}
	LowQPSHttpClient = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
		},
	}
	MediumQPSHttpClient = &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        20,
			MaxIdleConnsPerHost: 10,
		},
	}
	HighQPSHttpClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        40,
			MaxIdleConnsPerHost: 20,
		},
	}
}

func getInsecureDefaultHttpClient() *http.Client {
	insecureDefaultHttpClientOnce.Do(func() {
		insecureDefaultHttpClient = &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	})
	return insecureDefaultHttpClient
}

// HTTPError is returned for non-2xx responses. Body and Header preserve the
// raw response for callers that need to parse structured error payloads or
// inspect headers like Retry-After.
type HTTPError struct {
	Code   int
	Body   []byte
	Header http.Header
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("Status: %d, Body: %s", e.Code, string(e.Body))
}

func HTTPErrorCode(err error) int {
	if err != nil {
		if e, ok := err.(*HTTPError); ok {
			return e.Code
		}
	}
	return -1
}

// CallOption customizes a Call or CallVoid invocation.
type CallOption func(*callConfig)

type callConfig struct {
	client *http.Client
}

// WithClient overrides the *http.Client used to perform the request.
// If unset, DefaultHttpClient is used.
func WithClient(c *http.Client) CallOption {
	return func(cfg *callConfig) { cfg.client = c }
}

// WithInsecureTLS uses a package-shared client whose Transport has
// InsecureSkipVerify=true. Useful for internal endpoints with self-signed
// certificates. Mutually exclusive with WithClient — last option wins.
func WithInsecureTLS() CallOption {
	return func(cfg *callConfig) { cfg.client = getInsecureDefaultHttpClient() }
}

// Call performs req, reads the entire response body, and JSON-decodes it into T.
//
// Contract: this helper is for request/response endpoints whose body fits in
// memory (JSON APIs, typed CRUD). For streaming/large bodies use client.Do
// directly; for SSE see SSEReader.
//
// Non-2xx responses return a zero T and *HTTPError carrying the status,
// raw body, and response headers. Empty bodies on 2xx return a zero T with
// no error (handles 204 No Content).
func Call[T any](ctx context.Context, req *http.Request, opts ...CallOption) (T, error) {
	var zero T
	body, _, err := doCall(ctx, req, opts)
	if err != nil {
		return zero, err
	}
	if len(body) == 0 {
		return zero, nil
	}
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return zero, err
	}
	return out, nil
}

// CallVoid performs req and discards the response body. Use for endpoints
// where the body is not needed (DELETE, ack-style POST). Non-2xx still
// produces an *HTTPError with body and headers preserved.
func CallVoid(ctx context.Context, req *http.Request, opts ...CallOption) error {
	_, _, err := doCall(ctx, req, opts)
	return err
}

func doCall(ctx context.Context, req *http.Request, opts []CallOption) ([]byte, *http.Response, error) {
	cfg := callConfig{client: DefaultHttpClient}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.client == nil {
		cfg.client = DefaultHttpClient
	}

	req = req.WithContext(ctx)
	resp, err := cfg.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, err
	}
	if resp.StatusCode >= 400 {
		return nil, resp, &HTTPError{
			Code:   resp.StatusCode,
			Body:   body,
			Header: resp.Header.Clone(),
		}
	}
	return body, resp, nil
}

// MakeUrl creates a URL from host, path, and optional pre-encoded query args.
func MakeUrl(host, path string, args string) (url string) {
	path = strings.TrimPrefix(path, "/")
	url = fmt.Sprintf("%s/%s", host, path)
	if args != "" {
		url += "?" + args
	}
	return url
}

// NewRequest builds an http.Request with Content-Type: application/json.
func NewRequest(method string, endpoint string, bodyReader io.Reader) (req *http.Request, err error) {
	req, err = http.NewRequest(method, endpoint, bodyReader)
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return
}

// NewJsonRequest marshals body as JSON and wraps NewRequest.
func NewJsonRequest(method string, endpoint string, body map[string]any) (req *http.Request, err error) {
	var bodyBytes []byte
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	return NewBytesRequest(method, endpoint, bodyBytes)
}

// NewBytesRequest wraps NewRequest with a byte-slice body.
func NewBytesRequest(method string, endpoint string, body []byte) (req *http.Request, err error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewBuffer(body)
	}
	return NewRequest(method, endpoint, bodyReader)
}

// JsonGet performs a GET and decodes the body via gut.JsonDecodeBytes.
// onReq, if non-nil, is invoked to customize the request before sending.
//
// Prefer Call[T] for typed responses; JsonGet remains for callers that
// want the raw response alongside the decoded body.
func JsonGet(url string, onReq func(req *http.Request)) (any, *http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, err
	}
	if onReq != nil {
		onReq(req)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, resp, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error reading body: ", string(body), err)
	}
	result, err := gut.JsonDecodeBytes(body)
	if err != nil {
		log.Println("Error decoding json: ", string(body), err)
	}
	return result, resp, err
}
