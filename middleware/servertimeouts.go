package middleware

import (
	"net/http"
	"time"
)

// ApplyDefaults sets sensible timeout defaults on an http.Server,
// only overwriting fields that are zero-valued.
//
// Defaults:
//   - ReadTimeout: 10s
//   - WriteTimeout: 30s
//   - IdleTimeout: 120s
//   - ReadHeaderTimeout: 5s
//
// For SSE or long-lived streaming endpoints, callers should either set
// WriteTimeout to a non-zero value before calling ApplyDefaults (it
// won't overwrite), or set WriteTimeout = 0 after calling this function
// to disable the write deadline for streaming responses.
func ApplyDefaults(srv *http.Server) {
	if srv == nil {
		return
	}
	if srv.ReadTimeout == 0 {
		srv.ReadTimeout = 10 * time.Second
	}
	if srv.WriteTimeout == 0 {
		srv.WriteTimeout = 30 * time.Second
	}
	if srv.IdleTimeout == 0 {
		srv.IdleTimeout = 120 * time.Second
	}
	if srv.ReadHeaderTimeout == 0 {
		srv.ReadHeaderTimeout = 5 * time.Second
	}
}
