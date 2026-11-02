#!/bin/sh
# Claude Code UserPromptSubmit hook: brings the part of the brain that is
# relevant to the prompt into the context, instead of waiting for the agent
# to look it up.
#
# Reads the hook JSON on stdin and takes the `prompt`. Key words are the
# prompt lowercased and split on anything but a-z and 0-9, minus stop words
# and words shorter than 3 characters, deduplicated, first MYSELF_RECALL_WORDS
# (default 40) kept. Each key word is searched with `myself search --json`
# (one call per word, since search is a substring match); a word with no hits
# is retried without a trailing "s". Only subjects (entities with their own
# records) are ranked, never bare objects such as type nodes. A word that hits
# more than MYSELF_RECALL_MAX_HITS (default 8) entities is too common and is
# dropped; every other word gives each entity it hits 1/(entities it hit), so
# specific words count more, and an entity's score is the sum. The top
# MYSELF_RECALL_ENTITIES (default 8) are printed once each with
# `myself describe` (no neighbours), in rank order, each cut to whole lines
# within a third of the budget, and the whole output,
# header included, never exceeds MYSELF_RECALL_BUDGET characters (default 3000).
#
# Prints nothing when nothing matches. Never fails or blocks the prompt: any
# error prints nothing and exits 0. Words are ASCII only; accented letters
# split a word.
#
# Environment:
#   MYSELF_BIN           myself binary (default: myself on PATH)
#   MYSELF_RECALL        off disables the hook
#   MYSELF_RECALL_BUDGET characters of recalled text (default 3000)
#   MYSELF_RECALL_WORDS  most key words searched (default 40)
#   MYSELF_RECALL_MAX_HITS  entities a word may hit before it is dropped (default 8)
#   MYSELF_RECALL_ENTITIES  most entities described (default 8)
#   MYSELF_FILE          brain file, passed through to myself
#   MYSELF_URL, MYSELF_TOKEN  run against a myself server (remote mode). An
#                        unreachable server prints nothing after the first failed search
#   MYSELF_HOOKS_JQ      jq binary (default: jq); without it a sed fallback parses stdin

[ "$MYSELF_RECALL" = off ] && exit 0

bin=${MYSELF_BIN:-myself}
jq=${MYSELF_HOOKS_JQ:-jq}
budget=${MYSELF_RECALL_BUDGET:-3000}
maxwords=${MYSELF_RECALL_WORDS:-40}
maxents=${MYSELF_RECALL_ENTITIES:-8}
maxhits=${MYSELF_RECALL_MAX_HITS:-8}
case $budget$maxwords$maxents$maxhits in
'' | *[!0-9]*) exit 0 ;;
esac

command -v "$bin" >/dev/null 2>&1 || exit 0

input=$(cat 2>/dev/null) || input=

if command -v "$jq" >/dev/null 2>&1; then
	prompt=$(printf '%s' "$input" | "$jq" -r '.prompt // empty' 2>/dev/null)
	jqok=1
else
	prompt=$(printf '%s' "$input" | tr -d '\n' |
		sed -n 's/.*"prompt"[[:space:]]*:[[:space:]]*"\(\([^"\\]\|\\.\)*\)".*/\1/p' |
		sed 's/\\[nrt]/ /g')
	jqok=
fi
[ -n "$prompt" ] || exit 0

stop=" the and for are but not you your with that this from have has had was were will would can could should about into over under what when where which who how why its it's our their them they then than there here some any all out off just also very more most such only own same too use used using please make let get got let's don't want need like one two "
words=
n=0
for w in $(printf '%s' "$prompt" | tr 'A-Z' 'a-z' | tr -c 'a-z0-9' '\n'); do
	[ "${#w}" -ge 3 ] || continue
	case "$stop" in *" $w "*) continue ;; esac
	case " $words " in *" $w "*) continue ;; esac
	words="$words $w"
	n=$((n + 1))
	[ "$n" -lt "$maxwords" ] || break
done
[ -n "$words" ] || exit 0

# Subjects of the records matching a word, one per line.
subjects() {
	res=$("$bin" search --json "$1" 2>/dev/null) || return 1
	if [ -n "$jqok" ]; then
		printf '%s' "$res" | "$jq" -r '.matches[]?.subject' 2>/dev/null
	else
		printf '%s' "$res" | tr '}' '\n' | sed -n 's/.*"subject":"\([^"]*\)".*/\1/p'
	fi
}

# "word<TAB>subject" for every entity with records that a word hits. A word with
# no hits is tried again without a trailing s.
hits=$(
	for w in $words; do
		subs=$(subjects "$w") || exit 1
		if [ -z "$subs" ]; then
			case $w in
			*[!s]s) subs=$(subjects "${w%s}") || exit 1 ;;
			esac
		fi
		printf '%s\n' "$subs" | sort -u | sed -n "/./s/^/$w	/p"
	done
) || exit 0
[ -n "$hits" ] || exit 0

# A word that hits more than maxhits entities is too common to say anything and
# is dropped. Every other word gives each entity it hits 1/(entities it hit), so
# specific words count more.
ranked=$(printf '%s\n' "$hits" | awk -F'\t' -v max="$maxhits" '
	{ n[$1]++; w[NR] = $1; e[NR] = $2 }
	END {
		for (i = 1; i <= NR; i++)
			if (n[w[i]] <= max) score[e[i]] += 1 / n[w[i]]
		for (x in score) printf "%.6f\t%s\n", score[x], x
	}' |
	sort -t'	' -k1,1nr -k2,2 | head -n "$maxents" | cut -f2)
[ -n "$ranked" ] || exit 0

header='What myself recalls for this prompt (myself describe|search|path for more):'
out=
used=$((${#header} + 1))
nl='
'
# One entity may take at most a third of the budget, so a big one cannot crowd out
# the rest. A longer description loses whole lines from its end.
cap=$((budget / 3))
for e in $ranked; do
	case $e in -*) continue ;; esac
	room=$((budget - used - 1))
	[ "$room" -gt "$cap" ] && room=$cap
	[ "$room" -gt 0 ] || break
	text=$("$bin" describe "$e" 2>/dev/null) || continue
	[ -n "$text" ] || continue
	if [ "${#text}" -gt "$room" ]; then
		text=$(printf '%s' "$text" | head -c "$room" | sed '$d')
		[ -n "$text" ] || continue
	fi
	out="$out$text$nl"
	used=$((used + ${#text} + 1))
done
[ -n "$out" ] || exit 0

printf '%s\n%s' "$header" "$out"
exit 0
