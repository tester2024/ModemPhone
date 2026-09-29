// Package applog keeps a small rolling log beside the app's settings.
//
// The executable is linked against the GUI subsystem so that launching it does
// not flash a terminal window, which also means the standard logger has nowhere
// to write. This file is the replacement: the same messages still get recorded,
// and the path is shown in the app's diagnostics block.
package applog

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// maxLogBytes is when the file is truncated. It is small on purpose: this is a
// record of the last few sessions, not an audit trail.
const maxLogBytes = 512 * 1024

// Path returns the log file, or the empty string if it cannot be worked out.
func Path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "modemphone", "modemphone.log")
}

// Open starts the logger, rotating the file if it has grown too large. It
// returns nil when the log cannot be opened, in which case logging falls back to
// being discarded rather than failing the app.
func Open() *os.File {
	p := Path()
	if p == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil
	}
	rotate(p)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil
	}
	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime)
	return f
}

// rotate keeps one previous session's log, so a crash is still visible after
// the next launch.
func rotate(p string) {
	st, err := os.Stat(p)
	if err != nil || st.Size() < maxLogBytes {
		return
	}
	old := p + ".1"
	_ = os.Remove(old)
	if err := os.Rename(p, old); err != nil {
		// Not fatal: the log simply keeps growing this session.
		log.Printf("could not rotate the log: %v", err)
	}
}

// Describe returns a one-line summary for the diagnostics block.
func Describe() string {
	p := Path()
	if p == "" {
		return "unavailable"
	}
	st, err := os.Stat(p)
	if err != nil {
		return fmt.Sprintf("%s (not written yet)", p)
	}
	return fmt.Sprintf("%s (%d KB)", p, st.Size()/1024)
}
