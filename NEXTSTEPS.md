# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- master — no open thread. v0.1.12 is being cut for #47 (PR #48) and #46 (PR #49).
- This run: converted the old TODO checklist. Its open items were already issues #11 to #17. Dropped the fix/issue-46-browser-field and feat/issue-47-close-on-stream-end work, since PRs #49 and #48 merged.

## Across threads

- Old TODO "Consider grpc-gateway integration for hybrid deployments" has no issue yet. It needs one filed, or it gets dropped.
- massrelay (panyam/massrelay#46) is waiting on v0.1.12. It needs both `CloseOnStreamEnd` and `WriteTimeout`, plus a lock-step bump of Go servicekit and npm servicekit-client.
