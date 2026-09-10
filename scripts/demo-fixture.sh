#!/usr/bin/env bash
# Single owner of the demo fixture build recipe (GDK-1751).
#
# examples/demo-source.db is the content original; examples/demo.db is always
# a regenerated artifact of it and is never read as build input. Reading the
# output back in is how regeneration stacked: the second pass off its own
# output grew changelog 1757 -> 1758 (a re-derived sprint-history row).
#
# To change fixture *content*, edit examples/demo-source.db and run
# `make demo-fixture`; commit both files. Editing examples/demo.db directly
# is lost on the next regen — it is an output, and `make demo-fixture-check`
# fails while the committed artifact is not what the source rebuilds.
#
# The build clock is pinned below, so one source + one code state rebuild
# byte-identically. Bump DEMO_NOW deliberately to freshen the demo's dates;
# recording demos re-stamp the sync badge at serve time (GADAK_FRESHEN), so
# this pin does not age what those recordings show.
#
# The scrub step is not optional and this script offers no flag to skip it:
# Datasette Lite reads the committed file raw (GDK-101), and a regen that
# skipped the scrub rebuilt items_fts with contentless_delete and went red on
# CI (2026-08-21).

set -euo pipefail

SOURCE="examples/demo-source.db"
OUTPUT="examples/demo.db"
DEMO_NOW="2026-09-10T00:00:00Z"

# build_to <dest> — snapshot from the source, scrub onto <dest>, print the stamp.
build_to() {
	local dest="$1"
	rm -f "$dest.new"
	go run ./cmd/gadak snapshot "$dest.new" --from "$SOURCE" \
		--spread 90d --seed 1 --derive-sprints --now "$DEMO_NOW"
	python3 scripts/scrub-demo-db.py "$dest.new" "$dest"
	rm -f "$dest.new"
	bash scripts/demo-schema.sh "$dest"
}

arg="${1:-build}"
arg="${arg#-}"   # tolerate -c/--check spellings
arg="${arg#-}"
case "$arg" in
build)
	[ -f "$SOURCE" ] || {
		echo "demo-fixture: $SOURCE missing — it is the content original and must be committed" >&2
		exit 1
	}
	build_to "$OUTPUT"
	;;
check)
	# GDK-1751 idempotency gate: two builds from the same source must agree
	# byte-for-byte, and the committed artifact must be what they build. A
	# hand-edited examples/demo.db or an edited source without a regen both
	# fail here.
	[ -f "$SOURCE" ] || {
		echo "demo-fixture: $SOURCE missing — it is the content original and must be committed" >&2
		exit 1
	}
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT
	build_to "$tmp/a.db" >"$tmp/a.log"
	build_to "$tmp/b.db" >"$tmp/b.log"
	sha_a="$(shasum -a 256 "$tmp/a.db" | cut -d' ' -f1)"
	sha_b="$(shasum -a 256 "$tmp/b.db" | cut -d' ' -f1)"
	if [ "$sha_a" != "$sha_b" ]; then
		echo "demo-fixture-check: two builds from $SOURCE differ" >&2
		echo "  a: $sha_a" >&2
		echo "  b: $sha_b" >&2
		cat "$tmp/a.log" "$tmp/b.log" >&2
		exit 1
	fi
	if [ ! -f "$OUTPUT" ]; then
		echo "demo-fixture-check: $OUTPUT is missing; run make demo-fixture" >&2
		exit 1
	fi
	sha_c="$(shasum -a 256 "$OUTPUT" | cut -d' ' -f1)"
	if [ "$sha_a" != "$sha_c" ]; then
		echo "demo-fixture-check: committed $OUTPUT is not what $SOURCE builds now" >&2
		echo "  committed: $sha_c" >&2
		echo "  rebuild:   $sha_a" >&2
		echo "  run make demo-fixture and commit both examples/demo-source.db and examples/demo.db" >&2
		exit 1
	fi
	echo "demo-fixture-check: OK — rebuild is byte-identical to the committed fixture ($sha_a)"
	;;
*)
	echo "usage: $0 [build|check]" >&2
	exit 2
	;;
esac
