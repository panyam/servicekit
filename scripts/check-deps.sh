#!/usr/bin/env bash
# The packages a JSON-only WebSocket/SSE server imports must not pull in grpc,
# protobuf, Connect or gorm. Those live in grpcws/, http/protocodec/,
# http/grpcutil/, connectbridge/, auth/ and the hybrid/ module. Fails with the
# offending packages and the path that brings each one in. CI runs it.
#
#   scripts/check-deps.sh
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

lean=(./http ./middleware)
heavy='^(google\.golang\.org/(grpc|protobuf|genproto)|connectrpc\.com|gorm\.io)(/|$)'

bad=$(go list -deps "${lean[@]}" | grep -E "$heavy" || true)
if [ -n "$bad" ]; then
  echo "lean packages (${lean[*]}) depend on heavy modules:" >&2
  echo "$bad" | sed 's/^/  /' >&2
  echo "imported directly by:" >&2
  go list -f '{{.ImportPath}}: {{join .Imports " "}}' "${lean[@]}" |
    tr ' ' '\n' | grep -E "$heavy|:$" | sed 's/^/  /' >&2
  exit 1
fi
