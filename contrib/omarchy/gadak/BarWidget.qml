import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui

// Bar label: "<open>·<stuck>" from `gadak sql --json` of query.sql.
// Data enters only through that command; this file never opens the mirror.
// Click opens the gadak serve default bind (cmd/gadak/serve.go:41), not
// gadak:// — that scheme is registered by the macOS app bundle only
// (cmd/gadak/views.go deepLinkURL).
BarWidget {
  id: root
  moduleName: "io.github.midagedev.gadak"

  // serve.go default --addr. Named here so the port is not a magic number.
  readonly property string serveUrl: "http://127.0.0.1:7777"
  readonly property string queryPath: String(Qt.resolvedUrl("query.sql")).replace(/^file:\/\//, "")

  // loading | ok | no-gadak | not-synced | sql-err.
  // Not named `state`: QQuickItem already declares that property, and
  // redeclaring it is a QML compile error we have no local qmllint to catch.
  property string viewState: "loading"
  property int openCount: 0
  property int stuckCount: 0

  readonly property string displayText: {
    if (viewState === "ok") return openCount + "·" + stuckCount
    if (viewState === "no-gadak") return "no gadak"
    if (viewState === "not-synced") return "not synced"
    if (viewState === "sql-err") return "sql err"
    return "…"
  }

  readonly property string tooltipText: {
    if (viewState === "ok")
      return openCount + " open · " + stuckCount + " stuck >7d"
    if (viewState === "no-gadak")
      return "gadak is not on PATH. Install the Linux tarball from https://github.com/midagedev/gadak/releases/latest (gadak_<version>_linux_amd64.tar.gz or linux_arm64, plus checksums.txt)."
    if (viewState === "not-synced")
      return "no mirror — run gadak sync"
    if (viewState === "sql-err")
      return "gadak sql --json failed or was not NDJSON"
    return "reading gadak sql --json"
  }

  function refresh() {
    if (queryProc.running) return
    if (queryPath === "") {
      viewState = "sql-err"
      return
    }
    // One argv is the query file; stdout is NDJSON. stderr is ignored for parse
    // (a stale-mirror warning is expected there).
    queryProc.command = ["bash", "-c", "gadak sql --json \"$(cat \"$1\")\"", "gadak-omarchy", queryPath]
    queryProc.running = true
  }

  function openGadak() {
    if (root.bar && typeof root.bar.run === "function") {
      root.bar.run("omarchy-launch-webapp " + serveUrl)
      return
    }
    Quickshell.execDetached(["omarchy-launch-webapp", serveUrl])
  }

  function applyResult(exitCode, stdout, stderr) {
    var err = String(stderr || "")
    var out = String(stdout || "").trim()
    var code = Number(exitCode)

    // "no mirror" first: gadak ran, so it is not the missing-binary case.
    if (/no mirror/i.test(err)) {
      viewState = "not-synced"
      return
    }
    // 127 is the shell's "command not found". The message test names gadak
    // explicitly, because `cat` reports a missing query.sql with the same
    // "No such file or directory" and that is a broken install, not a
    // missing binary.
    if (code === 127 || /gadak: command not found/i.test(err)) {
      viewState = "no-gadak"
      return
    }
    if (code !== 0) {
      viewState = "sql-err"
      return
    }

    var line = out.split("\n")[0] || ""
    var obj
    try {
      obj = JSON.parse(line)
    } catch (e) {
      viewState = "sql-err"
      return
    }
    if (!obj || typeof obj !== "object" || obj.open === undefined || obj.stuck === undefined) {
      viewState = "sql-err"
      return
    }
    var o = Number(obj.open)
    var s = Number(obj.stuck)
    if (!isFinite(o) || !isFinite(s)) {
      viewState = "sql-err"
      return
    }
    openCount = o
    stuckCount = s
    viewState = "ok"
  }

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  Timer {
    interval: 60000
    repeat: true
    running: true
    triggeredOnStart: true
    onTriggered: root.refresh()
  }

  Process {
    id: queryProc
    running: false
    command: []
    stdout: StdioCollector { id: queryOut; waitForEnd: true }
    stderr: StdioCollector { id: queryErr; waitForEnd: true }
    onExited: function (exitCode) {
      var code = (exitCode === undefined || exitCode === null) ? queryProc.exitCode : exitCode
      root.applyResult(code, String(queryOut.text || ""), String(queryErr.text || ""))
    }
  }

  WidgetButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: root.displayText
    tooltipText: root.tooltipText
    onPressed: function (b) {
      if (b === Qt.RightButton) root.refresh()
      else root.openGadak()
    }
  }
}
