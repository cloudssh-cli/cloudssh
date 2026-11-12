#!/bin/sh
# Stop hook: keep ./bin/cloudssh current so whatever the user runs after a
# session is built from the latest code. It rebuilds only when a Go source (or
# go.mod/go.sum) is newer than the binary, so idle turns cost nothing.
#
# A failing build exits 2: Claude Code then hands the output back to Claude,
# which must fix the build before finishing. stop_hook_active (set while we are
# already inside such a retry) prevents an endless loop.
cd "${CLAUDE_PROJECT_DIR:-$(dirname "$0")/..}" || exit 0

if [ -f bin/cloudssh ] && [ -z "$(find cmd internal go.mod go.sum -type f -newer bin/cloudssh -print -quit 2>/dev/null)" ]; then
  exit 0 # up to date
fi

if out=$(make build 2>&1); then
  exit 0
fi

if cat 2>/dev/null | grep -q '"stop_hook_active"[[:space:]]*:[[:space:]]*true'; then
  exit 0 # already retried once; do not loop
fi
printf 'make build failed, so ./bin/cloudssh is stale. Fix the build and finish again:\n%s\n' "$(printf '%s' "$out" | tail -25)" >&2
exit 2
