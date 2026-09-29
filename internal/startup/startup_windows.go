// Package startup registers the app to run when Windows starts.
//
// The registration is the per-user Run key rather than a service or a scheduled
// task, so it needs no elevation and can be removed by the user at any time.
package startup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// runKey is the per-user autostart list: HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run.
const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// valueName is the entry's name, which is also what shows in Task Manager's
// Startup tab.
const valueName = "ModemPhone"

// IsEnabled reports whether the app is registered to start with Windows.
func IsEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(valueName)
	return err == nil
}

// Enable registers the app to start with Windows, using the path of the running
// executable so the installed copy is registered rather than the build output.
func Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate the running program: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return fmt.Errorf("resolve the program path: %w", err)
	}
	// A build left in bin\ is not worth autostarting: it is rebuilt and
	// overwritten. Only an installed copy registers.
	if strings.Contains(strings.ToLower(filepath.Base(filepath.Dir(exe))), "bin") {
		return fmt.Errorf("this is a build copy in %s; install it first so the registered path stays valid",
			filepath.Dir(exe))
	}

	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open the startup key: %w", err)
	}
	defer k.Close()

	// Quoted, so a path containing spaces is still understood.
	cmd := fmt.Sprintf("\"%s\"", exe)
	if err := k.SetStringValue(valueName, cmd); err != nil {
		return fmt.Errorf("write the startup entry: %w", err)
	}
	return nil
}

// Disable removes the registration. It is not an error if there was none.
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		// Nothing registered, and no key to remove it from.
		return nil
	}
	defer k.Close()
	if err := k.DeleteValue(valueName); err != nil {
		// A missing value is the state the caller asked for.
		if err == registry.ErrNotExist {
			return nil
		}
		return fmt.Errorf("remove the startup entry: %w", err)
	}
	return nil
}

// Set turns autostart on or off and reports the resulting state.
func Set(enabled bool) (bool, error) {
	if enabled {
		if err := Enable(); err != nil {
			return IsEnabled(), err
		}
		return IsEnabled(), nil
	}
	if err := Disable(); err != nil {
		return IsEnabled(), err
	}
	return IsEnabled(), nil
}

// Command is the string written to the Run key, exposed so it can be shown in
// the app.
func Command() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue(valueName)
	if err != nil {
		return ""
	}
	return v
}
