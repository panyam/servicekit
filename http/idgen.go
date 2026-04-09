package http

import (
	"strconv"
	"sync/atomic"
)

// IDGen generates unique string IDs. Implementations must be safe for
// concurrent use. IDs are opaque — callers must not assume any ordering,
// format, or structure beyond uniqueness within the generator's scope.
type IDGen interface {
	Next() string
}

// AtomicIDGen is an IDGen backed by an atomic counter. It produces
// decimal string IDs ("1", "2", "3", ...). Suitable for per-session
// or per-stream ID generation where global uniqueness is not required.
//
// Zero value is ready to use.
type AtomicIDGen struct {
	counter atomic.Int64
}

// Next returns the next unique ID as a decimal string.
func (g *AtomicIDGen) Next() string {
	return strconv.FormatInt(g.counter.Add(1), 10)
}
