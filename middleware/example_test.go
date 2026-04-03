package middleware_test

import (
	"fmt"
	"net/http"

	mw "github.com/panyam/servicekit/middleware"
)

// ExampleGuard demonstrates composing middleware into a chain using Guard.
// Middleware are applied in Use() order — the first added is outermost
// (runs first on request, last on response). Nil middleware are silently skipped.
func ExampleGuard() {
	// Create middleware components
	rl := mw.NewRateLimiter(mw.RateLimitConfig{
		GlobalPerSec: 100,
		PerKeyPerSec: 10,
	})
	oc := mw.NewOriginChecker([]string{
		"https://example.com",
		"https://*.example.com",
	})

	// Compose into a Guard chain
	g := &mw.Guard{}
	g.Use(
		oc.Middleware,           // Check origin first
		rl.Middleware(nil),      // Then rate limit (nil key = use ClientIP)
		mw.Recovery,             // Catch panics
	)

	// Wrap your handler
	handler := g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))

	http.Handle("/api", handler)
	_ = handler
	fmt.Println("Guard chain configured")
	// Output: Guard chain configured
}

// ExampleApplyDefaults demonstrates setting sensible timeout defaults on
// http.Server. For SSE or WebSocket endpoints, set WriteTimeout = 0 after
// calling ApplyDefaults to prevent the server from killing long-lived
// connections.
func ExampleApplyDefaults() {
	srv := &http.Server{Addr: ":8080"}
	mw.ApplyDefaults(srv)

	// For SSE/WebSocket servers, override WriteTimeout
	srv.WriteTimeout = 0

	fmt.Printf("ReadTimeout: %v, WriteTimeout: %v, IdleTimeout: %v\n",
		srv.ReadTimeout, srv.WriteTimeout, srv.IdleTimeout)
	// Output: ReadTimeout: 10s, WriteTimeout: 0s, IdleTimeout: 2m0s
}

// ExampleNewRateLimiter demonstrates per-key rate limiting with a custom
// key function. The default key is client IP; override it to rate limit
// per user, per API key, or per any request attribute.
func ExampleNewRateLimiter() {
	rl := mw.NewRateLimiter(mw.RateLimitConfig{
		GlobalPerSec: 100,
		PerKeyPerSec: 5,
		PerKeyBurst:  3,
	})

	// Rate limit by API key header instead of IP
	apiKeyFunc := func(r *http.Request) string {
		return r.Header.Get("X-API-Key")
	}

	handler := rl.Middleware(apiKeyFunc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))

	_ = handler
	fmt.Println("Rate limiter configured")
	// Output: Rate limiter configured
}

// ExampleNewOriginChecker demonstrates WebSocket origin validation with
// exact matches, wildcard subdomains, and localhost support. The checker
// is typically used via Guard middleware, but Check() can be called directly.
func ExampleNewOriginChecker() {
	oc := mw.NewOriginChecker([]string{
		"https://example.com",       // exact match
		"*.example.com",             // wildcard subdomain
		"localhost",                 // any localhost port
	})

	fmt.Println(oc.Check("https://example.com"))       // true
	fmt.Println(oc.Check("https://app.example.com"))   // true
	fmt.Println(oc.Check("http://localhost:3000"))      // true
	fmt.Println(oc.Check("https://evil.com"))           // false
	// Output:
	// true
	// true
	// true
	// false
}
