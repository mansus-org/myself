#!/bin/sh
# Claude Code UserPromptSubmit hook: brings the part of the brain that is
# relevant to the prompt into the context, instead of waiting for the agent
# to look it up.
#
# Reads the hook JSON on stdin and takes the `prompt`. Key words are the
# prompt lowercased and split on anything but a-z and 0-9, minus stop words
# and words shorter than 3 characters, deduplicated, first MYSELF_RECALL_WORDS
# (default 12) kept. Each key word is searched with `myself search --json`
# (one call per word, since search is a substring match). Entities are the
# subjects of the matching records, and their objects when the object is a
# single token (no spaces), since that is what an entity name looks like.
# They are ranked by how many distinct key words hit them, then by number of
# hits. The top MYSELF_RECALL_ENTITIES (default 5) are printed with
# `myself describe` (depth 1 when it fits, else depth 0), skipping entities
# already shown, until MYSELF_RECALL_BUDGET characters (default 3000) are used.
#
# Prints nothing when nothing matches. Never fails or blocks the prompt: any
# error prints nothing and exits 0. Words are ASCII only; accented letters
# split a word.
#
# Environment:
#   MYSELF_BIN           myself binary (default: myself on PATH)
#   MYSELF_RECALL        off disables the hook
#   MYSELF_RECALL_BUDGET characters of recalled text (default 3000)
#   MYSELF_RECALL_WORDS  most key words searched (default 12)
#   MYSELF_RECALL_ENTITIES  most entities described (default 5)
#   MYSELF_FILE          brain file, passed through to myself
#   MYSELF_URL, MYSELF_TOKEN  run against a myself server (remote mode). An
#                        unreachable server prints nothing after the first failed search
#   MYSELF_HOOKS_JQ      jq binary (default: jq); without it a sed fallback parses stdin

[ "$MYSELF_RECALL" = off ] && exit 0

bin=${MYSELF_BIN:-myself}
jq=${MYSELF_HOOKS_JQ:-jq}
budget=${MYSELF_RECALL_BUDGET:-3000}
maxwords=${MYSELF_RECALL_WORDS:-12}
maxents=${MYSELF_RECALL_ENTITIES:-5}
case $budget$maxwords$maxents in
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

# "word<TAB>entity" for every entity hit by every word.
hits=$(
	for w in $words; do
		res=$("$bin" search --json "$w" 2>/dev/null) || exit 1
		if [ -n "$jqok" ]; then
			printf '%s' "$res" | "$jq" -r '.matches[]? | .subject, (.object | select(type == "string" and test("^[^ \t]+$")))' 2>/dev/null
		else
			printf '%s' "$res" | tr '}' '\n' |
				sed -n 's/.*"subject":"\([^"]*\)".*/\1/p; s/.*"object":"\([^" ]*\)".*/\1/p'
		fi | sed "s/^/$w	/"
	done
) || exit 0
[ -n "$hits" ] || exit 0

ranked=$(printf '%s\n' "$hits" | awk -F'\t' '
	$2 != "" {
		hit[$2]++
		if (!(($1 SUBSEP $2) in seen)) { seen[$1, $2] = 1; words[$2]++ }
	}
	END { for (e in hit) printf "%d\t%d\t%s\n", words[e], hit[e], e }' |
	sort -t'	' -k1,1nr -k2,2nr -k3,3 | head -n "$maxents" | cut -f3)
[ -n "$ranked" ] || exit 0

out=
used=0
shown=
nl='
'
for e in $ranked; do
	case $e in -*) continue ;; esac
	case "$shown" in *"$nl$e$nl"*) continue ;; esac
	left=$((budget - used))
	[ "$left" -gt 0 ] || break
	text=$("$bin" describe --depth 1 "$e" 2>/dev/null) || continue
	if [ "${#text}" -gt "$left" ]; then
		text=$("$bin" describe "$e" 2>/dev/null) || continue
	fi
	[ -n "$text" ] || continue
	if [ "${#text}" -gt "$left" ]; then
		# Only the first entity may be cut; later ones that do not fit are skipped.
		[ -z "$out" ] || continue
		text=$(printf '%s' "$text" | head -c "$left")
	fi
	out="$out$text$nl"
	used=$((used + ${#text} + 1))
	shown="$shown$nl$e$nl"
done
[ -n "$out" ] || exit 0

printf 'What myself recalls for this prompt (%s; more with myself describe|search|path):\n%s' "$(echo $words)" "$out"
exit 0
