package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestConnLimiter_Nil(t *testing.T) {
	c := NewConnLimiter(0)
	if c != nil {
		t.Error("expected nil for max=0")
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := c.Middleware(ok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Errorf("got %d, want 200", w.Code)
	}
}

func TestConnLimiter_RejectsOverLimit(t *testing.T) {
	c := NewConnLimiter(2)
	blocker := make(chan struct{})

	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocker
		w.WriteHeader(200)
	})
	h := c.Middleware(slow)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		}()
	}

	for c.Active() < 2 {
		// spin until both are active
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("got %d, want 503", w.Code)
	}

	close(blocker)
	wg.Wait()

	if c.Active() != 0 {
		t.Errorf("active = %d, want 0 after release", c.Active())
	}

	fast := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h2 := c.Middleware(fast)
	w2 := httptest.NewRecorder()
	h2.ServeHTTP(w2, httptest.NewRequest("GET", "/", nil))
	if w2.Code != 200 {
		t.Errorf("got %d, want 200 after slots freed", w2.Code)
	}
}

func TestConnLimiter_Active(t *testing.T) {
	c := NewConnLimiter(10)
	if c.Active() != 0 {
		t.Errorf("initial active = %d, want 0", c.Active())
	}

	var nilC *ConnLimiter
	if nilC.Active() != 0 {
		t.Errorf("nil active = %d, want 0", nilC.Active())
	}
}
