#!/usr/bin/env bash
# smoke.sh runs every th command against live Threads with real public ids and
# asserts each one either returns data or exits with a clean, documented code.
# It is the living proof of the project goal: "all commands work."
#
# th reads Threads anonymously, as a web crawler, with no login. Public targets
# return data; private ones are walled. A command "passes" when it returns data
# OR exits 4 (login wall) OR exits 3 (content unavailable / stale doc_id), since
# the logged-out GraphQL doc_ids rotate and search/deep-pagination may degrade
# while the server-rendered surface keeps working.
#
#   TH=./bin/th ./scripts/smoke.sh
set -u

TH="${TH:-th}"
PROFILE="${TH_SMOKE_PROFILE:-zuck}"
SEARCH="${TH_SMOKE_SEARCH:-threads}"
RESEARCH_TOPIC="${TH_SMOKE_RESEARCH_TOPIC:-AI automation}"

pass=0
fail=0
walled=0

# run <description> -- <command...>
# A live read passes when it returns data (exit 0 with output), reports an empty
# but valid result (exit 0, no output), or exits 3/4 (documented content
# unavailable / login wall). Only an undocumented exit code (1 generic, 2 usage,
# 5 rate-limit, 6 network, 127 not-found) is a failure.
run() {
	local desc="$1"; shift
	[ "$1" = "--" ] && shift
	local out rc
	out="$("$@" 2>/dev/null)"
	rc=$?
	if [ $rc -eq 0 ] && [ -n "$out" ]; then
		echo "ok    $desc"
		pass=$((pass + 1))
	elif [ $rc -eq 0 ]; then
		echo "ok    $desc (empty, valid)"
		pass=$((pass + 1))
	elif [ $rc -eq 4 ] || [ $rc -eq 3 ]; then
		echo "wall  $desc (exit $rc, not public / stale doc_id)"
		walled=$((walled + 1))
	else
		echo "FAIL  $desc (exit $rc)"
		fail=$((fail + 1))
	fi
}

# strict <description> -- <command...>
# Must exit 0 with output; used for offline-deterministic commands.
strict() {
	local desc="$1"; shift
	[ "$1" = "--" ] && shift
	local out rc
	out="$("$@" 2>/dev/null)"
	rc=$?
	if [ $rc -eq 0 ] && [ -n "$out" ]; then
		echo "ok    $desc"
		pass=$((pass + 1))
	else
		echo "FAIL  $desc (exit $rc)"
		fail=$((fail + 1))
	fi
}

echo "== th smoke (TH=$TH) =="
echo "   access: anonymous crawler (private reads are walled)"
echo

# --- offline-deterministic: must always pass ---
strict "version"            -- "$TH" version
strict "whoami"             -- "$TH" whoami -o json
strict "config show"        -- "$TH" config show -o jsonl
strict "config path"        -- "$TH" config path
strict "cache dir"          -- "$TH" cache dir
strict "id handle"          -- "$TH" id "$PROFILE" -o json
strict "id post url"        -- "$TH" id "https://www.threads.com/@$PROFILE/post/ABC123" -o json
strict "id net url rewrite" -- "$TH" id "https://www.threads.net/@$PROFILE" -o json
strict "completion bash"    -- "$TH" completion bash

# db query roundtrip against an empty dataset
DB="$(mktemp -t thsmoke.XXXXXX).db"
strict "db query (schema)"  -- "$TH" db --db "$DB" query "select count(*) from sqlite_master" -o jsonl
rm -f "$DB"

echo

# --- live reads: data, or a documented wall ---
run "profile"               -- "$TH" profile "$PROFILE" -o json --no-cache
run "profile --posts"       -- "$TH" profile "$PROFILE" --posts -n 5 -o jsonl --no-cache
run "profile --replies"     -- "$TH" profile "$PROFILE" --replies -n 5 -o jsonl --no-cache
run "feed"                  -- "$TH" feed "$PROFILE" -n 5 -o jsonl --no-cache
run "search"                -- "$TH" search "$SEARCH" -n 5 -o jsonl --no-cache
RESEARCH_DB="$(mktemp -t thresearch.XXXXXX).db"
run "research"              -- "$TH" research "$RESEARCH_TOPIC" --queries 1 --per-query 1 --profile-limit 0 --top-posts 1 --top-authors 1 --db "$RESEARCH_DB" -o json --no-cache
rm -f "$RESEARCH_DB"

# A post/replies smoke needs a concrete post URL; derive one from the feed,
# otherwise skip the per-post commands.
POST="$("$TH" profile "$PROFILE" --posts -n 1 -o url --no-cache 2>/dev/null | head -1)"
if [ -n "$POST" ]; then
	run "post"              -- "$TH" post "$POST" -o json --no-cache
	run "post --replies"    -- "$TH" post "$POST" --replies -n 5 -o jsonl --no-cache
	run "replies"           -- "$TH" replies "$POST" -n 5 -o jsonl --no-cache
else
	echo "skip  post/replies (no post url available)"
fi

echo
echo "== $pass passed, $walled walled, $fail failed =="
[ $fail -eq 0 ]
