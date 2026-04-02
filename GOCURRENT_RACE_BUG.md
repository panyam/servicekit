# gocurrent Race Condition Bug — File on github.com/panyam/gocurrent

## Title

`DATA RACE: RunnerBase.isRunning accessed without synchronization`

## Summary

`RunnerBase.isRunning` is a plain `bool` field accessed from multiple goroutines without synchronization, causing a data race detectable by `go test -race`.

## Repro

Any use of `Writer` where `Stop()` is called while the writer goroutine is still running triggers the race. Minimal repro:

```go
func TestWriterRace(t *testing.T) {
    w := gocurrent.NewWriter(func(msg string) error {
        return nil
    })
    w.Send("hello")
    w.Stop() // races with writer goroutine's IsRunning() call
}
```

Run with: `go test -race -run TestWriterRace`

## Race Report

```
WARNING: DATA RACE
Read at rwbase.go:35 by goroutine G1 (writer goroutine):
  RunnerBase.IsRunning()           → reads isRunning
  Writer.InputChan()    writer.go:75
  Writer.start.func1()  writer.go:119

Previous write at rwbase.go:57 by goroutine G2 (caller):
  RunnerBase.Stop()                → writes isRunning = false
```

## Root Cause

In `rwbase.go`, the `isRunning` field (line 11) is a plain `bool`. Three methods access it without any synchronization:

- **`IsRunning()` (line 35)** — reads `isRunning` (called from writer goroutine via `InputChan` at writer.go:75,119)
- **`start()` (line 43)** — writes `isRunning = true`
- **`Stop()` (line 57)** — writes `isRunning = false`

When `Stop()` is called from the main goroutine while the writer goroutine is in its `select` loop calling `InputChan() → IsRunning()`, Go's race detector flags the concurrent read-write on the plain bool.

## Suggested Fix

Replace `bool` with `atomic.Bool` (available since Go 1.19):

```go
// rwbase.go
type RunnerBase[C any] struct {
    controlChan chan C
    isRunning   atomic.Bool  // was: bool
    wg          sync.WaitGroup
    stopVal     C
}

func (r *RunnerBase[C]) IsRunning() bool {
    return r.isRunning.Load()
}

func (r *RunnerBase[C]) start() error {
    if r.IsRunning() {
        return errors.New("Channel already running")
    }
    r.isRunning.Store(true)
    r.wg.Add(1)
    return nil
}

func (r *RunnerBase[C]) Stop() error {
    if !r.IsRunning() && r.controlChan != nil {
        return nil
    }
    r.controlChan <- r.stopVal
    r.isRunning.Store(false)
    r.wg.Wait()
    return nil
}
```

`atomic.Bool` is preferred over `sync.RWMutex` since only a single boolean needs protection, and atomic ops have lower overhead.

## Impact

This race affects all users of `Writer` and `Reader` under `-race`. Discovered in `servicekit/http` tests for the new `SSEConn` type, but the same race exists with `BaseConn` (WebSocket) — it just isn't triggered in current WebSocket tests because those don't use `-race`.

## Versions

- gocurrent v0.0.10
- Go 1.26.1
- darwin/arm64
