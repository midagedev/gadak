#!/bin/sh
# A stand-in `claude` for FRAMING takes of the round-trip rig (GDK-1381).
#
# record-roundtrip.sh without --live puts this on the pane's PATH as `claude`,
# so the spec's boot/ask/answer gates pass in seconds instead of the ~1m30s a
# real boot takes, and a camera or layout change can be re-shot in ~3 minutes.
# Nothing it prints is a claim about the product and every frame says STUB in
# its first line — a take made with it is for the sheet, never for a viewer.
# The shipped cut is --live (GDK-1253): a real model reading really-broken
# code is the whole argument of the film.
printf '  STUB \342\200\224 framing take, not Claude Code   ? for shortcuts\n\n'
printf '> '
IFS= read -r prompt
printf '\n\342\217\272 %s\n\n' "$prompt"
i=0
while [ $i -lt 14 ]; do
  i=$((i + 1))
  printf '  \342\216\277 stub line %02d for: %s\n' "$i" "$(printf '%s' "$prompt" | cut -c1-48)"
  sleep 0.3
done
printf '\n  (stub) standing finished.\n'
# stay alive like a TUI would; the rig kills the serve at the end
while :; do sleep 60; done
