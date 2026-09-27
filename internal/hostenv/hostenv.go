// Package hostenv is the single owner of the correction a GUI-launched
// process applies to the environment it hands to the shells it starts
// (GDK-2032).
//
// Finder/Dock launches a process with launchd's environment: a four-entry
// PATH and no locale. A terminal pane started under it runs a non-login
// shell, which never reads the profiles that would fix either (/etc/zprofile's
// path_helper, ~/.zprofile's brew shellenv) — so brew-installed tools are
// invisible in the pane, and zsh's line editor cannot see multibyte input
// for want of LANG. The fix belongs in one place, not patched per call
// site: Resolve asks the login shell once what its environment is, Augment
// applies it to the current environment under a whitelist, and every
// child-spawning surface composes the two.
//
// The whitelist is the load-bearing rule. A login shell's rc files can say
// anything — GADAK_WORKSPACE among them — and a variable some other
// window's identity exported must never overwrite this process's own: the
// pane's bare `gadak` points at the workspace the pane's serve named
// (internal/server/terminal.go), not at whatever a profile last set. Only
// PATH and the locale trio ever cross from the login environment.
package hostenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Login is what a login shell said its environment is.
type Login struct {
	// Env is the "K=V" pairs the login shell reported; nil when
	// resolution failed. It carries only the keys Augment may consume —
	// PATH and the locale trio — because those are the whole whitelist;
	// the probe never asks for the rest.
	Env []string
	// Err is non-nil when the login shell could not be asked or did not
	// answer within the timeout. A failure is never fatal to the caller:
	// Augment still applies its locale default and the session opens on
	// the unaugmented environment.
	Err error
}

// GuardEnv marks the probe child so a gadak started under it does not
// probe again: a login shell's rc file that launches a serve would
// otherwise recurse (login shell → rc → serve → Resolve → login shell …).
// The probe child carries GuardEnv=1, and Resolve refuses to run while
// the current process's own environment has it.
const GuardEnv = "GADAK_HOSTENV"

// resolveTimeout bounds the login-shell probe. A user's profile can hang
// (a network mount, a slow prompt theme); a pane must not wait on it.
const resolveTimeout = 3 * time.Second

// pipeDrainDelay bounds how long the probe keeps waiting on the output
// pipe after the shell itself is gone — the gap between killing the shell
// and a grandchild that inherited the pipe letting it go.
const pipeDrainDelay = time.Second

// dumpScript is the one command Resolve runs. NUL-separated so a value
// containing a newline survives the round trip, and restricted to the
// whitelist's keys so the login environment never carries anything this
// package would refuse to use anyway. printf with \0 (octal zero) is the
// POSIX format escape, understood by the sh/dash/bash/zsh builtins alike.
const dumpScript = `printf 'PATH=%s\0LANG=%s\0LC_ALL=%s\0LC_CTYPE=%s\0' "$PATH" "$LANG" "$LC_ALL" "$LC_CTYPE"`

// dumpKeys is the emission order of parseDump; localeKeys is the priority
// order the locale rules read the trio in. Both orders are fixed so the
// same login environment always parses to the same Login.
var (
	dumpKeys   = []string{"PATH", "LANG", "LC_ALL", "LC_CTYPE"}
	localeKeys = []string{"LC_ALL", "LANG", "LC_CTYPE"}
)

// resolveCache memoizes one probe per (goos, shell) for the life of the
// process: opening a hundred panes must still launch one login shell, and
// a failing shell must log its one line once, not per session. Success and
// failure are both cached — a shell that cannot be asked stays unasked.
var (
	resolveMu    sync.Mutex
	resolveCache = map[string]*resolved{}
)

type resolved struct {
	once  sync.Once
	login Login
}

// Resolve runs `<shell> -l -c '<env dump>'` once and caches the result.
// goos is an argument, never runtime.GOOS inside this package: the same
// binary must be testable against all three platforms from one host.
//
// An empty shell takes $SHELL, then /bin/sh — the same fallback order the
// terminal itself uses, so doctor's report and the pane's environment
// answer for the same shell. On windows there is no login shell to ask
// and no process is started.
func Resolve(ctx context.Context, goos, shell string) Login {
	if goos == "windows" {
		return Login{Err: errors.New("hostenv: no login shell on windows")}
	}
	if os.Getenv(GuardEnv) != "" {
		return Login{Err: fmt.Errorf("hostenv: %s is set; this process is already a login-environment probe", GuardEnv)}
	}
	if shell == "" {
		if s := os.Getenv("SHELL"); s != "" {
			shell = s
		} else {
			shell = "/bin/sh"
		}
	}
	key := goos + "\x00" + shell
	resolveMu.Lock()
	r := resolveCache[key]
	if r == nil {
		r = &resolved{}
		resolveCache[key] = r
	}
	resolveMu.Unlock()
	r.once.Do(func() {
		r.login = probe(ctx, shell)
	})
	return r.login
}

// probe is the single execution behind Resolve's cache.
func probe(ctx context.Context, shell string) Login {
	c, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()
	cmd := exec.CommandContext(c, shell, "-l", "-c", dumpScript)
	cmd.Env = append(os.Environ(), GuardEnv+"=1")
	// WaitDelay is what makes the timeout real: the deadline kills the
	// shell, but a profile's own child (or `sleep 30` itself) inherits the
	// output pipe, and Output would wait that child out however dead the
	// shell already is (measured: a 30s sleep held the probe for 30s past
	// a 3s deadline). Bounding the pipe wait keeps the return near the
	// deadline; the stray child dies with its session when it ever exits.
	cmd.WaitDelay = pipeDrainDelay
	out, err := cmd.Output()
	if err != nil {
		if c.Err() != nil {
			err = fmt.Errorf("timed out after %s", resolveTimeout)
		}
		login := Login{Err: fmt.Errorf("hostenv: %s -l -c: %w", shell, err)}
		// One line per process per shell — the cache guarantees this is
		// not repeated per session.
		log.Printf("hostenv: login environment not resolved: %v", login.Err)
		return login
	}
	return Login{Env: parseDump(out)}
}

