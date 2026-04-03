---
name: testing-patterns
description: Preferred testing patterns for servicekit — channel-driven test doubles, no codegen, httptest
type: feedback
---

Use channel-driven test doubles for grpcws integration tests instead of real gRPC servers. *timestamppb.Timestamp satisfies proto.Message without codegen. httptest.NewServer + gorilla/websocket for WS tests.

**Why:** Tests run in <0.5s for 28 tests. No proto codegen dependencies. In-process httptest is goroutine-to-goroutine with minimal overhead.

**How to apply:** When writing new streaming tests, follow grpcws/integration_test.go patterns. Use channels for synchronization, not time.Sleep.
