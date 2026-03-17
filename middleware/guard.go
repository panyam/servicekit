package middleware

import "net/http"

// Guard composes multiple middleware into a single wrapper.
// Middleware are applied in the order they are added via Use —
// the first middleware added is the outermost (runs first on request,
// last on response).
//
// Usage:
//
//	g := &Guard{}
//	g.Use(originChecker.Middleware, rateLimiter.Middleware(nil), auth.Middleware, connLimiter.Middleware)
//	http.Handle("/ws", g.Wrap(wsHandler))
type Guard struct {
	middlewares []func(http.Handler) http.Handler
}

// Use adds middleware to the guard chain. Nil middleware are silently skipped.
func (g *Guard) Use(mw ...func(http.Handler) http.Handler) {
	for _, m := range mw {
		if m != nil {
			g.middlewares = append(g.middlewares, m)
		}
	}
}

// Wrap applies all configured middleware to a handler.
// On a nil receiver, returns the handler unchanged.
func (g *Guard) Wrap(h http.Handler) http.Handler {
	if g == nil {
		return h
	}
	// Apply in reverse order so the first Use'd middleware is outermost
	for i := len(g.middlewares) - 1; i >= 0; i-- {
		h = g.middlewares[i](h)
	}
	return h
}
