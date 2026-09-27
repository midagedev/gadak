package hostenv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The fixtures are fake shells and synthetic environments: nothing here
// runs the developer's real login shell, because a machine's rc files are
// not a contract (CI runners differ, and a slow profile would turn the
// suite into a timing test).

// pathEntries splits a PATH value, dropping empty segments.
func pathEntries(p string) []string {
	var out []string
	for _, e := range strings.Split(p, ":") {
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

// guiBase is the environment Finder/Dock hands a process: launchd's PATH
// and nothing else (GDK-2032, measured on 0.24.0). GADAK_WORKSPACE stands
// in for the process's own identity — the variable the whitelist exists
// to protect.
var guiBase = []string{
	"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
	"GADAK_WORKSPACE=this-window",
}

// ① The PATH merge: the login shell's entries that base does not have are
// appended after base's, base's entries keep their relative order, and no
// entry appears twice. This is the case the pane actually hits — brew's
// /opt/homebrew/bin is invisible in a GUI-launched serve until this
// merge runs.
func TestAugmentAppendsLoginOnlyPathEntries(t *testing.T) {
	login := Login{Env: []string{
		"PATH=/opt/homebrew/bin:/usr/bin:/bin",
		"LANG=en_US.UTF-8",
	}}
	got := Augment("darwin", guiBase, login)

	entries := pathEntries(envValue(got, "PATH"))
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e] {
			t.Errorf("PATH has a duplicate entry %q: %q", e, envValue(got, "PATH"))
		}
		seen[e] = true
	}
	if !seen["/opt/homebrew/bin"] {
		t.Errorf("PATH %q does not carry the login shell's /opt/homebrew/bin", envValue(got, "PATH"))
	}
	// The four base entries keep their relative order.
	last := -1
	for _, e := range []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
		at := -1
		for i, x := range entries {
			if x == e {
				at = i
				break
			}
		}
		if at < 0 {
			t.Fatalf("base PATH entry %q lost from %q", e, envValue(got, "PATH"))
		}
		if at <= last {
			t.Errorf("base PATH entry %q reordered: %v", e, entries)
		}
		last = at
	}
}

// ② The locale floor: with no locale anywhere, darwin gets
// LANG=en_US.UTF-8, other unixes C.UTF-8, and windows nothing at all —
// base comes back exactly as it went in.
func TestAugmentLocaleFloorPerGoos(t *testing.T) {
	for _, tc := range []struct {
		goos string
		want string // "" means no locale variable may be added
	}{
		{"darwin", "en_US.UTF-8"},
		{"linux", "C.UTF-8"},
		{"windows", ""},
	} {
		got := Augment(tc.goos, guiBase, Login{})
		if tc.want == "" {
			for _, k := range []string{"LANG", "LC_ALL", "LC_CTYPE"} {
				if envValue(got, k) != "" {
					t.Errorf("%s: %s = %q; want unset", tc.goos, k, envValue(got, k))
				}
			}
			if len(got) != len(guiBase) {
				t.Errorf("%s: windows must return base unchanged, got %v", tc.goos, got)
			}
			continue
		}
		if got := envValue(got, "LANG"); got != tc.want {
			t.Errorf("%s: LANG = %q; want %q", tc.goos, got, tc.want)
		}
	}
}

// ③ A base that already carries a locale keeps every locale variable
// exactly as it was: no LANG is added, nothing is renamed.
func TestAugmentDoesNotTouchAnExistingLocale(t *testing.T) {
	base := append([]string{"PATH=/usr/bin:/bin"}, "LC_ALL=ko_KR.UTF-8")
	got := Augment("darwin", base, Login{Env: []string{"LANG=en_US.UTF-8"}})
	if got := envValue(got, "LC_ALL"); got != "ko_KR.UTF-8" {
		t.Errorf("LC_ALL = %q; want base's ko_KR.UTF-8", got)
	}
	if got := envValue(got, "LANG"); got != "" {
		t.Errorf("LANG = %q; want unset (a locale was already present)", got)
	}
}

// ④ The whitelist: a login shell that speaks for another window's
// identity — GADAK_WORKSPACE, or anything else it exported — never
// overwrites the base. The pane's `gadak` stays pointed at this window.
func TestAugmentWhitelistsLoginVariables(t *testing.T) {
	login := Login{Env: []string{
		"PATH=/opt/homebrew/bin:/usr/bin:/bin",
		"GADAK_WORKSPACE=other",
		"SHELL=/bin/other",
	}}
	got := Augment("darwin", guiBase, login)
	if got := envValue(got, "GADAK_WORKSPACE"); got != "this-window" {
		t.Errorf("GADAK_WORKSPACE = %q; want base's this-window — the login shell's value must not cross", got)
	}
	if got := envValue(got, "SHELL"); got != "" {
		t.Errorf("SHELL = %q; want unset — not on the whitelist", got)
	}
}

