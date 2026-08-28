package applog

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/midagedev/gadak/internal/pairing"
	"github.com/midagedev/gadak/internal/secretscan"
)

func installForTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prevOut, prevFlags := log.Writer(), log.Flags()
	closer, err := Install(dir)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if closer == nil {
		t.Fatal("Install returned a nil closer")
	}
	log.SetFlags(0)
	t.Cleanup(func() {
		closer()
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return dir
}

// closeForTest ends the writer the way a process exit does: the file keeps its
// bytes and the ring goes empty. That is the state every later process — a
// `gadak doctor` run, say — actually finds. The cleanup installForTest
// registered still runs; close is a no-op the second time.
func closeForTest(t *testing.T) {
	t.Helper()
	stateMu.Lock()
	st := active
	stateMu.Unlock()
	if st == nil {
		t.Fatal("no active applog state to close")
	}
	st.closer()
}

func logFile(dir string) string {
	return filepath.Join(dir, "logs", "gadak.log")
}

func TestInstallWritesFileAndStderr(t *testing.T) {
	dir := t.TempDir()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// The write end is closed in-line so ReadAll sees EOF; the read end
	// outlives that and needs its own close (GDK-990).
	t.Cleanup(func() { _ = r.Close() })
	old := os.Stderr
	os.Stderr = w
	prevOut, prevFlags := log.Writer(), log.Flags()
	closer, err := Install(dir)
	if err != nil {
		os.Stderr = old
		t.Fatalf("Install: %v", err)
	}
	log.SetFlags(0)
	log.Println("hello from gadak")
	closer()
	os.Stderr = old
	log.SetOutput(prevOut)
	log.SetFlags(prevFlags)
	_ = w.Close()
	stderr, _ := io.ReadAll(r)

	if !strings.Contains(string(stderr), "hello from gadak") {
		t.Fatalf("stderr missing line, got %q", stderr)
	}
	body, err := os.ReadFile(logFile(dir))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(body), "hello from gadak") {
		t.Fatalf("log file missing line, got %q", body)
	}
	if Path(dir) != logFile(dir) {
		t.Fatalf("Path(dir) = %q, want %q", Path(dir), logFile(dir))
	}
}

func TestInstallIdempotent(t *testing.T) {
	dir := t.TempDir()
	prevOut, prevFlags := log.Writer(), log.Flags()
	c1, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c1()
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	log.SetFlags(0)
	log.Println("first")
	c2, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}
	log.Println("second")
	body, err := os.ReadFile(logFile(dir))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Fatalf("idempotent Install lost lines: %q", got)
	}
	// Same closer identity: calling c2 must not restore output while c1 is live.
	if fmt.Sprintf("%p", c1) != fmt.Sprintf("%p", c2) {
		t.Fatalf("second Install returned a different closer")
	}
}

func discardStderr(t *testing.T) {
	t.Helper()
	old := os.Stderr
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = f
	t.Cleanup(func() {
		os.Stderr = old
		_ = f.Close()
	})
}

func TestRotation(t *testing.T) {
	discardStderr(t)
	dir := installForTest(t)
	line := strings.Repeat("a", 1024)
	// 6 MiB of 1 KiB lines exceeds the 5 MiB cap.
	for i := 0; i < 6*1024; i++ {
		log.Print(line)
	}
	rotated := logFile(dir) + ".1"
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("expected rotated file %s: %v", rotated, err)
	}
	fi, err := os.Stat(logFile(dir))
	if err != nil {
		t.Fatalf("current log: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("current log is empty after rotation")
	}
	old, err := os.Stat(rotated)
	if err != nil {
		t.Fatal(err)
	}
	if old.Size() < 5<<20 {
		t.Fatalf("rotated size %d, want at least 5 MiB", old.Size())
	}
}