// parseDump reads the NUL-separated dump. Anything that is not one of the
// whitelist keys with a non-empty value — noise a profile printed, a
// stray newline — is dropped; a repeated key keeps its last value, the
// same rule os/exec applies to the environment it hands children.
func parseDump(out []byte) []string {
	got := map[string]string{}
	known := map[string]bool{}
	for _, k := range dumpKeys {
		known[k] = true
	}
	for _, tok := range bytes.Split(out, []byte{0}) {
		k, v, ok := strings.Cut(string(tok), "=")
		if !ok || v == "" || !known[k] {
			continue
		}
		got[k] = v
	}
	env := make([]string, 0, len(got))
	for _, k := range dumpKeys {
		if v, ok := got[k]; ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// Augment returns the environment a child process should get: base (the
// current process environment, os.Environ()) plus what the login shell is
// allowed to contribute.
//
// The rules, which the tests pin one by one:
//
//   - PATH keeps every base entry in place and in order; entries that only
//     the login PATH has are appended after them, in login order, with no
//     duplicates. A base with no PATH at all takes the login PATH whole.
//   - Locale: if base already carries a value in LC_ALL, LANG or LC_CTYPE,
//     nothing is touched. Otherwise the login environment's trio is taken,
//     and if it has none either, LANG is set — en_US.UTF-8 on darwin,
//     C.UTF-8 elsewhere.
//   - The whitelist: only PATH and the locale trio ever come from login.
//     A login shell that says GADAK_WORKSPACE=other is ignored, because
//     that variable names this process's own window.
//   - windows returns base unchanged, and a failed Login leaves PATH as
//     base had it and applies only the locale default: neither failure
//     mode may keep a session from opening.
//
// Path entries are split on os.PathListSeparator — ":" on every GOOS that
// reaches the merge, since windows returns before it.
func Augment(goos string, base []string, login Login) []string {
	if goos == "windows" {
		return base
	}
	out := make([]string, 0, len(base)+len(localeKeys)+1)
	out = append(out, base...)

	if login.Err == nil {
		if basePATH, ok := envLookup(base, "PATH"); ok {
			if merged := mergePath(basePATH, envValue(login.Env, "PATH")); merged != basePATH {
				replaceValue(out, "PATH", merged)
			}
		} else if lp := envValue(login.Env, "PATH"); lp != "" {
			out = append(out, "PATH="+lp)
		}
	}

	if !hasLocaleValue(base) {
		fromLogin := false
		if login.Err == nil {
			for _, k := range localeKeys {
				if v := envValue(login.Env, k); v != "" {
					out = append(out, k+"="+v)
					fromLogin = true
				}
			}
		}
		if !fromLogin {
			lang := "C.UTF-8"
			if goos == "darwin" {
				lang = "en_US.UTF-8"
			}
			out = append(out, "LANG="+lang)
		}
	}
	return out
}

// LocaleSource names where Augment's locale came from: "base" (base
// already carried one, or the platform does no augmenting), "login" (the
// login shell's), or "default" (the floor Augment set itself). It is the
// diagnostic half of the same rule Augment applies, exposed once so a
// report cannot disagree with the merge it reports on.
func LocaleSource(goos string, base []string, login Login) string {
	if goos == "windows" || hasLocaleValue(base) {
		return "base"
	}
	if login.Err == nil {
		for _, k := range localeKeys {
			if envValue(login.Env, k) != "" {
				return "login"
			}
		}
	}
	return "default"
}

// mergePath appends to base the entries login has that base does not, in
// login order, skipping duplicates — base's own entries, positions and
// internal repetitions are never touched.
func mergePath(base, login string) string {
	if login == "" {
		return base
	}
	sep := string(os.PathListSeparator)
	have := map[string]bool{}
	for _, e := range strings.Split(base, sep) {
		if e != "" {
			have[e] = true
		}
	}
	var add []string
	for _, e := range strings.Split(login, sep) {
		if e == "" || have[e] {
			continue
		}
		have[e] = true
		add = append(add, e)
	}
	if len(add) == 0 {
		return base
	}
	return base + sep + strings.Join(add, sep)
}

// envValue returns the value of key in a "K=V" list, "" when the key is
// absent or set empty.
func envValue(env []string, key string) string {
	v, _ := envLookup(env, key)
	return v
}

// envLookup is envValue with presence: ok says the key exists at all.
func envLookup(env []string, key string) (string, bool) {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

// replaceValue overwrites every occurrence of key in env (the caller
// passes a fresh copy): realistic input has at most one.
func replaceValue(env []string, key, value string) {
	for i, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok && k == key {
			env[i] = key + "=" + value
		}
	}
}

// hasLocaleValue reports whether env carries any locale variable with a
// value — the condition that leaves the locale axes untouched.
func hasLocaleValue(env []string) bool {
	for _, k := range localeKeys {
		if envValue(env, k) != "" {
			return true
		}
	}
	return false
}
