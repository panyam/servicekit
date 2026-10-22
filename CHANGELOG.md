# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/). Releases up to v0.1.5 were tag-only
and are not back-filled here.

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
