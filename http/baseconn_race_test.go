package http

import (
	"sync"
	"testing"
)

// TestBaseConn_SendPingConcurrentWithDebugInfo reads the ping id from another
// goroutine while the connection loop sends pings.
func TestBaseConn_SendPingConcurrentWithDebugInfo(t *testing.T) {
	c := &BaseConn[any, any]{}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			c.DebugInfo()
		}
	}()
	for i := 0; i < 1000; i++ {
		c.SendPing()
	}
	wg.Wait()

	if got := c.PingId.Load(); got != 1000 {
		t.Errorf("PingId = %d, want 1000", got)
	}
}
