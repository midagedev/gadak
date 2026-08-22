package main

// gadak raycast install — unpack the embedded Raycast extension and register
// it with a one-shot `npx ray develop`. Source is go:embed'd so a brew or
// app-bundle install works without a checkout (GDK-182).

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	raycastext "github.com/midagedev/gadak/contrib/raycast"
	"github.com/midagedev/gadak/internal/clitool"
)

func init() {
	// helps is declared in help.go; register here so the command stays in
	// lockstep with TestHelpCoversAllCommands.
	helps["raycast"] = cmdHelp{
		summary: "install the Raycast extension that searches the local mirror (macOS only)",
		usage:   "gadak raycast install",
		examples: []string{
			"gadak raycast install",
		},
		seeAlso: []string{"gadak search", "gadak mcp install raycast"},
	}
}

const (
	developSuccessMarker   = "built extension successfully"
	developTimeoutDefault  = 90 * time.Second
	developSettleDefault   = 3 * time.Second
	developStopWaitDefault = 10 * time.Second
)

// lookPath is exec.LookPath; tests inject a stub.
var lookPath = exec.LookPath

// fileIsExec reports a path exists and has an execute bit. Tests inject a stub.
var fileIsExec = isExecutable

func cmdRaycast(args []string) error {
	if len(args) == 0 || wantsHelp(args) {
		printHelp("raycast")
		return nil
	}
	if args[0] == "install" {
		return cmdRaycastInstall(args[1:])
	}
	return usageError("raycast", "usage: gadak raycast install")
}

func cmdRaycastInstall(args []string) error {
	return cmdRaycastInstallFor(runtime.GOOS, args)
}

// raycastInstallErrFor is the GOOS seam for `gadak raycast install`.
// Raycast is a macOS app; every other GOOS must refuse before unpacking
// the extension so a Windows/Linux binary does not write a directory that
// cannot register (GDK-354 / os-audit F-7). Tests pin linux/windows/js on
// any host the same way clitool.ResolveFor does.
func raycastInstallErrFor(goos string) error {
	if goos == "darwin" {
		return nil
	}
	return fmt.Errorf("gadak raycast install is not supported on %s — Raycast is macOS-only", goos)
}

func cmdRaycastInstallFor(goos string, args []string) error {
	if wantsHelp(args) {
		printHelp("raycast")
		return nil
	}
	if len(args) > 0 {
		return usageError("raycast", "usage: gadak raycast install")
	}
	if err := raycastInstallErrFor(goos); err != nil {
		return err
	}

	dest, err := clitool.RaycastExtDir()
	if err != nil {
		return err
	}
	if err := deployRaycastExt(dest, raycastext.FS); err != nil {
		return err
	}

	npm, ok := clitool.ResolveNPM(lookPath, fileIsExec)
	if !ok {
		return fmt.Errorf("%s", strings.TrimRight(npmMissingMessage(), "\n"))
	}

	if err := runNPMCi(npm, dest); err != nil {
		return err
	}
	npx := filepath.Join(filepath.Dir(npm), "npx")
	if !isExecutable(npx) {
		if p, err := lookPath("npx"); err == nil && p != "" {
			npx = p
		}
	}
	if err := runRayDevelop(npx, dest, developTimeoutDefault, developSettleDefault); err != nil {
		return err
	}

	fmt.Printf("설치 완료: %s\n", clitool.TildeHome(dest))
	fmt.Println("Raycast에서 'Search Jira & Confluence'를 실행하세요")
	fmt.Println("스토어 심사가 끝나면 Raycast Store 설치로 교체할 수 있습니다.")
	return nil
}

func deployRaycastExt(dst string, src fs.FS) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	if err := clearManagedExcept(dst, "node_modules"); err != nil {
		return fmt.Errorf("clear %s: %w", dst, err)
	}
	if err := extractEmbedFS(dst, src); err != nil {
		return fmt.Errorf("extract extension: %w", err)
	}
	return nil
}

func clearManagedExcept(dir, keep string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.Name() == keep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func extractEmbedFS(dst string, src fs.FS) error {
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return os.MkdirAll(dst, 0o755)
		}
		target := filepath.Join(dst, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode()&0o111 != 0
}

func npmMissingMessage() string {
	return "Node.js (npm) is required to install the Raycast extension.\n" +
		"Looked for npm on " + clitool.NPMNotFoundDetail() + ".\n" +
		"The extension is under review at https://github.com/raycast/extensions/pull/30297\n" +
		"Until it is in the store, you can install from source:\n" +
		"  git clone https://github.com/midagedev/gadak && cd gadak/contrib/raycast\n" +
		"  npm ci && npm run dev\n"
}

func runNPMCi(npm, dir string) error {
	cmd := exec.Command(npm, "ci", "--no-audit", "--no-fund", "--loglevel=error")
	cmd.Dir = dir
	cmd.Env = prependPATH(os.Environ(), filepath.Dir(npm))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm ci failed: %w", err)
	}
	return nil
}