func TestScrubTokenAbsentFromFileAndStderr(t *testing.T) {
	dir := t.TempDir()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// The write end is closed in-line so ReadAll sees EOF; the read end
	// outlives that and needs its own close (GDK-990).
	t.Cleanup(func() { _ = r.Close() })
	old := os.Stderr
	os.Stderr = w
	prevOut, prevFlags := log.Writer(), log.Flags()
	closer, err := Install(dir)
	if err != nil {
		os.Stderr = old
		t.Fatal(err)
	}
	log.SetFlags(0)

	token := "ATATT" + strings.Repeat("A", 20)
	if secretscan.Match(token) != "atlassian_api_token" {
		t.Fatalf("fixture is not the secretscan atlassian shape")
	}
	log.Printf("credential=%s done", token)

	closer()
	os.Stderr = old
	log.SetOutput(prevOut)
	log.SetFlags(prevFlags)
	_ = w.Close()
	stderr, _ := io.ReadAll(r)
	body, err := os.ReadFile(logFile(dir))
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"file": body, "stderr": stderr} {
		if strings.Contains(string(b), token) {
			t.Errorf("%s still contains the raw token", name)
		}
		if !strings.Contains(string(b), "<redacted>") {
			t.Errorf("%s missing <redacted>, got %q", name, b)
		}
	}
}

