# Optional CLI-only tape (status / sql / search). The README agent clip is
# now the Playwright split in e2e/demo/agent-demo.spec.ts (`make media-agent`):
# a paper terminal types `gadak views open` and the real list follows.
#
# Keep this tape for a SQL-only re-take: `make media-prep && vhs tools/tapes/agent.tape`.
# Deterministic: the output is gadak's own, from examples/demo.db. No live model.
#
# 77 cols × 24 rows, font 20, README renders this at 800 px. Paper/ink theme
# so the clip sits next to the web GIF instead of looking like a leftover
# dark dashboard.
#
# After vhs, `make media-agent` runs gifsicle -O3 --colors 64 when present.

Output docs/media/agent.gif

Set Shell "bash"
Set FontSize 20
Set FontFamily "D2Coding, Menlo, Monaco, monospace"
Set Width 1080
Set Height 500
Set Theme {"name":"gadak-paper","background":"#f4efe4","foreground":"#1c1812","cursor":"#2e4560","selection_background":"#d5dde8","black":"#1c1812","red":"#8f3530","green":"#2f5e42","yellow":"#8a6418","blue":"#2f5d8c","magenta":"#2e4560","cyan":"#2a4159","white":"#e4d9c4","brightBlack":"#635a4f","brightRed":"#8f3530","brightGreen":"#2f5e42","brightYellow":"#8a6418","brightBlue":"#2e4560","brightMagenta":"#2e4560","brightCyan":"#2a4159","brightWhite":"#f4efe4"}
Set Padding 14
Set Framerate 8
Set TypingSpeed 38ms
Set WindowBar Colorful
Set BorderRadius 6
Set WaitTimeout 15s

Hide
Type "source tools/tapes/.tmp/env.sh || exit 1"
Enter
Type "clear"
Enter
Show

Sleep 400ms

# Freshness, then the question JQL cannot ask, then the one-index search.
Type "gadak status"
Enter
Sleep 1600ms
Type "clear"
Enter
Sleep 250ms

Type "gadak sql 'select key, reopen_count, substr(summary,1,44) from issues_full where reopen_count > 0 order by reopen_count desc limit 6'"
Enter
Sleep 2000ms
Type "clear"
Enter
Sleep 250ms

Type "gadak search 'idempotency' --limit 4"
Enter
Sleep 2200ms