// ⑤ A failed resolution is not fatal: PATH stays as base had it and only
// the locale default applies.
func TestAugmentFailureKeepsBasePathAndDefaultsLocale(t *testing.T) {
	login := Login{Err: errors.New("probe refused")}
	got := Augment("darwin", guiBase, login)
	if got, want := envValue(got, "PATH"), envValue(guiBase, "PATH"); got != want {
		t.Errorf("PATH = %q; want base's %q", got, want)
	}
	if got := envValue(got, "LANG"); got != "en_US.UTF-8" {
		t.Errorf("LANG = %q; want the darwin default", got)
	}
}

// fakeShell writes an executable shell script into a temp dir and returns
// its path. body runs with the arguments Resolve passes (-l -c <dump>).
func fakeShell(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fakeshell")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// ⑥ A login shell that hangs is cut off at the timeout, not waited out.
func TestResolveTimesOutASlowShell(t *testing.T) {
	shell := fakeShell(t, "sleep 30")
	start := time.Now()
	login := Resolve(context.Background(), "darwin", shell)
	elapsed := time.Since(start)
	if login.Err == nil {
		t.Fatalf("Resolve returned no error; want the timeout")
	}
	if login.Env != nil {
		t.Errorf("Env = %v; want nil on failure", login.Env)
	}
	if elapsed < resolveTimeout {
		t.Errorf("Resolve returned after %s; want it to wait out the %s timeout", elapsed, resolveTimeout)
	}
	if elapsed > 15*time.Second {
		t.Errorf("Resolve took %s; the cap did not cut the hanging shell", elapsed)
	}
}

// ⑦ The probe runs once per shell per process, success and failure alike:
// a hundred panes may not launch a hundred login shells.
func TestResolveRunsTheShellOncePerProcess(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "runs")
	shell := fakeShell(t, fmt.Sprintf("printf 'run\\n' >> %s\nprintf 'PATH=/opt/fake/bin\\0'", counter))
	for i := 0; i < 3; i++ {
		if login := Resolve(context.Background(), "darwin", shell); login.Err != nil {
			t.Fatalf("Resolve %d: %v", i, login.Err)
		}
	}
	runs, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Split(strings.TrimSpace(string(runs)), "\n")); n != 1 {
		t.Errorf("the fake shell ran %d times; want 1 (cached per process)", n)
	}
	// And the cached answer is the probe's, not a zero Login.
	if login := Resolve(context.Background(), "darwin", shell); envValue(login.Env, "PATH") != "/opt/fake/bin" {
		t.Errorf("cached PATH = %q; want the probe's /opt/fake/bin", envValue(login.Env, "PATH"))
	}
}

// The probe asks a login shell: -l, then -c, then the dump. A plain -c
// would read no profile and answer exactly the environment this package
// exists to correct.
func TestResolveAsksALoginShell(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	shell := fakeShell(t, fmt.Sprintf("printf '%%s\\0' \"$@\" > %s\nprintf 'PATH=/opt/fake/bin\\0'", argsFile))
	if login := Resolve(context.Background(), "darwin", shell); login.Err != nil {
		t.Fatalf("Resolve: %v", login.Err)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	// NUL-separated: the dump argument itself contains spaces.
	got := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	want := []string{"-l", "-c", dumpScript}
	if len(got) != len(want) {
		t.Fatalf("probe args = %q; want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("probe arg %d = %q; want %q", i, got[i], want[i])
		}
	}
}

// The recursion guard: a process that is itself a login-environment probe
// (GADAK_HOSTENV inherited from the probe child) must not launch another
// one, however reachable its shell is.
func TestResolveRefusesUnderTheGuardEnv(t *testing.T) {
	shell := fakeShell(t, "printf 'PATH=/opt/never/bin\\0'")
	t.Setenv(GuardEnv, "1")
	login := Resolve(context.Background(), "darwin", shell)
	if login.Err == nil {
		t.Fatal("Resolve probed under the guard; want refusal")
	}
	if login.Env != nil {
		t.Errorf("Env = %v; want nil", login.Env)
	}
}

// A shell that fails outright (missing binary) reports Err, once.
func TestResolveReportsAMissingShell(t *testing.T) {
	login := Resolve(context.Background(), "darwin", filepath.Join(t.TempDir(), "no-such-shell"))
	if login.Err == nil {
		t.Fatal("Resolve of a missing shell returned no error")
	}
	if login.Env != nil {
		t.Errorf("Env = %v; want nil on failure", login.Env)
	}
}

// LocaleSource names the branch Augment took, so a report and the merge
// it describes cannot disagree.
func TestLocaleSource(t *testing.T) {
	for _, tc := range []struct {
		name  string
		goos  string
		base  []string
		login Login
		want  string
	}{
		{"base carries LANG", "darwin", []string{"LANG=ko_KR.UTF-8"}, Login{}, "base"},
		{"login supplies it", "darwin", nil, Login{Env: []string{"LANG=en_US.UTF-8"}}, "login"},
		{"nobody has it", "linux", nil, Login{}, "default"},
		{"failed probe", "darwin", nil, Login{Err: errors.New("no")}, "default"},
		{"windows", "windows", nil, Login{Env: []string{"LANG=en_US.UTF-8"}}, "base"},
	} {
		if got := LocaleSource(tc.goos, tc.base, tc.login); got != tc.want {
			t.Errorf("%s: LocaleSource = %q; want %q", tc.name, got, tc.want)
		}
	}
}
