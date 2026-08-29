module github.com/midagedev/gadak/desktop

go 1.26.4

replace github.com/midagedev/gadak => ../

// wails v3.0.0-beta.12 plus wailsapp/wails#6006, nothing else. The tag is
// the upstream tag with those two commits cherry-picked
// (github.com/midagedev/wails, branch gadak/v3.0.0-beta.12); `git diff
// v3.0.0-beta.12..v3.0.0-beta.12-gadak.1` is 10 lines in one file.
//
// Why: webview_window_windows.go registers a "*" WebResourceRequested
// filter for asset serving, so on Windows EVERY request the WebView makes
// runs through edge.Chromium's handler — and that handler called
// log.Fatal(err) when args.GetRequest() failed. One transient COM failure
// on one request killed gadak, skipping deferred cleanup, and log.Fatal
// does not even reach the error callback wails' own SetErrorCallback
// configures. The PR drops the request and logs instead.
//
// The PR is open and unblocked (its only review comment was addressed and
// resolved the same day) — it is waiting on a maintainer, and gadak is not.
// DELETE THIS REPLACE when it merges into a beta we take: the fork branch
// exists only to carry it, and every wails bump has to redo it (fetch the
// new tag, cherry-pick, tag as -gadak.N) until then.
replace github.com/wailsapp/wails/v3 => github.com/midagedev/wails/v3 v3.0.0-beta.12-gadak.1

require (
	github.com/midagedev/gadak v0.0.0
	github.com/wailsapp/wails/v3 v3.0.0-beta.12
	golang.org/x/sys v0.47.0
)

require (
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
	github.com/midagedev/issuetap v0.0.0-20260828070557-ccb366aea250 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.56.0 // indirect
)
