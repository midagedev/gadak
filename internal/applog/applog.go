// Package applog is the single owner of where process log output goes.
//
// The log file is a disposable diagnostic, never origin data: it is not
// exported, not synced, and not the record of anything. Call sites keep
// using the standard log package; this package only sets the destination
// and scrubs credential-shaped bytes on the way in.
package applog

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/midagedev/gadak/internal/fsperm"
)

const (
	relLog     = "logs"
	fileName   = "gadak.log"
	maxSize    = 5 << 20 // 5 MiB; one rotated sibling
	ringCap    = 500
	fileFlags  = os.O_APPEND | os.O_CREATE | os.O_WRONLY
	fileMode   = 0o600
	maxPartial = 64 << 10
	tailWindow = 64 << 10
)

var (
	stateMu sync.Mutex
	active  *state
	lines   memRing
)

type state struct {
	file    *os.File
	path    string
	closer  func()
	sink    *sink
	partial []byte
}

type sink struct {
	st *state
}

// Path is <dir>/logs/gadak.log — the file Install opens.
func Path(dir string) string {
	return filepath.Join(dir, relLog, fileName)
}

// Install opens <dir>/logs/gadak.log and points the standard log package at a
// writer that scrubs credential-shaped bytes, appends them to that file, and
// mirrors them to stderr. A second call in one process is a no-op that
// returns the same closer.
//
// A file that cannot be opened (a read-only home is a supported state) does
// not fail the process: Install logs one line to stderr and continues with
// stderr-only output plus the in-memory ring. The returned error names the
// open failure; callers at process start must ignore it.
func Install(dir string) (func(), error) {
	stateMu.Lock()
	defer stateMu.Unlock()
	if active != nil {
		return active.closer, nil
	}

	st := &state{path: Path(dir)}
	st.sink = &sink{st: st}
	st.closer = st.close

	var openErr error
	logsDir := filepath.Join(dir, relLog)
	if err := fsperm.EnsurePrivateDir(logsDir); err != nil && !errors.Is(err, fsperm.ErrChmod) {
		openErr = err
	} else {
		if errors.Is(err, fsperm.ErrChmod) {
			fmt.Fprintf(os.Stderr, "gadak: logs: %v\n", err)
		}
		f, err := os.OpenFile(st.path, fileFlags, fileMode)
		if err != nil {
			openErr = err
		} else {
			st.file = f
		}
	}
	if openErr != nil {
		fmt.Fprintf(os.Stderr, "gadak: logs: %v\n", openErr)
	}

	active = st
	log.SetOutput(st.sink)
	return st.closer, openErr
}

func (st *state) close() {
	stateMu.Lock()
	defer stateMu.Unlock()
	if active != st {
		return
	}
	if st.file != nil {
		_ = st.file.Close()
		st.file = nil
	}
	log.SetOutput(os.Stderr)
	active = nil
	lines.reset()
}

func (s *sink) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	cleaned := scrub(p)
	stateMu.Lock()
	s.st.feedRingLocked(cleaned)
	if s.st.file != nil {
		_, _ = s.st.file.Write(cleaned)
		s.st.rotateIfNeededLocked()
	}
	stateMu.Unlock()
	if _, err := os.Stderr.Write(cleaned); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (st *state) rotateIfNeededLocked() {
	if st.file == nil {
		return
	}
	fi, err := st.file.Stat()
	if err != nil {
		return
	}
	if fi.Size() <= maxSize {
		return
	}
	_ = st.file.Close()
	st.file = nil
	rotated := st.path + ".1"
	_ = os.Remove(rotated)
	if err := os.Rename(st.path, rotated); err != nil {
		f, openErr := os.OpenFile(st.path, fileFlags, fileMode)
		if openErr == nil {
			st.file = f
		}
		return
	}
	f, err := os.OpenFile(st.path, fileFlags, fileMode)
	if err != nil {
		return
	}
	st.file = f
}

func (st *state) feedRingLocked(p []byte) {
	st.partial = append(st.partial, p...)
	for {
		i := bytes.IndexByte(st.partial, '\n')
		if i < 0 {
			break
		}
		line := string(bytes.TrimRight(st.partial[:i], "\r"))
		lines.add(line)
		st.partial = st.partial[i+1:]
	}
	if len(st.partial) > maxPartial {
		lines.add(string(st.partial))
		st.partial = nil
	}
}

// Recent returns the last n lines written through Install, oldest first.
// n is capped at 500. Without Install, the result is empty.
func Recent(n int) []string {
	stateMu.Lock()
	defer stateMu.Unlock()
	return lines.recent(n)
}

// Tail returns the last n lines of the log file at Path(dir), oldest first.
// It exists because the ring is process-local and `gadak doctor` is its own
// process: without this, doctor's log section can only ever be empty, which
// is the one place the lines were meant to be pasteable from.
//
// The bytes on disk are already scrubbed, but they were scrubbed by whichever
// build wrote them; running them back through scrub costs nothing and keeps
// the never-print rule owned here rather than at the reader.
func Tail(dir string, n int) []string {
	if n <= 0 {
		return nil
	}
	f, err := os.Open(Path(dir))
	if err != nil {
		return nil
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil
	}
	size := fi.Size()
	off := int64(0)
	if size > tailWindow {
		off = size - tailWindow
	}
	buf := make([]byte, size-off)
	if _, err := f.ReadAt(buf, off); err != nil {
		return nil
	}
	if off > 0 {
		// The window almost certainly cut a line in half; drop the fragment.
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	all := bytes.Split(bytes.TrimRight(scrub(buf), "\n"), []byte{'\n'})
	if len(all) > n {
		all = all[len(all)-n:]
	}
	out := make([]string, 0, len(all))
	for _, line := range all {
		s := string(bytes.TrimRight(line, "\r"))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

type memRing struct {
	buf   [ringCap]string
	start int
	n     int
}

func (r *memRing) add(line string) {
	if r.n < ringCap {
		r.buf[r.n] = line
		r.n++
		return
	}
	r.buf[r.start] = line
	r.start = (r.start + 1) % ringCap
}

func (r *memRing) recent(n int) []string {
	if n <= 0 || r.n == 0 {
		return nil
	}
	if n > r.n {
		n = r.n
	}
	if n > ringCap {
		n = ringCap
	}
	out := make([]string, n)
	off := r.n - n
	for i := 0; i < n; i++ {
		out[i] = r.buf[(r.start+off+i)%ringCap]
	}
	return out
}

func (r *memRing) reset() {
	r.start = 0
	r.n = 0
}
