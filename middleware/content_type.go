package middleware

import (
	"net/http"
	"strings"
)

// RequireContentType returns middleware that rejects POST, PUT, and PATCH
// requests whose Content-Type header does not match any of the accepted
// types (prefix match). GET, DELETE, OPTIONS, and HEAD requests are passed
// through since they typically have no body.
//
// Returns 415 Unsupported Media Type for requests with a missing or
// mismatched Content-Type. This is a defense-in-depth measure against CSRF
// via cross-origin form submissions (browsers send forms as
// application/x-www-form-urlencoded without CORS preflight).
//
// Example:
//
//	// Single type
//	mux.Handle("/api", middleware.RequireContentType("application/json")(handler))
//
//	// Multiple types
//	mux.Handle("/rpc", middleware.RequireContentType("application/json", "application/json-rpc")(handler))
func RequireContentType(accepted ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
				ct := r.Header.Get("Content-Type")
				matched := false
				for _, a := range accepted {
					if strings.HasPrefix(ct, a) {
						matched = true
						break
					}
				}
				if !matched {
					http.Error(w, "Content-Type must be one of: "+strings.Join(accepted, ", "), http.StatusUnsupportedMediaType)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
