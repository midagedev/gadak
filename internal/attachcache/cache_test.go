package attachcache

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fetcher(body string, ct string) func() (io.ReadCloser, Meta, error) {
	return func() (io.ReadCloser, Meta, error) {
		return io.NopCloser(strings.NewReader(body)), Meta{ContentType: ct, Size: int64(len(body))}, nil
	}
}

func TestFillThenGet(t *testing.T) {
	c, err := New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Has("10001") {
		t.Fatal("empty cache reported a hit")
	}
	if err := c.Fill("10001", fetcher("PNGBYTES", "image/png")); err != nil {
		t.Fatal(err)
	}
	if !c.Has("10001") {
		t.Fatal("filled entry reported a miss")
	}
	f, meta, err := c.Get("10001")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, _ := io.ReadAll(f)
	if string(got) != "PNGBYTES" || meta.ContentType != "image/png" || meta.Size != 8 {
		t.Fatalf("got %q %+v", got, meta)
	}
}

// A miss on the same id from several renders at once must fetch once.
func TestFillIsSingleFlight(t *testing.T) {
	c, err := New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	calls := 0
	release := make(chan struct{})
	slow := func() (io.ReadCloser, Meta, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		<-release
		return io.NopCloser(strings.NewReader("x")), Meta{ContentType: "image/png", Size: 1}, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Fill("same", slow)
		}()
	}
	// Let the first goroutine register its flight before releasing.
	for {
		mu.Lock()
		started := calls
		mu.Unlock()
		if started > 0 {
			break
		}
	}
	close(release)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("fetch called %d times, want 1", calls)
	}
}

// A hostile id must not write outside the cache directory.
func TestIdCannotEscapeDirectory(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Fill("../../etc/passwd", fetcher("nope", "text/plain")); err != nil {
		t.Fatal(err)
	}
	var outside []string
	_ = filepath.WalkDir(filepath.Dir(dir), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !strings.HasPrefix(p, dir) && strings.Contains(p, "passwd") {
			outside = append(outside, p)
		}
		return nil
	})
	if len(outside) > 0 {
		t.Fatalf("wrote outside the cache: %v", outside)
	}
	if !c.Has("../../etc/passwd") {
		t.Fatal("entry not retrievable by its own id")
	}
}

func TestEvictsLeastRecentlyUsedOverBudget(t *testing.T) {
	// Budget fits two of the three 40-byte payloads.
	c, err := New(t.TempDir(), 100)
	if err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("a", 40)
	for _, id := range []string{"one", "two", "three"} {
		if err := c.Fill(id, fetcher(payload, "image/png")); err != nil {
			t.Fatal(err)
		}
	}
	files, bytes := c.Stats()
	if bytes > 100 {
		t.Fatalf("cache holds %d bytes over a 100-byte budget (%d files)", bytes, files)
	}
	if !c.Has("three") {
		t.Fatal("evicted the newest entry")
	}
}

func TestOversizedEntryIsRejectedNotTruncated(t *testing.T) {
	c, err := New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	huge := func() (io.ReadCloser, Meta, error) {
		// Declares a size past the per-file limit.
		return io.NopCloser(bytes.NewReader([]byte("x"))), Meta{ContentType: "video/mp4", Size: maxEntryBytes + 1}, nil
	}
	err = c.Fill("big", huge)
	if !TooLarge(err) {
		t.Fatalf("err = %v, want TooLarge", err)
	}
	if c.Has("big") {
		t.Fatal("oversized entry was cached")
	}
}

func TestKeySeparatesSiteAndIssue(t *testing.T) {
	c, err := New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	kA := Key("https://a.example", "work", "NMB-1", "100")
	kB := Key("https://b.example", "work", "NMB-1", "100")
	kIssue := Key("https://a.example", "work", "NMB-2", "100")
	if err := c.Fill(kA, fetcher("A", "image/png")); err != nil {
		t.Fatal(err)
	}
	if c.Has(kB) || c.Has(kIssue) || c.Has("100") {
		t.Fatal("scoped fill was visible under another key")
	}
	if !c.Has(kA) {
		t.Fatal("scoped fill missing under its own key")
	}
	if Key("", "work", "NMB-1", "100") != "100" {
		t.Fatal("empty site must keep the legacy id-only key (demo ImportFile)")
	}
}

func TestImportManifestUsesKeyAndDoesNotWriteRawID(t *testing.T) {
	c, err := New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte("PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	man := `{"attachments":[{"id":"100","file":"a.png","filename":"a.png","content_type":"image/png"}]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(man), 0o600); err != nil {
		t.Fatal(err)
	}
	site := "https://a.example"
	stats, err := c.ImportManifest(dir, site, "work", func(id string) (string, bool) {
		return "NMB-1", id == "100"
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Seeded != 1 || len(stats.SkippedIDs) != 0 {
		t.Fatalf("stats %+v", stats)
	}
	want := Key(site, "work", "NMB-1", "100")
	if !c.Has(want) {
		t.Fatal("missing scoped key")
	}
	if c.Has("100") {
		t.Fatal("also wrote the raw id")
	}
}

func TestFetchFailureLeavesNoEntry(t *testing.T) {
	c, err := New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("upstream down")
	if err := c.Fill("nope", func() (io.ReadCloser, Meta, error) { return nil, Meta{}, boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if c.Has("nope") {
		t.Fatal("failed fetch left a cached entry")
	}
	files, _ := c.Stats()
	if files != 0 {
		t.Fatalf("%d files left behind", files)
	}
}

// A waiter on a failed flight must receive the owner's typed cause — a
// fresh errors.New here sent every errors.Is branch (auth, too-large) in
// the server to default (GDK-1237). FAIL-first: red on the pre-fix Fill.
func TestWaiterSeesTheOwnersTypedError(t *testing.T) {
	c, err := New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("origin said no")
	release := make(chan struct{})
	failing := func() (io.ReadCloser, Meta, error) {
		<-release
		return nil, Meta{}, sentinel
	}

	ownerDone := make(chan error, 1)
	go func() { ownerDone <- c.Fill("same", failing) }()
	// Wait until the owner's flight is registered, so the second Fill is a
	// waiter, never a new owner.
	for {
		c.mu.Lock()
		_, busy := c.flight["same"]
		c.mu.Unlock()
		if busy {
			break
		}
		time.Sleep(time.Millisecond)
	}

	waiterDone := make(chan error, 1)
	go func() { waiterDone <- c.Fill("same", failing) }()
	// The waiter parks in wg.Wait; nothing observable marks that, so give
	// it a beat and release the owner either way — a waiter that arrived
	// late would become a new owner and hang on release, which the test
	// would catch as a second fetch blocking forever.
	time.Sleep(10 * time.Millisecond)
	close(release)

	if err := <-ownerDone; !errors.Is(err, sentinel) {
		t.Fatalf("owner error = %v, want the sentinel", err)
	}
	if err := <-waiterDone; !errors.Is(err, sentinel) {
		t.Fatalf("waiter error = %v, want errors.Is(sentinel) to hold through the wrap", err)
	}
}
