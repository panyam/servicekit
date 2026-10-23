package grpcws

import (
	"sync"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestStreamConns_OnCloseConcurrentWithIncrements closes each stream conn
// type while its stream goroutines are still counting messages, which is what
// happens on every WebSocket close. Fails under -race if OnClose reads the
// counters without atomic loads (#41).
func TestStreamConns_OnCloseConcurrentWithIncrements(t *testing.T) {
	type ts = *timestamppb.Timestamp
	conns := map[string]interface {
		OnClose()
		streamMetrics() *StreamMetrics
	}{
		"bidi":   &BidiStreamConn[ts, ts, *testBidiStream]{},
		"client": &ClientStreamConn[ts, ts, *testClientStream]{},
		"server": &ServerStreamConn[ts, ts, *testServerStream]{},
	}
	for name, conn := range conns {
		t.Run(name, func(t *testing.T) {
			m := conn.streamMetrics()
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 1000; i++ {
					m.IncrementSent()
					m.IncrementReceived()
				}
			}()
			conn.OnClose()
			wg.Wait()

			if got := m.Sent(); got != 1000 {
				t.Errorf("Sent() = %d, want 1000", got)
			}
			if got := m.Received(); got != 1000 {
				t.Errorf("Received() = %d, want 1000", got)
			}
		})
	}
}

// TestSendPing_ConcurrentWithDebugInfo reads the ping id from another
// goroutine while the connection loop sends pings.
func TestSendPing_ConcurrentWithDebugInfo(t *testing.T) {
	c := &baseGRPCConn{}
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

func (c *baseGRPCConn) streamMetrics() *StreamMetrics { return &c.metrics }
