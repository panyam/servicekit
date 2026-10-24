# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/). Releases up to v0.1.5 were tag-only
and are not back-filled here.

## [v0.2.0] - 2026-10-24

A breaking release that batches everything we had queued, so consumers
migrate once. A JSON-only WebSocket or SSE server no longer links grpc or
protobuf, and the root module no longer requires gorm or Connect. Most callers
only need the import and rename table in UPGRADING.md. The one change in
behaviour is `EventStore.Replay`, which now iterates instead of copying. The new
`hybrid` module (tagged `hybrid/v0.2.0`) serves one proto service over grpcws,
grpc-gateway JSON and Connect v2 with the same JSON on all three.

### Breaking
- `ProtoJSONCodec` and `BinaryProtoCodec` moved from `servicekit/http` to
  `servicekit/http/protocodec`. `SendJsonResponse`, `ErrorToHttpCode`,
  `WSConnWriteError` and `WSConnJSONReaderWriter` moved to
  `servicekit/http/grpcutil`. Behaviour is unchanged, and there are no aliases
  (see UPGRADING.md) (issue 17, PR 54).
- Go-style initialisms with no deprecated wrappers: `ConnID()` (on `WSConn`,
  `BiDirStreamConn`, `SSEConn`), `ConnIDStr`, `PingID`, `JSONGet`,
  `JSONToQueryString`, `NewJSONRequest`, `MakeURL`, `NormalizeWSURL`,
  `DefaultHTTPClient` and the QPS clients, `grpcutil.SendJSONResponse`,
  `grpcutil.ErrorToHTTPCode`. Wire JSON keys are unchanged (issue 56, PR 57).
- `EventStore.Replay` returns `iter.Seq2[StoredEvent, error]` instead of a
  slice. `ReplayAll` collects it. `MemoryEventStore` replays without copying,
  and without holding its lock while the caller writes each event (issue 20,
  PR 57).
- `connectbridge` moved into the `hybrid` module, at
  `servicekit/hybrid/connectbridge`. `NewConnectStreamBridge` takes any
  `Sender[T]`, which v1's `*connect.ServerStream[T]` still satisfies
  (issue 56, PR 57).
- `auth/` is deleted. goapplib and oneauth cover it (issue 56, PR 57).
- The root `go.mod` no longer requires gorm or Connect.

### Added
- `servicekit/hybrid`, a separate module, serves one proto service over
  grpcws, grpc-gateway JSON and Connect from one handler. `hybrid.ProtoJSON`
  gives every transport the same protojson options: by default the gateway
  emits zero values and Connect and grpcws don't, so the same message came back
  in two shapes. `hybrid.Mux` mounts the gateway under a prefix so it can't
  swallow the other routes. `hybrid/demo` and `make -C hybrid exercise` show
  it end to end (issue 50, PR 54). It's on Connect v2,
  `connectrpc.com/connect/v2` (issue 55, PR 57).

### Changed
- `servicekit/http` and `middleware` no longer import grpc or protobuf. A
  minimal JSON WebSocket server goes from 14.8 MB to 9.5 MB.
  `scripts/check-deps.sh` (also `make check-deps`) fails CI if either package
  picks them up again (issue 17, PR 54).

## [v0.1.12] - 2026-10-24

Lets a grpcws server close the WebSocket when its stream ends, and drop
clients that stop reading. massrelay needs both to evict slow consumers
(issue 47). It also fixes a server connection that outlived a clean client
close by up to the pong timeout, and a TS client bundle that imported nothing
(issue 46). The new options all default to off.

### Added
- `grpcws.ConnOptions`, embedded in all three stream handlers:
  `CloseOnStreamEnd` sends a 1000 close frame after `stream_end` and ends the
  connection, `CloseGracePeriod` (default 2s) bounds how long a stuck client
  can hold the socket after that, and `WriteTimeout` sets a per-write
  deadline (PR 48).
- `http.BaseConn.WriteTimeout` and `http.BaseConn.CloseAfterSend`, plus a
  `Close` variant on `OutgoingMessage` (PR 48).
- TS client: `GRPCWSClient` and `TypedGRPCWSClient` report `streamEnded` in
  `onClose`, so an app can tell a normal end from a dropped connection. The
  mock gains `simulateStreamEnd()` and `simulateClose(code, reason)` (PR 48).

### Changed
- TS client: `onClose` receives `CloseInfo` (`code`, `reason`, `wasClean`).
  Existing `() => {}` handlers still type-check, but code that calls
  `onClose()` directly needs an argument (PR 48).
- A failed write on `BaseConn` now closes the socket. Before, the Writer
  stopped but the read loop kept a connection open that could no longer send
  (PR 48).
- Any close or read error now ends `WSHandleConn`, even when `OnError`
  returns nil. The reader has already stopped by then, so carrying on only
  meant waiting for the pong timeout (PR 48).

### Fixed
- `WSHandleConn` now ends when the client closes cleanly (code 1000) or the
  server closes the socket. Both surfaced as `net.ErrClosed`, which
  `conc.Reader` reports only on `ClosedChan`, so the loop waited for the pong
  timeout (PR 48).
