package http

import (
	"fmt"
	"net/url"
)

// ResolveURL resolves a URL reference against a base URL per RFC 3986 §5.2.2.
//
// This handles three cases:
//   - Absolute URL ("http://host/path"): returned unchanged
//   - Absolute path ("/path?q=1"): inherits scheme+host from base
//   - Relative path ("path?q=1"): inherits scheme+host+directory from base
//
// Used by SSE clients to resolve endpoint event URLs against the SSE
// connection URL, and generally useful for any URL reference resolution.
//
// See: https://www.rfc-editor.org/rfc/rfc3986#section-5.2.2
func ResolveURL(baseURL, ref string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parsing base URL: %w", err)
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("parsing reference URL %q: %w", ref, err)
	}
	return base.ResolveReference(r).String(), nil
}
