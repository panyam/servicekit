package http

import (
	"net/http"

	"github.com/panyam/servicekit/middleware"
)

// CORS is a convenience wrapper for local development that allows all origins.
// For production use, prefer middleware.CORS with an OriginChecker.
//
// Deprecated: Use middleware.CORS(nil) for allow-all, or
// middleware.CORS(middleware.NewOriginChecker(origins)) for production.
func CORS(next http.Handler) http.Handler {
	return middleware.CORS(nil)(next)
}
