//go:build !windows

// Package startup registers the app to run at login. This file is the
// placeholder for platforms that have no such list; every call is a no-op so
// the rest of the app does not need to branch.
package startup

// IsEnabled always reports false.
func IsEnabled() bool { return false }

// Enable does nothing.
func Enable() error { return nil }

// Disable does nothing.
func Disable() error { return nil }

// Set does nothing and reports false.
func Set(bool) (bool, error) { return false, nil }

// Command returns the empty string.
func Command() string { return "" }
