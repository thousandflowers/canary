#!/usr/bin/env bash
# Self-check for the phrase system — no framework, just asserts.
#   bash test_canary_phrases.sh   # exits non-zero on any failure
#
# Covers the two things that are easy to break and invisible when broken:
# the bird speaking when it shouldn't, and a row that wraps or truncates ugly.

set -u
HERE=$(cd "$(dirname "$0")" && pwd)
SCRIPT="$HERE/canary-statusline.sh"
CORPUS="$HERE/phrases"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

fails=0
fail()          { echo "FAIL [$1]: $2"; fails=$((fails+1)); }
assert_has()    { case "$2" in *"$3"*) ;; *) fail "$1" "expected '$3' in: $(printf '%s' "$2" | tr '\n' '|')";; esac; }
assert_lacks()  { case "$2" in *"$3"*) fail "$1" "did NOT expect '$3' in: $(printf '%s' "$2" | tr '\n' '|')";; esac; }

# a transcript with 40 human turns, enough to move the score
T="$TMP/t"; : > "$T"
i=0; while [ "$i" -lt 40 ]; do echo '{"type":"user","message":{"role":"user","content":"hi"}}' >> "$T"; i=$((i+1)); done
ccjson() { printf '{"transcript_path":"%s","cost":{"total_duration_ms":%s}}' "$T" "$1"; }

# run the real path with every piece of persisted state redirected into $TMP
run() { # $1=duration_ms, rest=env
  local ms=$1; shift
  ccjson "$ms" | env COLUMNS="${COLUMNS:-100}" CANARY_NIGHT_MULT=100 \
    CANARY_HISTORY_FILE="$TMP/h" CANARY_PHRASE_STATE="$TMP/ps" \
    CANARY_RECENT_FILE="$TMP/rec" CANARY_PHRASE_DIR="$CORPUS" "$@" bash "$SCRIPT"
}
pv() { env COLUMNS="${COLUMNS:-100}" CANARY_PHRASE_DIR="$CORPUS" "$@" bash "$SCRIPT" preview; }
seed() { # $1=state $2=score $3=phrase-age-seconds $4=phrase
  local now; now=$(date +%s)
  printf 'state=%s\nscore=%s\nts=%s\nph_ts=%s\nph=%s\n' \
    "$1" "$2" "$now" "$(( now - $3 ))" "$4" > "$TMP/ps"
}

# 1) every band renders, and the slot right of the bird is never empty
for st in fresh chirpy tired worn; do
  out=$(env COLUMNS=100 CANARY_PHRASE_DIR="$CORPUS" bash "$SCRIPT" preview --state "$st")
  assert_has "preview-$st" "$out" "$st"
  case "$out" in *'⌐ '*|*'♪'*) ;; *) fail "preview-$st" "no phrase and no note glyph";; esac
done

# 2) dead says its one line — and it is the one line in the corpus
out=$(env COLUMNS=100 CANARY_PHRASE_DIR="$CORPUS" bash "$SCRIPT" preview --state dead)
assert_has "dead-line" "$out" "the canary is quiet."
assert_has "dead-face" "$out" "▐ x ▌v"
n=$(grep -cvE '^[[:space:]]*(#|$)' "$CORPUS/en/states/dead.txt")
[ "$n" = 1 ] || fail "dead-corpus" "states/dead.txt must hold exactly one line, has $n"

# 3) dead, having said it, goes silent — no note glyph, ever
seed dead 95 300 "the canary is quiet."
out=$(run 36000000)
assert_lacks "dead-silent" "$out" "♪"
assert_lacks "dead-silent" "$out" "⌐"

# 4) state+note beats plain state
out=$(env COLUMNS=100 CANARY_PHRASE_DIR="$CORPUS" bash "$SCRIPT" preview --state worn --note falling)
case "$out" in
  *"one more of these and i'm out."*|*"i've stopped taking notes."*|*"this is the last thing i say."*) ;;
  *) fail "worn+falling" "expected a worn+falling line, got: $(printf '%s' "$out" | tail -1)" ;;
