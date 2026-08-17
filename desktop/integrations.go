package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/midagedev/gadak/internal/integrations"
)

// installGate allows one in-flight install per catalog id (process-local).
var installGate = struct {
	mu     sync.Mutex
	active map[string]struct{}
}{active: map[string]struct{}{}}

func handleIntegrationsGET(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": integrations.List()})
}

func handleIntegrationsInstall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	args, ok := integrations.InstallArgs(id)
	if !ok {
		http.Error(w, `{"error":"unknown_integration"}`, http.StatusNotFound)
		return
	}
	if !beginInstall(id) {
		http.Error(w, `{"error":"install_in_progress"}`, http.StatusConflict)
		return
	}
	defer endInstall(id)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	cli, err := resolveDesktopCLI()
	if err != nil {
		writeInstallLine(w, nil, err.Error())
		writeInstallLine(w, nil, "exit=127")
		return
	}
	runInstallCLI(w, cli, args)
}

func beginInstall(id string) bool {
	installGate.mu.Lock()
	defer installGate.mu.Unlock()
	if _, ok := installGate.active[id]; ok {
		return false
	}
	installGate.active[id] = struct{}{}
	return true
}

func endInstall(id string) {
	installGate.mu.Lock()
	delete(installGate.active, id)
	installGate.mu.Unlock()
}

// resolveDesktopCLI is the bundled gadak used to run install verbs.
// Order: GADAK_DESKTOP_CLI (test override, exclusive), then
// <os.Executable>/../Resources/bin/gadak, LookPath("gadak"),
// /opt/homebrew/bin/gadak, /usr/local/bin/gadak.
func resolveDesktopCLI() (string, error) {
	if p := os.Getenv("GADAK_DESKTOP_CLI"); p != "" {
		if desktopCLIOK(p) {
			return p, nil
		}
		return "", fmt.Errorf("gadak CLI not found at GADAK_DESKTOP_CLI=%s", p)
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "..", "Resources", "bin", "gadak")
		if desktopCLIOK(p) {
			return p, nil
		}
	}
	if p, err := exec.LookPath("gadak"); err == nil && p != "" {
		return p, nil
	}
	for _, p := range []string{"/opt/homebrew/bin/gadak", "/usr/local/bin/gadak"} {
		if desktopCLIOK(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("gadak CLI not found")
}

func desktopCLIOK(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode()&0o111 != 0
}

func runInstallCLI(w http.ResponseWriter, cli string, args []string) {
	// No request context: a dropped client must not kill the installer
	// (a half-written raycast-extension is worse than a finished orphan).
	cmd := exec.Command(cli, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeInstallLine(w, nil, err.Error())
		writeInstallLine(w, nil, "exit=127")
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		writeInstallLine(w, nil, err.Error())
		writeInstallLine(w, nil, "exit=127")
		return
	}
	if err := cmd.Start(); err != nil {
		writeInstallLine(w, nil, err.Error())
		writeInstallLine(w, nil, "exit=127")
		return
	}

	var writeMu sync.Mutex
	var wg sync.WaitGroup
	drain := func(r io.Reader) {
		defer wg.Done()
		br := bufio.NewReader(r)
		for {
			chunk, err := br.ReadBytes('\n')
			if len(chunk) > 0 {
				writeInstallLine(w, &writeMu, strings.TrimRight(string(chunk), "\r\n"))
			}
			if err != nil {
				return
			}
		}
	}
	wg.Add(2)
	go drain(stdout)
	go drain(stderr)
	wg.Wait()

	exit := 0
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			exit = 1
		}
	}
	writeInstallLine(w, &writeMu, fmt.Sprintf("exit=%d", exit))
}

func writeInstallLine(w http.ResponseWriter, mu *sync.Mutex, line string) {
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	_, _ = fmt.Fprintln(w, line)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
