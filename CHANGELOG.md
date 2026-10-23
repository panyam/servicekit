# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/). Releases up to v0.1.5 were tag-only
and are not back-filled here.

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