- TS client: a browser bundle of `@panyam/servicekit-client` imported nothing.
  The `browser` field named the IIFE script build, which has no exports, and
  esbuild prefers that field over `module` for a browser, so every named
  import was `undefined`. The package now has an `exports` map whose
  `import`, `browser` and `default` conditions name `dist/index.mjs` and whose
  `require` names `dist/index.js`. The script build moved to the `unpkg` and
  `jsdelivr` fields, and `./dist/*` and `./package.json` stay reachable by
  path. CI now builds the client before its tests (issue 46, PR 49).

## [v0.1.11] - 2026-10-23

Dependency patch release for five Dependabot alerts on the Go module (two
high, three moderate). No API changes.

### Fixed
- grpc v1.83.2 (from v1.82.1), gorilla/websocket v1.5.3 (from v1.5.0), and
  golang.org/x/net v0.58.0 with matching x/sys, x/text and genproto updates
  (PR 45).

## [v0.1.10] - 2026-10-23

Fixes the data race `go test -race` reported on every grpcws WebSocket close,
which kept mcpkit from running its massrelay room tests under `-race`. The fix
changes two exported field types, so callers that read them directly need a
one-line change. CI now runs the Go tests, which it didn't before.

### Changed
- **Breaking:** `grpcws.StreamMetrics.MsgsSent`/`MsgsReceived` and
  `http.BaseConn.PingId` are `atomic.Int64` instead of `int64`. Read them with
  `Load()`, or `StreamMetrics.Sent()`/`Received()`. Neither type may be copied
  after first use (issue 41, PR 42).
- TS client dev dependencies: esbuild 0.28, vite 8.3, postcss 8.5.31, and
  vitest 4.1.12 (PR 35). `npm audit` is clean.

### Fixed
- Data race on every grpcws WebSocket close: `OnClose` logged the message
  counters with plain reads while the stream goroutines were still
  incrementing them (issue 41, PR 42). `BaseConn.DebugInfo` had the same
  race on `PingId` against `SendPing`.

### Added
- CI runs `go vet` and `go test -race` on the Go module, and fails on files
  `gofmt` would change (PR 42, PR 44).

## [v0.1.9] - 2026-10-23

First release of `@panyam/servicekit-client` through CI. v0.1.8's publish
run failed before uploading, so 0.1.8 is not on npm.

### Changed
- grpc v1.82.1 and protobuf v1.36.11 (PR 34), with golang.org/x/net and
  x/sys patch updates.

### Fixed
- The shared `npm-publish.yml` reads npm 12's `npm pack --json` output, an
  object keyed by package name where npm 10/11 printed an array (PR 40).

### Added
- `npm-publish.yml` takes `package-manager` (`npm` or `pnpm`), `pnpm-version`
  and `test-command`, so pnpm repos can call it (PR 40).

## [v0.1.8] - 2026-10-23

The TS client now shares the Go module's version and publishes from CI
(PR 39). No Go changes.

### Changed
- `@panyam/servicekit-client` now carries the Go module's version (it was
  0.0.7 while the module was at v0.1.7), checked by `scripts/check-versions.sh`.
  The next release tag publishes both at the same number.
- The client's npm tarball no longer ships `src/__tests__` or the compiled
  `dist/__tests__`. Its build uses `tsconfig.build.json`, which excludes tests.
- esbuild ^0.27 (was ^0.24) for the client's ESM and browser bundles, and a
  regenerated lockfile, which had drifted from package.json so `npm ci` failed.

### Added
- `.github/workflows/npm-publish.yml`, a reusable workflow that publishes one
  npm package with npm trusted publishing (OIDC, no token). servicekit's own
  `publish-npm.yml` uses it, and so can other repos.
- CI (`ci.yml`) runs the version check and the TS client's type-check and tests.

## [v0.1.7] - 2026-10-23

WebSocket upgrades work behind `RequestLogger` (PR 38).

### Fixed
- WebSocket upgrades work behind `RequestLogger` again. Its `statusRecorder`
  now implements `http.Hijacker`, which gorilla/websocket (through v1.5.3)
  asserts on directly; before this every upgrade failed with "response does
  not implement http.Hijacker" and a 400. A successful hijack is logged as
  status 101. Hijacking a writer that can't be hijacked returns
  `http.ErrNotSupported`.

## [v0.1.6] - 2026-10-21

Graceful shutdown no longer hangs on long-lived requests (PR 37, issue 36).

### Changed
- `ListenAndServeGraceful` cancels every request context right after the
  `OnShutdown` callbacks and before draining, so handlers waiting on
  `r.Context().Done()` (Connect server streams, SSE outside `SSEHub`,
  long-polls) return straight away instead of holding the drain for the full
  timeout. A caller-set `srv.BaseContext` keeps its values. This is on by
  default; pass `WithCancelRequests(false)` if short in-flight requests must
  keep their context until the drain timeout.
- Signal handling stops as soon as shutdown starts, so a second Ctrl-C kills
  the process.
- `OriginChecker.CheckRequest` with no `Origin` header now accepts a `Host`
  that is on the allowlist, as well as localhost. A LAN address listed so a
  phone can reach a dev server now works for Origin-less requests.

### Added
- `WithCancelRequests(bool)` option.
- `ErrDrainTimeout`, returned when the drain times out. The remaining
  connections are closed with `srv.Close()` first. It wraps
  `context.DeadlineExceeded`, so existing `errors.Is` checks still match, and
  it lets callers tell a slow drain apart from a startup failure.