func runRayDevelop(npx, dir string, timeout, settle time.Duration) error {
	cmd := exec.Command(npx, "ray", "develop")
	cmd.Dir = dir
	cmd.Env = prependPATH(os.Environ(), filepath.Dir(npx))

	// Both streams frame complete lines per stream and feed one pipe;
	// watchDevelopLines — the same judge the tests drive with io.Reader
	// fakes — reads it. Line judgement lives in exactly one place (GDK-615).
	pr, pw := io.Pipe()
	notify := func(line string) { _, _ = pw.Write([]byte(line + "\n")) }
	cmd.Stdout = &notifyWriter{out: os.Stdout, notify: notify}
	cmd.Stderr = &notifyWriter{out: os.Stderr, notify: notify}

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		return fmt.Errorf("start npx ray develop: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		_ = pw.Close() // EOF is how the watcher learns the process is gone.
		waitCh <- err
	}()

	w := watchDevelopLines(pr, timeout)
	// The watcher stopped reading. Drain the pipe until pw.Close or the
	// child's output copies block on notify and cmd.Wait can never return.
	go func() { _, _ = io.Copy(io.Discard, pr) }()

	switch {
	case w.Found:
		time.Sleep(settle)
		_ = interruptProcess(cmd.Process)
		select {
		case <-waitCh:
			return nil
		case <-time.After(developStopWaitDefault):
			_ = killProcess(cmd.Process)
			<-waitCh
			return nil
		}
	case w.TimedOut:
		_ = killProcess(cmd.Process)
		<-waitCh
		if w.RaycastMissing {
			return fmt.Errorf("Raycast가 설치되어 있어야 합니다\n%s", w.Output)
		}
		return fmt.Errorf("npx ray develop did not report success within %s\n%s", timeout, w.Output)
	default:
		// EOF before the marker: the process exited first.
		err := <-waitCh
		if w.RaycastMissing {
			return fmt.Errorf("Raycast가 설치되어 있어야 합니다\n%s", w.Output)
		}
		if err != nil {
			return fmt.Errorf("npx ray develop failed: %w\n%s", err, w.Output)
		}
		return fmt.Errorf("npx ray develop exited before reporting success\n%s", w.Output)
	}
}

func lineHasDevelopSuccess(line string) bool {
	return strings.Contains(line, developSuccessMarker)
}

func looksLikeRaycastMissing(line string) bool {
	l := strings.ToLower(line)
	return strings.Contains(l, "raycast is not running") ||
		strings.Contains(l, "cannot notify raycast") ||
		strings.Contains(l, "raycast is not installed") ||
		strings.Contains(l, "couldn't find raycast") ||
		strings.Contains(l, "could not find raycast")
}

// watchDevelopLines reads r line by line until the success marker appears,
// the timeout elapses, or r ends. It is the single judge for `npx ray
// develop` output: runRayDevelop feeds it the child's merged stdout/stderr,
// and tests drive it with io.Reader fakes so CI never needs Raycast or npm
// (GDK-615).
func watchDevelopLines(r io.Reader, timeout time.Duration) developWatch {
	type ev struct {
		line string
		err  error
		done bool
	}
	ch := make(chan ev, 8)
	go func() {
		s := bufio.NewScanner(r)
		s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for s.Scan() {
			ch <- ev{line: s.Text()}
		}
		if err := s.Err(); err != nil {
			ch <- ev{err: err, done: true}
			return
		}
		ch <- ev{done: true}
	}()

	var b strings.Builder
	var w developWatch
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			w.TimedOut = true
			w.Output = b.String()
			return w
		case e := <-ch:
			if e.done {
				// EOF or a read error ends the stream.
				w.Output = b.String()
				return w
			}
			// Every scanned line is recorded, blank ones included, so the
			// transcript matches what the child actually printed.
			b.WriteString(e.line)
			b.WriteByte('\n')
			if looksLikeRaycastMissing(e.line) {
				w.RaycastMissing = true
			}
			if lineHasDevelopSuccess(e.line) {
				w.Found = true
				w.Output = b.String()
				return w
			}
		}
	}
}

type developWatch struct {
	Found          bool
	TimedOut       bool
	RaycastMissing bool
	Output         string
}

type notifyWriter struct {
	out    io.Writer
	notify func(string)
	mu     sync.Mutex
	rest   []byte
}

func (w *notifyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	data := append(w.rest, p...)
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(data[:i]), "\r")
		w.notify(line)
		data = data[i+1:]
	}
	w.rest = append(w.rest[:0], data...)
	w.mu.Unlock()
	return w.out.Write(p)
}

func interruptProcess(p *os.Process) error {
	if p == nil {
		return nil
	}
	return p.Signal(os.Interrupt)
}

func killProcess(p *os.Process) error {
	if p == nil {
		return nil
	}
	return p.Kill()
}

func prependPATH(env []string, dir string) []string {
	if dir == "" || dir == "." {
		return env
	}
	prefix := "PATH=" + dir + string(os.PathListSeparator)
	out := make([]string, len(env))
	copy(out, env)
	for i, e := range out {
		if strings.HasPrefix(e, "PATH=") {
			out[i] = prefix + strings.TrimPrefix(e, "PATH=")
			return out
		}
	}
	return append(out, "PATH="+dir)
}
