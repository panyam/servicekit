package main

// The observable_relay mission's exercise (issue 51): run the demo with a few
// hundred subscribers, some of which stop reading, and check that /metrics
// shows what the driver did.

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/panyam/servicekit/cmd/grpcws-demo/gen"
	"github.com/panyam/servicekit/grpcws"
	"github.com/panyam/servicekit/internal/loadtest"
)

type exerciseOpts struct {
	clients, stuck, kick int
	payload              int
	writeTimeout         time.Duration
}

func exerciseFlags() *exerciseOpts {
	o := &exerciseOpts{}
	flag.IntVar(&o.clients, "clients", 300, "exercise: subscribers to open")
	flag.IntVar(&o.stuck, "stuck", 10, "exercise: subscribers that never read")
	flag.IntVar(&o.kick, "kick", 20, "exercise: readers whose stream the server ends")
	flag.IntVar(&o.payload, "payload", 64<<10, "exercise: chat payload bytes per broadcast")
	flag.DurationVar(&o.writeTimeout, "exercise-write-timeout", time.Second, "exercise: WriteTimeout")
	return o
}

const exerciseGame = "exercise"

func runExercise(o exerciseOpts) error {
	if o.stuck+o.kick > o.clients {
		return fmt.Errorf("stuck+kick (%d) > clients (%d)", o.stuck+o.kick, o.clients)
	}
	baseGoroutines := runtime.NumGoroutine()
	start := time.Now()

	cfg := newDemoConfig(grpcws.ConnOptions{CloseOnStreamEnd: true, WriteTimeout: o.writeTimeout})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: newRouter(cfg)}
	go srv.Serve(ln)
	addr := ln.Addr().String()
	m := &scraper{url: "http://" + addr + "/metrics"}

	step := func(format string, args ...any) {
		fmt.Printf("[%5.1fs] %s\n", time.Since(start).Seconds(), fmt.Sprintf(format, args...))
	}

	player := func(i int) string { return fmt.Sprintf("p%04d", i) }
	pool, err := loadtest.Dial(context.Background(), "", loadtest.Options{
		Clients: o.clients,
		Stuck:   o.stuck,
		Count:   func(frame []byte) bool { return bytes.Contains(frame, []byte(`"chat-`)) },
		URLFor: func(i int) string {
			return fmt.Sprintf("ws://%s/ws/v1/%s/subscribe?player_id=%s", addr, exerciseGame, player(i))
		},
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	grpcSrv := `mode="server",transport="grpcws"`
	closed := func(reason string) string {
		return `servicekit_connections_closed_total{mode="server",reason="` + reason + `",transport="grpcws"}`
	}
	active := `servicekit_connections_active{` + grpcSrv + `}`

	if err := m.waitFor(active, float64(o.clients), 10*time.Second); err != nil {
		return err
	}
	step("%d subscribers connected (%d will never read)", o.clients, o.stuck)

	// Broadcast big chat messages until every stuck client has been dropped.
	// Each one waits for the slowest live reader to be within a few messages,
	// so only the stuck clients fall behind, however fast the machine is.
	room := hub.GetOrCreateRoom(exerciseGame)
	big := strings.Repeat("x", o.payload)
	const window = 4
	broadcasts := 0
	dropDeadline := time.Now().Add(60 * time.Second)
	for {
		if snap, err := m.scrape(); err != nil {
			return err
		} else if snap[closed("write_timeout")] >= float64(o.stuck) {
			break
		}
		if time.Now().After(dropDeadline) {
			return m.waitFor(closed("write_timeout"), float64(o.stuck), 0)
		}
		for range 5 {
			// The room drops a chat for a player whose queue is full, so a
			// reader can stay behind for good. Stop waiting once deliveries
			// stall as well.
			catchUp := time.Now().Add(5 * time.Second)
			last, lastMoved := pool.Received(), time.Now()
			for {
				low, ok := pool.Slowest()
				if !ok || low >= int64(broadcasts-window) || time.Now().After(catchUp) {
					break
				}
				if n := pool.Received(); n != last {
					last, lastMoved = n, time.Now()
				} else if time.Since(lastMoved) > 20*time.Millisecond {
					break
				}
				time.Sleep(time.Millisecond)
			}
			broadcasts++
			room.eventsChan <- &gen.GameEvent{
				EventId:   fmt.Sprintf("chat-%d", broadcasts),
				EventType: "chat",
				GameId:    exerciseGame,
				Payload:   &gen.GameEvent_ChatMessage{ChatMessage: &gen.ChatMessage{PlayerId: "driver", Message: big}},
			}
		}
	}
	step("broadcast %d x %dKB; the %d stuck subscribers were dropped", broadcasts, o.payload>>10, o.stuck)

	var checks []check
	snap, err := m.scrape()
	if err != nil {
		return err
	}
	checks = append(checks,
		check{"open after the drops", snap[active], float64(o.clients - o.stuck), 0},
		check{"dropped for write_timeout", snap[closed("write_timeout")], float64(o.stuck), 0})

	// End some readers' streams from the server side.
	readers := pool.Readers()
	for i := 0; i < o.kick; i++ {
		room.RemovePlayer(player(i))
	}
	if err := m.waitFor(closed("server_close"), float64(o.kick), 10*time.Second); err != nil {
		return err
	}
	serverClosed := 0
	for _, c := range readers[:o.kick] {
		select {
		case <-c.Ended():
		case <-time.After(2 * time.Second):
		}
		if c.ServerClosed() {
			serverClosed++
		}
	}
	step("ended %d streams from the server (CloseOnStreamEnd)", o.kick)
	checks = append(checks,
		check{"closed on stream end", must(m.scrape())[closed("server_close")], float64(o.kick), 0},
		check{"  ...and the client saw a close frame", float64(serverClosed), float64(o.kick), 0})

	// The rest leave on their own.
	pool.Close()
	left := o.clients - o.stuck - o.kick
	if err := m.waitFor(closed("client_close"), float64(left), 10*time.Second); err != nil {
		return err
	}
	step("the other %d subscribers closed", left)

	final, err := m.scrape()
	if err != nil {
		return err
	}
	checks = append(checks,
		check{"closed by the client", final[closed("client_close")], float64(left), 0},
		check{"open at the end", final[active], 0, 0},
		check{"opened", final[`servicekit_connections_opened_total{`+grpcSrv+`}`], float64(o.clients), 0},
		check{"stream-duration samples", final[`servicekit_connection_duration_seconds_count{`+grpcSrv+`}`], float64(o.clients), 0},
		check{"subscribe upgrades (HTTP 101)", final[`servicekit_http_requests_total{code="101",method="GET",route="/ws/v1/{gameId}/subscribe"}`], float64(o.clients), 0})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	g := settleGoroutines(baseGoroutines, 10*time.Second)
	checks = append(checks, check{"goroutines above the start", float64(max(g-baseGoroutines, 0)), 0, 10})

	fmt.Printf("\n%s/metrics, servicekit_ series with samples:\n", addr)
	keys := make([]string, 0, len(final))
	for k := range final {
		if strings.HasPrefix(k, "servicekit_") && !strings.Contains(k, "_bucket{") && final[k] != 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s %g\n", k, final[k])
	}
	fmt.Printf("  (driver read %d frames)\n\n", pool.Received())

	failed := 0
	for _, c := range checks {
		mark := "ok  "
		if !c.pass() {
			mark, failed = "FAIL", failed+1
		}
		want := fmt.Sprintf("%g", c.want)
		if c.slack > 0 {
			want = fmt.Sprintf("<= %g", c.want+c.slack)
		}
		fmt.Printf("  %s %-38s got %-6g want %s\n", mark, c.what, c.got, want)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d checks failed", failed, len(checks))
	}
	fmt.Printf("\nall %d checks passed\n", len(checks))
	return nil
}

type check struct {
	what      string
	got, want float64
	slack     float64
}

func (c check) pass() bool { return c.got >= c.want-c.slack && c.got <= c.want+c.slack }

func must(m map[string]float64, err error) map[string]float64 {
	if err != nil {
		return map[string]float64{}
	}
	return m
}

// settleGoroutines waits for the goroutine count to fall back to base, and
// returns where it ended up.
func settleGoroutines(base int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		n := runtime.NumGoroutine()
		if n <= base || time.Now().After(deadline) {
			return n
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// scraper reads /metrics over HTTP, the way Prometheus would, into a map
// keyed by the series exactly as the text format prints it.
type scraper struct{ url string }

func (s *scraper) scrape() (map[string]float64, error) {
	resp, err := http.Get(s.url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := map[string]float64{}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		i := strings.LastIndexByte(line, ' ')
		if i < 0 {
			continue
		}
		v, err := strconv.ParseFloat(line[i+1:], 64)
		if err != nil {
			continue
		}
		out[line[:i]] = v
	}
	return out, sc.Err()
}

func (s *scraper) waitFor(series string, want float64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var got float64
	for time.Now().Before(deadline) {
		snap, err := s.scrape()
		if err != nil {
			return err
		}
		if got = snap[series]; got == want {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	snap, _ := s.scrape()
	var related []string
	for k, v := range snap {
		if strings.HasPrefix(k, "servicekit_connections_") {
			related = append(related, fmt.Sprintf("  %s %g", k, v))
		}
	}
	sort.Strings(related)
	return fmt.Errorf("%s: got %g after %v, want %g\n%s", series, got, timeout, want, strings.Join(related, "\n"))
}
