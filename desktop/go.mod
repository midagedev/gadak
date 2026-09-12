module github.com/midagedev/gadak/desktop

go 1.26.4

replace github.com/midagedev/gadak => ../

// Pure upstream pin since v3.0.0-beta.17: the fork's only cargo,
// wailsapp/wails#6006 (WebResourceRequested handler log.Fatal on a
// transient COM failure), merged upstream in the beta.13..beta.17 range,
// so the replace is gone and this file names an upstream tag only.

require (
	github.com/midagedev/gadak v0.0.0
	github.com/wailsapp/wails/v3 v3.0.0-beta.17
	golang.org/x/sys v0.47.0
	modernc.org/sqlite v1.56.0
)

require (
	git.sr.ht/~jackmordaunt/go-toast/v2 v2.0.3 // indirect
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/clipperhouse/stringish v0.1.1 // indirect
	github.com/clipperhouse/uax29/v2 v2.5.0 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/midagedev/issuetap v0.0.0-20260911063337-b2a2614dd570 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e // indirect
	github.com/yuin/goldmark v1.8.5 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
