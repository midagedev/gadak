package config

// Machine-scoped settings (GDK-2070): values that belong to the machine's
// gadak home rather than to one workspace. The first resident is
// skill.autoInstall — the opt-out that stops `gadak init`, pairing and
// `gadak install-cli` from creating the Claude Code skill — which must hold
// across workspaces: opting out on this machine and then running init for a
// new named workspace may not prompt or install. config.json cannot carry
// that (it is per-workspace), so machine settings live in their own file at
// the gadak home root (config.DirFor("")), beside the skill auto-sync stamp,
// which is machine-scoped for the same reason.

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/midagedev/gadak/internal/atomicfile"
	"github.com/midagedev/gadak/internal/fsperm"
)

// machineSettingsFile is the machine-level settings document at the gadak
// home root. JSON, one object of dotted path → value, 0600 like config.json.
const machineSettingsFile = "machine-settings.json"

// SkillAutoInstallPath is the catalog path of the skill auto-install switch.
// Named here so the settings catalog, the CLI's undo line and the opt-out
// check cannot drift apart on the spelling.
const SkillAutoInstallPath = "skill.autoInstall"

func machineSettingsPath() (string, error) {
	dir, err := DirFor("")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, machineSettingsFile), nil
}

// readMachineSettings is the zero map for a missing, unreadable or corrupt
// file — same convention as Load: a fresh machine is not an error, it just
// has every machine setting at its default.
func readMachineSettings() map[string]any {
	p, err := machineSettingsPath()
	if err != nil {
		return nil
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// SetMachineSetting stores one machine-level path. A nil value deletes the
// key (back to the default), mirroring the config.json convention that an
// untouched setting persists nothing.
func SetMachineSetting(path string, value any) error {
	p, err := machineSettingsPath()
	if err != nil {
		return err
	}
	m := readMachineSettings()
	if m == nil {
		m = map[string]any{}
	}
	if value == nil {
		if _, ok := m[path]; !ok {
			return nil
		}
		delete(m, path)
	} else {
		if v, ok := m[path]; ok && v == value {
			return nil
		}
		m[path] = value
	}
	if len(m) == 0 {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := fsperm.EnsurePrivateDir(filepath.Dir(p)); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(p, "machine-settings-*.json", append(raw, '\n'))
}

// MachineBoolSetting reads one machine-level bool path. ok is false when the
// key is absent or not a bool, which every caller reads as the default.
func MachineBoolSetting(path string) (value, ok bool) {
	v, present := readMachineSettings()[path]
	if !present {
		return false, false
	}
	b, isBool := v.(bool)
	return b, isBool
}

// SkillAutoInstallEnabled reports whether the automatic skill install is
// allowed on this machine. Absent means allowed: the opt-out is the state a
// user acts to create, so it is the only thing worth storing.
func SkillAutoInstallEnabled() bool {
	if off, ok := MachineBoolSetting(SkillAutoInstallPath); ok {
		return off
	}
	return true
}

// SetSkillAutoInstall records the machine-wide skill auto-install switch.
// false opts out (init's "n" answer lands here too); true returns to the
// default, so the key is removed rather than stored as an explicit on.
func SetSkillAutoInstall(enabled bool) error {
	if enabled {
		return SetMachineSetting(SkillAutoInstallPath, nil)
	}
	return SetMachineSetting(SkillAutoInstallPath, false)
}

// machineBoolSetting builds the catalog entry for one machine-level bool.
// Get reads the machine file with the default filled, ignoring the
// per-workspace Config the catalog hands every closure (a machine setting
// never reads it). Set writes the machine file itself and never touches c,
// so the caller's cfg.Save() afterwards rewrites config.json byte-identically
// (or writes a bare {} into a home that had no workspace config yet — the
// same empty document many verbs mint; a machine setting rides beside it,
// never inside it).
//
// The description must say the value is machine-wide; a reader deciding
// whether to set it per-workspace needs that fact where the value is shown.
func machineBoolSetting(path, root, desc string, def bool, set func(bool) error) Setting {
	return Setting{
		Path:        path,
		Root:        root,
		Description: desc,
		Get: func(*Config) any {
			if v, ok := MachineBoolSetting(path); ok {
				return v
			}
			return def
		},
		Set: func(_ *Config, raw json.RawMessage) error {
			b, err := decodeBool(raw, path)
			if err != nil {
				return err
			}
			return set(b)
		},
	}
}