func TestScrubBearerAndPairingOffer(t *testing.T) {
	dir := installForTest(t)
	bearer := "Bearer " + strings.Repeat("t", 20)
	offer, err := pairing.EncodeOffer(pairing.Offer{
		V:        pairing.OfferV1,
		Endpoint: "http://127.0.0.1:7899",
		Token:    "offer-vector-token-applog",
		Label:    "laptop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pairing.DecodeOffer(offer); err != nil {
		t.Fatalf("fixture offer must decode: %v", err)
	}
	log.Printf("auth %s offer %s", bearer, offer)

	body, err := os.ReadFile(logFile(dir))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if strings.Contains(got, strings.Repeat("t", 20)) {
		t.Fatalf("raw bearer value present: %q", got)
	}
	if strings.Contains(got, offer) {
		t.Fatalf("raw pairing offer present: %q", got)
	}
	if strings.Contains(got, "offer-vector-token-applog") {
		t.Fatalf("pairing token inside offer leaked: %q", got)
	}
	if !strings.Contains(got, "<redacted>") {
		t.Fatalf("missing <redacted>: %q", got)
	}
}

func TestRecentRing(t *testing.T) {
	_ = installForTest(t)
	if got := Recent(10); len(got) != 0 {
		t.Fatalf("Recent before writes = %v, want empty", got)
	}
	for i := 0; i < 3; i++ {
		log.Printf("ring-line-%d", i)
	}
	got := Recent(2)
	if len(got) != 2 {
		t.Fatalf("Recent(2) = %#v, want 2 lines", got)
	}
	if !strings.Contains(got[0], "ring-line-1") || !strings.Contains(got[1], "ring-line-2") {
		t.Fatalf("Recent(2) = %#v, want last two lines", got)
	}
	all := Recent(500)
	if len(all) != 3 {
		t.Fatalf("Recent(500) = %#v, want 3", all)
	}
	if len(Recent(0)) != 0 {
		t.Fatalf("Recent(0) should be empty")
	}
}

func TestRecentCap500(t *testing.T) {
	_ = installForTest(t)
	for i := 0; i < 520; i++ {
		log.Printf("cap-%d", i)
	}
	got := Recent(500)
	if len(got) != 500 {
		t.Fatalf("Recent(500) len=%d, want 500", len(got))
	}
	if !strings.Contains(got[0], "cap-20") {
		t.Fatalf("oldest kept = %q, want cap-20", got[0])
	}
	if !strings.Contains(got[len(got)-1], "cap-519") {
		t.Fatalf("newest = %q, want cap-519", got[len(got)-1])
	}
	if n := len(Recent(1000)); n != 500 {
		t.Fatalf("Recent(1000) len=%d, want cap 500", n)
	}
}

// Tail is what `gadak doctor` reads, and doctor is a different process from
// the one that wrote the log — so the whole point is that it works with an
// empty ring. Every case here closes the writer first to make that explicit.
func TestTailReadsFileNotRing(t *testing.T) {
	dir := installForTest(t)
	for i := 0; i < 5; i++ {
		log.Printf("tail-line-%d", i)
	}
	closeForTest(t)

	if got := Recent(10); len(got) != 0 {
		t.Fatalf("ring after close = %#v, want empty (this is doctor's situation)", got)
	}
	got := Tail(dir, 2)
	if len(got) != 2 {
		t.Fatalf("Tail(2) = %#v, want 2 lines", got)
	}
	if !strings.Contains(got[0], "tail-line-3") || !strings.Contains(got[1], "tail-line-4") {
		t.Fatalf("Tail(2) = %#v, want the last two lines oldest-first", got)
	}
	if n := len(Tail(dir, 100)); n != 5 {
		t.Fatalf("Tail(100) len=%d, want all 5", n)
	}
	if n := len(Tail(dir, 0)); n != 0 {
		t.Fatalf("Tail(0) len=%d, want 0", n)
	}
	if n := len(Tail(t.TempDir(), 10)); n != 0 {
		t.Fatalf("Tail of a home with no log file len=%d, want 0", n)
	}
}

// A log larger than the read window must not hand back the fragment the
// window cut in half — a half line reads as corruption in a bug report.
func TestTailDropsTruncatedFirstLine(t *testing.T) {
	dir := installForTest(t)
	filler := strings.Repeat("x", 512)
	for i := 0; i < 300; i++ { // ~150 KiB, comfortably past tailWindow
		log.Printf("bulk-%03d-%s", i, filler)
	}
	log.Print("bulk-last")
	closeForTest(t)

	got := Tail(dir, 2000)
	if len(got) == 0 {
		t.Fatal("Tail returned nothing")
	}
	if !strings.HasPrefix(got[0], "bulk-") {
		t.Fatalf("first line = %q, want a whole line, not the window's cut", got[0])
	}
	if last := got[len(got)-1]; last != "bulk-last" {
		t.Fatalf("last line = %q, want bulk-last", last)
	}
}

func TestTailScrubs(t *testing.T) {
	dir := installForTest(t)
	token := "ATATT" + strings.Repeat("a", 40)
	log.Printf("auth failed for %s", token)
	closeForTest(t)

	got := Tail(dir, 10)
	if len(got) != 1 {
		t.Fatalf("Tail = %#v, want 1 line", got)
	}
	if strings.Contains(got[0], token) {
		t.Fatal("Tail handed back the raw token")
	}
	if !strings.Contains(got[0], "<redacted>") {
		t.Fatalf("Tail = %q, want the token replaced", got[0])
	}
}

func TestInterleavedWritesNeverSplitLines(t *testing.T) {
	dir := installForTest(t)
	w := log.Writer()
	const n = 1000
	var wg sync.WaitGroup
	wg.Add(2)
	write := func(id int) {
		defer wg.Done()
		for i := 0; i < n; i++ {
			line := fmt.Sprintf("G%d-%05d-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n", id, i)
			if _, err := w.Write([]byte(line)); err != nil {
				t.Errorf("write: %v", err)
				return
			}
		}
	}
	go write(0)
	go write(1)
	wg.Wait()

	f, err := os.Open(logFile(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	re := regexp.MustCompile(`^G[01]-\d{5}-x+$`)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines int
	for sc.Scan() {
		text := sc.Text()
		if text == "" {
			continue
		}
		if !re.MatchString(text) {
			t.Fatalf("split or corrupt line %d: %q", lines, text)
		}
		lines++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if lines != 2*n {
		t.Fatalf("got %d intact lines, want %d", lines, 2*n)
	}
}

func TestReadOnlyHomeDoesNotFailProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	prevOut, prevFlags := log.Writer(), log.Flags()
	closer, err := Install(dir)
	t.Cleanup(func() {
		if closer != nil {
			closer()
		}
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	if closer == nil {
		t.Fatal("Install returned a nil closer on read-only home")
	}
	// File open failed, but the process must still be able to log.
	log.SetFlags(0)
	log.Println("still-logging")
	if _, statErr := os.Stat(logFile(dir)); !os.IsNotExist(statErr) {
		t.Fatalf("read-only home created a log file: stat err=%v", statErr)
	}
	got := Recent(10)
	found := false
	for _, line := range got {
		if strings.Contains(line, "still-logging") {
			found = true
		}
	}
	if !found {
		t.Fatalf("ring missing stderr-only line, Recent=%v (install err=%v)", got, err)
	}
}

func TestLogsDirMode0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on Windows")
	}
	dir := installForTest(t)
	fi, err := os.Stat(filepath.Join(dir, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Fatalf("logs/ mode = %04o, want 0700", perm)
	}
	lf, err := os.Stat(logFile(dir))
	if err != nil {
		t.Fatal(err)
	}
	if perm := lf.Mode().Perm(); perm != 0o600 {
		t.Fatalf("gadak.log mode = %04o, want 0600", perm)
	}
}
