---
name: race-safety-patterns
description: Race condition patterns to avoid in servicekit — ResponseWriter flush, Ready() channel
type: feedback
---

Never call flusher.Flush() on http.ResponseWriter from outside the conc.Writer goroutine after OnStart creates it. The initial header flush must happen inside OnStart, before conc.NewWriter().

Use BaseSSEConn.Ready() channel (lazily initialized via sync.Once) when accessing a connection from Validate before OnStart completes.

**Why:** Two separate data races were discovered and fixed: (1) SSEServe flush vs Writer goroutine flush, (2) test reading conn.Writer before OnStart writes it.

**How to apply:** Any new SSE-like connection type must flush headers before creating the Writer goroutine. Tests must wait for Ready() or startedChan before sending.
