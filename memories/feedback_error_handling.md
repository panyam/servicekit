---
name: error-handling-patterns
description: Error handling patterns enforced in servicekit — no os.Exit in library code, header ordering, marshal error handling
type: feedback
---

Library code must NEVER call log.Fatal/log.Fatalln/os.Exit — always return errors to the caller.

Set Content-Type header BEFORE calling WriteHeader on http.ResponseWriter (Go locks headers after WriteHeader).

Never discard json.Marshal errors with `_` — at minimum log them and return.

When a guard condition checks `err != nil` to proceed with success-path logic, verify the condition isn't inverted (`err == nil` is usually correct).

**Why:** Seven bugs found in a code quality audit. Three were critical: error messages never sent to clients (inverted condition), headers silently dropped (wrong ordering), and os.Exit killing processes on recoverable errors.

**How to apply:** During code review, check for these patterns. The audit in commit 002ea67 documents all seven fixes.
