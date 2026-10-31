#!/bin/sh
# Claude Code SessionStart hook: injects `myself prime` as session context.
#
# Reads the hook JSON on stdin and prints the primed brain on stdout, which
# Claude Code adds as context for SessionStart. Also records when the session
# started, in ${TMPDIR:-/tmp}/myself-hooks/<session_id>.start, so the Stop hook
# (stop-nudge.sh) can tell whether anything was remembered during the session.
# The marker is written once per session id, so clear and compact keep the
# original start time. On resume (source "resume") the start time is refreshed
# and the session's .nudged marker removed, so the Stop hook can nudge a
# resumed session once more.
#
# Never fails the session: if myself is missing or errors, it prints nothing
# and exits 0.
#
# Environment:
#   MYSELF_BIN           myself binary (default: myself on PATH)
#   MYSELF_PRIME_BUDGET  characters passed to `myself prime --budget` (default 4000)
#   MYSELF_FILE          brain file, passed through to myself
#   MYSELF_HOOKS_JQ      jq binary (default: jq); without it a sed fallback parses stdin

bin=${MYSELF_BIN:-myself}
jq=${MYSELF_HOOKS_JQ:-jq}
budget=${MYSELF_PRIME_BUDGET:-4000}
dir="${TMPDIR:-/tmp}/myself-hooks"

input=$(cat 2>/dev/null) || input=

if command -v "$jq" >/dev/null 2>&1; then
	sid=$(printf '%s' "$input" | "$jq" -r '.session_id // empty' 2>/dev/null)
	src=$(printf '%s' "$input" | "$jq" -r '.source // empty' 2>/dev/null)
else
	sid=$(printf '%s' "$input" | tr -d '\n' |
		sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
	src=$(printf '%s' "$input" | tr -d '\n' |
		sed -n 's/.*"source"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
fi

# Only plain ids become file names.
case $sid in
'' | *[!A-Za-z0-9._-]*) sid= ;;
esac

if [ -n "$sid" ] && mkdir -p "$dir" 2>/dev/null; then
	if [ "$src" = resume ]; then
		rm -f "$dir/$sid.nudged" 2>/dev/null
		date -u +%Y-%m-%dT%H:%M:%SZ >"$dir/$sid.start" 2>/dev/null
	elif [ ! -f "$dir/$sid.start" ]; then
		date -u +%Y-%m-%dT%H:%M:%SZ >"$dir/$sid.start" 2>/dev/null
	fi
fi

command -v "$bin" >/dev/null 2>&1 || exit 0
text=$("$bin" prime --budget "$budget" 2>/dev/null) || exit 0
[ -n "$text" ] || exit 0

printf 'What myself remembers (`myself prime`; query more with myself describe|search|path|log):\n%s\n' "$text"
exit 0
