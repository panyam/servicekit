#!/usr/bin/env bash
# servicekit and @panyam/servicekit-client share one version, the one under
# "## Version" in CAPABILITIES.md. Fails when clients/typescript/package.json
# says otherwise, and, given a tag (v0.1.8), when the tag says otherwise too.
# CI runs it on every PR; publish-npm.yml runs it with the tag before publishing.
#
#   scripts/check-versions.sh [tag]
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
want=$(awk '/^## Version/ { getline; print; exit }' "$root/CAPABILITIES.md" | tr -d '[:space:]')
[ -n "$want" ] || { echo "no version under '## Version' in CAPABILITIES.md" >&2; exit 1; }

bad=0
got=$(node -p "require('$root/clients/typescript/package.json').version")
if [ "$got" != "$want" ]; then
  echo "clients/typescript/package.json is $got; CAPABILITIES.md is $want" >&2
  bad=1
fi
if [ $# -gt 0 ] && [ "${1#v}" != "$want" ]; then
  echo "tag $1 doesn't match CAPABILITIES.md's $want" >&2
  bad=1
fi
[ $bad = 0 ] && echo "all at $want"
exit $bad
