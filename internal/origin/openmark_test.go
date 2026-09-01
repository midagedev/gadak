package origin

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// deadPID is past the kernel's maximum, so nothing can be running as it.
const deadPID = 4194305

func markPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "persist.db")
}

func TestOpenHolderIgnoresOwnMark(t *testing.T) {
	persist := markPath(t)
	markOpen(persist)
	if _, err := os.Stat(persist + ".open"); err != nil {
		t.Fatalf("markOpen wrote nothing: %v", err)
	}
	// The caller asking "must I stand aside?" is never its own answer.
	if pid := OpenHolder(persist); pid != 0 {
		t.Fatalf("OpenHolder = %d, want 0 for this process's own mark", pid)
	}
}

func TestOpenHolderReportsForeignLivePID(t *testing.T) {
	persist := markPath(t)
	// PID 1 always exists; on a normal machine it is root-owned, so this
	// also exercises the liveness check's EPERM path.
	writeMark(t, persist, 1)
	if pid := OpenHolder(persist); pid != 1 {
		t.Fatalf("OpenHolder = %d, want 1", pid)
	}
}

func TestOpenHolderDropsDeadMark(t *testing.T) {
	persist := markPath(t)
	writeMark(t, persist, deadPID)
	if pid := OpenHolder(persist); pid != 0 {
		t.Fatalf("OpenHolder = %d, want 0 for a dead PID", pid)
	}
	// Left in place, one crash would refuse conversion forever.
	if _, err := os.Stat(persist + ".open"); !os.IsNotExist(err) {
		t.Fatal("a dead mark must be removed on sight")
	}
}

func TestOpenHolderIgnoresGarbage(t *testing.T) {
	persist := markPath(t)
	if err := os.WriteFile(persist+".open", []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if pid := OpenHolder(persist); pid != 0 {
		t.Fatalf("OpenHolder = %d, want 0 for an unreadable mark", pid)
	}
}

func TestClearOpenLeavesAnotherProcessMark(t *testing.T) {
	persist := markPath(t)
	writeMark(t, persist, 1)
	clearOpen(persist)
	// Two holders of one persist is supported under WAL, so the first one
	// out must not erase the second one's mark.
	if _, err := os.Stat(persist + ".open"); err != nil {
		t.Fatalf("clearOpen removed another process's mark: %v", err)
	}

	markOpen(persist)
	clearOpen(persist)
	if _, err := os.Stat(persist + ".open"); !os.IsNotExist(err) {
		t.Fatal("clearOpen must remove this process's own mark")
	}
}

// A home that cannot be written is a supported state (GDK-149/GDK-173):
// the marker is a diagnostic convenience, never a precondition.
func TestMarkOpenNeverPanicsOnUnwritableDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("cannot make the dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	persist := filepath.Join(dir, "persist.db")
	markOpen(persist)
	if pid := OpenHolder(persist); pid != 0 {
		t.Fatalf("OpenHolder = %d, want 0 when no mark could be written", pid)
	}
}

// Every path that ends a session must drop the marker, and Close is the one
// the CLI actually takes — `cmd/gadak` main defers it, not CloseLocalOrigin.
// Hooking only the latter left a dead PID's marker behind after every
// command, which a real run found and none of the unit tests did, because
// they all called CloseLocalOrigin directly.
//
// FAIL-first: without clearOpen in Close, the marker survives here.
func TestCloseClearsTheMarker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	cfg := seedLocalOrigin(t, "")

	if _, err := Client(cfg); err != nil {
		t.Fatalf("open: %v", err)
	}
	persist := PersistPath(home)
	if _, err := os.Stat(persist + ".open"); err != nil {
		t.Fatalf("no marker while the workspace is open: %v", err)
	}

	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(persist + ".open"); !os.IsNotExist(err) {
		t.Fatal("Close left the marker behind — a dead PID in every workspace")
	}
}

// The desktop app never calls Client — it opens the workspace through
// LocalOriginHandler (apprun.StartOriginPassthrough, right after
// application.New). Since the app is the holder this whole mechanism exists
// for, that entry point has to reach the same hook, and this pins the
// convergence rather than leaving it to whoever next edits either path.
func TestLocalOriginHandlerMarksOpenToo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	cfg := seedLocalOrigin(t, "")

	if _, err := LocalOriginHandler(cfg); err != nil {
		t.Fatalf("LocalOriginHandler: %v", err)
	}
	if _, err := os.Stat(PersistPath(home) + ".open"); err != nil {
		t.Fatalf("the app's entry point left no marker: %v", err)
	}
}

func writeMark(t *testing.T, persist string, pid int) {
	t.Helper()
	body := `{"pid":` + strconv.Itoa(pid) + `,"startedAt":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(persist+".open", []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
