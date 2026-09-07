//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const gadakProtocolScheme = "gadak"

func protocolClassPath(scheme string) string {
	return `SOFTWARE\Classes\` + scheme
}

func protocolCommandPath(scheme string) string {
	return protocolClassPath(scheme) + `\shell\open\command`
}

func protocolIconPath(scheme string) string {
	return protocolClassPath(scheme) + `\DefaultIcon`
}

func registryMissing(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, registry.ErrNotExist) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.ERROR_FILE_NOT_FOUND || errno == syscall.ERROR_PATH_NOT_FOUND
	}
	return false
}

func readProtocolCommand(scheme string) (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, protocolCommandPath(scheme), registry.QUERY_VALUE)
	if registryMissing(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer k.Close()
	val, _, err := k.GetStringValue("")
	if registryMissing(err) {
		return "", nil
	}
	return val, err
}

// windowsRegistryState is the protocolRegistry over the live HKCU class
// key. Missing keys and values read as the zero protocolState with a nil
// error — nothing registered means "write it" — the same convention
// readProtocolCommand set. The one deliberate difference: "URL Protocol"
// is read as PRESENCE, not data (see protocolState.URLProtocolSet).
type windowsRegistryState struct{}

func (windowsRegistryState) readProtocolState(scheme string) (protocolState, error) {
	class, err := registry.OpenKey(registry.CURRENT_USER, protocolClassPath(scheme), registry.QUERY_VALUE)
	if registryMissing(err) {
		return protocolState{}, nil
	}
	if err != nil {
		return protocolState{}, err
	}
	defer class.Close()
	var st protocolState
	// Present-and-empty is what we write; junk data reads as unset so the
	// comparison sends the rewrite that normalizes it.
	if v, _, err := class.GetStringValue("URL Protocol"); err == nil && v == "" {
		st.URLProtocolSet = true
	} else if err != nil && !registryMissing(err) {
		return protocolState{}, err
	}
	if icon, err := readProtocolValue(protocolIconPath(scheme)); err != nil {
		return protocolState{}, err
	} else {
		st.Icon = icon
	}
	cmd, err := readProtocolCommand(scheme)
	if err != nil {
		return protocolState{}, err
	}
	st.Command = cmd
	return st, nil
}

// readProtocolValue reads a subkey's default value; missing is "".
func readProtocolValue(path string) (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if registryMissing(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer k.Close()
	val, _, err := k.GetStringValue("")
	if registryMissing(err) {
		return "", nil
	}
	return val, err
}

func (windowsRegistryState) writeProtocolState(scheme, exePath string) error {
	return writeProtocolScheme(scheme, exePath)
}

func writeProtocolScheme(scheme, exePath string) error {
	classPath := protocolClassPath(scheme)
	k, _, err := registry.CreateKey(registry.CURRENT_USER, classPath, registry.WRITE)
	if err != nil {
		return fmt.Errorf("HKCU\\%s: %w", classPath, err)
	}
	if err := k.SetStringValue("", "URL:"+scheme); err != nil {
		k.Close()
		return err
	}
	if err := k.SetStringValue("URL Protocol", ""); err != nil {
		k.Close()
		return err
	}
	k.Close()

	iconPath := protocolIconPath(scheme)
	icon, _, err := registry.CreateKey(registry.CURRENT_USER, iconPath, registry.WRITE)
	if err != nil {
		return fmt.Errorf("HKCU\\%s: %w", iconPath, err)
	}
	if err := icon.SetStringValue("", protocolDefaultIcon(exePath)); err != nil {
		icon.Close()
		return err
	}
	icon.Close()

	cmdPath := protocolCommandPath(scheme)
	cmd, _, err := registry.CreateKey(registry.CURRENT_USER, cmdPath, registry.WRITE)
	if err != nil {
		return fmt.Errorf("HKCU\\%s: %w", cmdPath, err)
	}
	if err := cmd.SetStringValue("", protocolCommand(exePath)); err != nil {
		cmd.Close()
		return err
	}
	return cmd.Close()
}

func deleteKeyTree(root registry.Key, path string) error {
	k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS)
	if registryMissing(err) {
		return nil
	}
	if err != nil {
		return err
	}
	names, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil && err != io.EOF {
		return err
	}
	for _, name := range names {
		if err := deleteKeyTree(root, path+`\`+name); err != nil {
			return err
		}
	}
	err = registry.DeleteKey(root, path)
	if registryMissing(err) {
		return nil
	}
	return err
}

func unregisterProtocolScheme(scheme string) error {
	if scheme == "" || strings.ContainsAny(scheme, `\/`) {
		return fmt.Errorf("invalid protocol scheme %q", scheme)
	}
	return deleteKeyTree(registry.CURRENT_USER, protocolClassPath(scheme))
}

// registerGadakProtocol writes HKCU\SOFTWARE\Classes\gadak when the live
// state is not exactly what a current handler would be (all three values —
// GDK-1582). Status is "registered" or "already current". Never fatal for
// the caller.
func registerGadakProtocol(exePath string) (string, error) {
	rewrote, err := registerProtocolScheme(gadakProtocolScheme, exePath, windowsRegistryState{})
	if err != nil {
		return "", err
	}
	if rewrote {
		return "registered", nil
	}
	return "already current", nil
}

func unregisterGadakProtocol() error {
	return unregisterProtocolScheme(gadakProtocolScheme)
}