esac

# 5) no transition, no phrase — the live line holds ~60s, then the bird drops it
seed tired 46 5 "this is the part before the part."
out=$(run 3600000)
assert_has "mute-live" "$out" "⌐ this is the part before the part."
seed tired 46 61 "this is the part before the part."
out=$(run 3600000)
assert_lacks "mute-expired" "$out" "⌐"
assert_has  "mute-expired-note" "$out" "♪"

# 6) CANARY_QUIET silences the text, keeps the bird and the note
out=$(pv CANARY_QUIET=1)
assert_lacks "quiet" "$out" "⌐"
assert_has   "quiet-note" "$out" "♪"

# 7) CANARY_ASCII swaps both glyphs
out=$(env COLUMNS=100 CANARY_PHRASE_DIR="$CORPUS" CANARY_ASCII=1 bash "$SCRIPT" \
       preview --state fresh --phrase "some candidate line")
assert_has   "ascii-tail" "$out" "- some candidate line"
assert_lacks "ascii-tail" "$out" "⌐"
out=$(env COLUMNS=100 CANARY_PHRASE_DIR="$CORPUS" CANARY_ASCII=1 CANARY_QUIET=1 bash "$SCRIPT" preview --state fresh)
assert_lacks "ascii-note" "$out" "♪"

# 8) narrow terminal: truncate at a word boundary, never wrap, never stub
long="somewhere someone is grinding a knife on a stone for the ninth hour."
out=$(env COLUMNS=60 CANARY_PHRASE_DIR="$CORPUS" bash "$SCRIPT" preview --state fresh --phrase "$long")
row=$(printf '%s' "$out" | tail -1)
[ "${#row}" -le 58 ] || fail "cols60" "row is ${#row} cells, wraps at 60: $row"
assert_lacks "cols60-midword" "$out" "on a stone f"
case "$row" in *' ') fail "cols60-trail" "trailing space: '$row'";; esac
# too narrow for anything worth reading -> no phrase at all, just the note
out=$(env COLUMNS=20 CANARY_PHRASE_DIR="$CORPUS" bash "$SCRIPT" preview --state fresh --phrase "$long")
assert_lacks "cols20" "$out" "⌐"
assert_has   "cols20-note" "$out" "♪"

# 9) the corpus itself, against the VOICE.md rules `canary lint` will own in v0.8
while IFS= read -r f; do
  while IFS= read -r line; do
    case "$line" in
      *'!'*)     fail "lint" "$f: exclamation mark: $line" ;;
      *' -- @'*) fail "lint" "$f: attribution left in a data line" ;;
      [[:upper:]]*) fail "lint" "$f: leading capital: $line" ;;
    esac
    case " $line " in
      *' you should '*|*' take a break'*|*' remember to '*|*' you must '*)
        fail "lint" "$f: imperative: $line" ;;
    esac
  done < <(grep -vE '^[[:space:]]*(#|$)' "$f")
  case "$f" in
    */lore/*) grep -qE '(^| )you( |$)' "$f" && fail "lint" "$f: 'you' is banned in lore/" ;;
  esac
done < <(find "$CORPUS" -name '*.txt')

# 10) the recent queue caps at 10 and keeps the selector moving
: > "$TMP/rec"; : > "$TMP/ps"
i=0; while [ "$i" -lt 120 ]; do
  ms=180000; [ $(( i % 2 )) -eq 0 ] && ms=3600000
  run "$ms" > /dev/null; i=$((i+1))
done
[ "$(wc -l < "$TMP/rec")" -le 10 ] || fail "recent-cap" "recent queue grew past 10"
distinct=$(sort -u "$TMP/rec" | wc -l | tr -d ' ')
[ "$distinct" -ge 3 ] || fail "recent-variety" "only $distinct distinct lines in 10 draws — selector is stuck"

[ "$fails" -eq 0 ] && { echo "ok — all phrase checks passed"; exit 0; }
echo "$fails check(s) failed"; exit 1
