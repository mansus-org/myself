#!/bin/sh
# Claude Code Stop hook: nudges the agent once per session to `myself remember`
# anything durable before it finishes.
#
# It blocks the stop once, with a reason Claude reads and acts on, when all of
# these hold:
#   - stop_hook_active is not true (Claude is not already continuing because
#     of a Stop hook);
#   - the session has not been nudged yet (marker
#     ${TMPDIR:-/tmp}/myself-hooks/<session_id>.nudged);
#   - session-start.sh recorded a start time for the session, and
#     `myself log --since <start> -n 1` prints nothing (no record written since);
#   - the session did something: its transcript has at least
#     MYSELF_NUDGE_MIN_LINES lines (default 10), or cwd is a git work tree with
#     uncommitted changes.
# Otherwise, and on any error (myself missing, bad input), it exits 0 silently.
# The marker is written before the nudge is printed, so it can never loop.
#
# Environment:
#   MYSELF_BIN              myself binary (default: myself on PATH)
#   MYSELF_NUDGE_MIN_LINES  transcript lines that count as activity (default 10)
#   MYSELF_FILE             brain file, passed through to myself
#   MYSELF_HOOKS_JQ         jq binary (default: jq); without it a sed fallback parses stdin

bin=${MYSELF_BIN:-myself}
jq=${MYSELF_HOOKS_JQ:-jq}
min_lines=${MYSELF_NUDGE_MIN_LINES:-10}
dir="${TMPDIR:-/tmp}/myself-hooks"

input=$(cat 2>/dev/null) || input=

# field NAME: a top-level string field of the hook JSON.
field() {
	if command -v "$jq" >/dev/null 2>&1; then
		printf '%s' "$input" | "$jq" -r --arg k "$1" '.[$k] // empty | tostring' 2>/dev/null
	else
		printf '%s' "$input" | tr -d '\n' |
			sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p"
	fi
}

if command -v "$jq" >/dev/null 2>&1; then
	active=$(printf '%s' "$input" | "$jq" -r '.stop_hook_active // false' 2>/dev/null)
elif printf '%s' "$input" | tr -d '\n' | grep -q '"stop_hook_active"[[:space:]]*:[[:space:]]*true'; then
	active=true
else
	active=false
fi
[ "$active" = true ] && exit 0

sid=$(field session_id)
case $sid in
'' | *[!A-Za-z0-9._-]*) exit 0 ;;
esac
[ -f "$dir/$sid.nudged" ] && exit 0
start=$(cat "$dir/$sid.start" 2>/dev/null) || exit 0
[ -n "$start" ] || exit 0

command -v "$bin" >/dev/null 2>&1 || exit 0
written=$("$bin" log --since "$start" -n 1 2>/dev/null) || exit 0
[ -z "$written" ] || exit 0

did_something=
transcript=$(field transcript_path)
case $transcript in
"~/"*) transcript="$HOME/${transcript#"~/"}" ;;
esac
if [ -n "$transcript" ] && [ -f "$transcript" ]; then
	lines=$(wc -l <"$transcript" 2>/dev/null | tr -d ' ')
	[ "${lines:-0}" -ge "$min_lines" ] 2>/dev/null && did_something=1
fi
if [ -z "$did_something" ]; then
	cwd=$(field cwd)
	if [ -n "$cwd" ] && [ -n "$(git -C "$cwd" status --porcelain 2>/dev/null | head -n 1)" ]; then
		did_something=1
	fi
fi
[ -n "$did_something" ] || exit 0

: >"$dir/$sid.nudged" 2>/dev/null || exit 0
printf '%s\n' '{"decision":"block","reason":"Before finishing: anything durable worth `myself remember`-ing from this session? If not, just stop."}'
exit 0
