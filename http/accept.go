package http

import "strings"

// ParseAcceptTypes parses an HTTP Accept header and returns whether
// application/json and text/event-stream are present.
//
// Handles quality values (;q=N) and whitespace per RFC 7231 §5.3.2.
// The wildcard type */* matches both JSON and SSE.
//
// See: https://www.rfc-editor.org/rfc/rfc7231#section-5.3.2
func ParseAcceptTypes(accept string) (acceptsJSON, acceptsSSE bool) {
	for _, part := range strings.Split(accept, ",") {
		mediaType := strings.TrimSpace(part)
		if semi := strings.Index(mediaType, ";"); semi >= 0 {
			mediaType = strings.TrimSpace(mediaType[:semi])
		}
		switch mediaType {
		case "application/json":
			acceptsJSON = true
		case "text/event-stream":
			acceptsSSE = true
		case "*/*":
			acceptsJSON = true
			acceptsSSE = true
		}
	}
	return
}
