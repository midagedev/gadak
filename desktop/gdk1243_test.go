package main

import (
	"bytes"
	"database/sql"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"

	_ "modernc.org/sqlite" // pure-Go driver: same one internal/store registers
)

// TestGDK1243MainRoutesBootErrorToBootFatal is the GDK-1243 wiring half: a
// boot failure returned from run() (mirror schema refusal, locked DB, any
// pre-window error) must reach bootFatal — the exit that shows a native
// dialog — not a bare log.Fatal, which a double-clicked app user never sees.
// AST pin for the same reason gdk658_test.go parses main.go: main() itself is
// not callable from a test.
//
// FAIL-first: against the pre-fix main (log.Fatal on run's error) the
// bootFatal lookup fails and log.Fatal is still present.
func TestGDK1243MainRoutesBootErrorToBootFatal(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var mainFn *ast.FuncDecl
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "main" || fn.Recv != nil {
			continue
		}
		mainFn = fn
		break
	}
	if mainFn == nil || mainFn.Body == nil {
		t.Fatal("func main() not found in main.go")
	}
	calls := topLevelCallNames(mainFn.Body)
	hasBootFatal, hasLogFatal := false, false
	for _, name := range calls {
		switch name {
		case "bootFatal":
			hasBootFatal = true
		case "log.Fatal":
			hasLogFatal = true
		}
	}
	if !hasBootFatal {
		t.Fatalf("GDK-1243: main() never calls bootFatal — a run() error still dies without a dialog. calls=%v", calls)
	}
	if hasLogFatal {
		t.Fatalf("GDK-1243: main() still calls log.Fatal directly — that path has no dialog. calls=%v", calls)
	}
}

// writeFutureMirror builds the GDK-1243 repro home: a scratch GADAK_HOME
// whose gadak.db carries a user_version no gadak build can read (999 > any
// len(migrations)). Same technique as internal/store's too-new test.
func writeFutureMirror(t *testing.T, home string) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 999"); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestGDK1243FutureMirrorReachesDialogSeam is the runtime half: the exact
// boot the v0.18.1 measurement saw die silently (scratch home, mirror from a
// newer gadak) must return run()'s error, and reporting it must hand the
// store's sentence to the dialog seam verbatim. Headless by construction —
// the seam is swapped, so no modal runs (the CI macos-14 runner has no user
// session to dismiss one). The real ~/.gadak is never touched: GADAK_HOME
// points at t.TempDir().
func TestGDK1243FutureMirrorReachesDialogSeam(t *testing.T) {
	home := t.TempDir()
	writeFutureMirror(t, home)
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	err := run()
	if err == nil {
		t.Fatal("run() with a future mirror returned nil — it would have opened a window; the refusal is gone")
	}
	var tooNew *store.SchemaTooNewError
	if !errors.As(err, &tooNew) {
		t.Fatalf("boot failure must be the typed store refusal, got %T: %v", err, err)
	}

	var gotTitle, gotText string
	shown := 0
	orig := showBootErrorDialog
	showBootErrorDialog = func(title, text string) {
		shown++
		gotTitle, gotText = title, text
	}
	defer func() { showBootErrorDialog = orig }()

	reportBootFailure(err)

	if shown != 1 {
		t.Fatalf("dialog seam called %d times, want 1", shown)
	}
	if gotTitle != "Gadak" {
		t.Fatalf("dialog title = %q, want Gadak", gotTitle)
	}
	if gotText != tooNew.Error() {
		t.Fatalf("dialog text must be the store's sentence verbatim — no translation, no summary:\n got: %s\nwant: %s", gotText, tooNew.Error())
	}
}

// TestGDK1243ReportKeepsStderrTrail pins the CLI-parity half: the dialog is
// additive, and the stderr/log line the old log.Fatal wrote still happens.
func TestGDK1243ReportKeepsStderrTrail(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	orig := showBootErrorDialog
	showBootErrorDialog = func(string, string) {}
	defer func() { showBootErrorDialog = orig }()

	reportBootFailure(errors.New("boot boom"))

	if !strings.Contains(buf.String(), "boot boom") {
		t.Fatalf("stderr/log trail lost; log wrote: %q", buf.String())
	}
}

// TestGDK1243BootDialogScriptEscaping: the dialog body is arbitrary error
// prose — quotes, backslashes (Windows paths in the store's recovery
// command), line breaks. The AppleScript literal must carry all of them.
func TestGDK1243BootDialogScriptEscaping(t *testing.T) {
	script := bootDialogScript("Gad\"ak", "mirror \"x\" at C:\\Users\\a\\b.db\nsecond line")
	want := "display dialog \"mirror \\\"x\\\" at C:\\\\Users\\\\a\\\\b.db\nsecond line\" with title \"Gad\\\"ak\" with icon stop"
	if script != want {
		t.Fatalf("script:\n got: %q\nwant: %q", script, want)
	}
}

// TestGDK1243BootDialogScriptCompiles is the artifact/code detector for the
// AppleScript string: bootDialogScript's output is consumed by an external
// interpreter, so a Go-level string assertion alone cannot prove it is valid
// AppleScript. osacompile compiles the production script — with a nasty body
// and with the real store refusal sentence — without running or showing
// anything. darwin-only; elsewhere the dialog is not the surface anyway.
func TestGDK1243BootDialogScriptCompiles(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("osacompile is darwin-only; other GOOS keep stderr as the boot-failure surface")
	}
	tooNew := &store.SchemaTooNewError{Path: "/tmp/h/gadak.db", Have: 999, Supported: 40}
	for _, text := range []string{
		tooNew.Error(),
		"quote \" backslash \\ and\nnewline",
	} {
		out := filepath.Join(t.TempDir(), "probe.scpt")
		cmd := exec.Command("osacompile", "-o", out, "-e", bootDialogScript("Gadak", text))
		if err := cmd.Run(); err != nil {
			t.Fatalf("boot dialog script does not compile as AppleScript (text=%q): %v", text, err)
		}
	}
}
