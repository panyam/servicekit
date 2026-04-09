package http

import "strings"

// ParseWWWAuthenticate extracts the resource_metadata URL and scopes from a
// WWW-Authenticate: Bearer header value per RFC 6750 §3.
//
// Used by HTTP clients to:
//   - Discover the Protected Resource Metadata (PRM) endpoint from a 401 response
//   - Parse required scopes from a 403 insufficient_scope response
//
// Handles both quoted ("value") and unquoted (value) parameter formats.
//
// See: https://www.rfc-editor.org/rfc/rfc6750#section-3
func ParseWWWAuthenticate(header string) (resourceMetadata string, scopes []string, err error) {
	header = strings.TrimSpace(header)
	if strings.HasPrefix(header, "Bearer ") {
		header = header[len("Bearer "):]
	}

	resourceMetadata = extractWWWAuthParam(header, "resource_metadata")
	scopeStr := extractWWWAuthParam(header, "scope")
	if scopeStr != "" {
		scopes = strings.Fields(scopeStr)
	}

	return resourceMetadata, scopes, nil
}

// extractWWWAuthParam extracts a named parameter value from a WWW-Authenticate
// header. Handles both quoted ("value") and unquoted (value) parameter formats.
// Ensures the match is a full parameter name (not a suffix like "noscope" when
// searching for "scope").
func extractWWWAuthParam(header, name string) string {
	search := name + "="
	idx := strings.Index(header, search)
	for idx >= 0 {
		if idx == 0 || header[idx-1] == ' ' || header[idx-1] == ',' {
			break
		}
		next := strings.Index(header[idx+1:], search)
		if next < 0 {
			return ""
		}
		idx = idx + 1 + next
	}
	if idx < 0 {
		return ""
	}
	rest := header[idx+len(search):]

	if len(rest) > 0 && rest[0] == '"' {
		end := strings.Index(rest[1:], `"`)
		if end < 0 {
			return rest[1:]
		}
		return rest[1 : end+1]
	}

	end := strings.IndexAny(rest, ", ")
	if end < 0 {
		return rest
	}
	return rest[:end]
}
