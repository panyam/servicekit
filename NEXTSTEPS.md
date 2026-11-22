# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No active mission and no open threads. servicekit takes pushdowns from consumer missions (massrelay, mcpkit). Off-mission and ready: #15, #16 (P3). Parked with triggers: #12, #14, #21, #23, #24, #69.
- v0.2.5 and hybrid/v0.2.5 shipped #68. Still to do: open an agni issue to move `internal/authgate` to servicekit's `TrustedProxies` and `CheckPlaintext` (#67 asks for it).
- #64: galore PR 17, massrelay PR 83, notations PR 37 and tlex PR 15 move them to ci-actions npm-publish. Once all four merge, delete `.github/workflows/npm-publish.yml` and close #64.
- `TestWSLoad_SlowConsumer` (`http/load_test.go`) failed once on CI with "1 of 9 readers read fewer than 40 frames" and passed on rerun. File it if it recurs.
- Likeliest next pushdown: panyam/mcpkit#267 (generic `RetryWithBackoff`), still `waiting` in mcpkit.
- This run: no open threads to carry. Merged PRs #58, #62, #63, #66, #68 and #70 still have remote branches; delete them by hand.

## Across threads

