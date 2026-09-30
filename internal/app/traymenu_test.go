package app

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
)

// TestTrayMenuIsComplete checks the menu an unseen user would rely on: a way
// back to the window, a way to act, and a way out.
func TestTrayMenuIsComplete(t *testing.T) {
	a := newTestApp(t)
	m := a.trayMenu()

	labels := map[string]*struct {
		action    bool
		separator bool
		checked   bool
	}{}
	seps := 0
	for _, item := range m.Items {
		if item.IsSeparator {
			seps++
			continue
		}
		e := &struct {
			action    bool
			separator bool
			checked   bool
		}{action: item.Action != nil, checked: item.Checked}
		labels[item.Label] = e
	}

	for _, want := range []string{"Open", "Inbox", "New message", "Check for messages now", "Start with Windows", "Quit"} {
		e, ok := labels[want]
		if !ok {
			t.Errorf("the tray menu has no %q entry; it has %v", want, keys(labels))
			continue
		}
		if !e.action {
			t.Errorf("tray entry %q does nothing", want)
		}
	}

	if seps < 2 {
		t.Errorf("got %d separators, want the menu grouped into sections", seps)
	}

	// Quit must be the last thing on the menu, so it cannot be hit by accident.
	last := m.Items[len(m.Items)-1]
	if last.Label != "Quit" {
		t.Errorf("the last tray entry is %q, want Quit", last.Label)
	}
}

// TestTrayMenuSeparatorsAreRealSeparators guards the subtlety that an item with
// an empty label and no action is a blank clickable row, not a separator.
func TestTrayMenuSeparatorsAreRealSeparators(t *testing.T) {
	a := newTestApp(t)
	for i, item := range a.trayMenu().Items {
		if item.IsSeparator {
			continue
		}
		if strings.TrimSpace(item.Label) == "" {
			t.Errorf("entry %d has an empty label and no action, which draws a blank row", i)
		}
	}
}

// TestTrayMenuStartAtLoginIsChecked checks the tick reflects the saved setting
// rather than being hard-coded.
func TestTrayMenuStartAtLoginIsChecked(t *testing.T) {
	a := newTestApp(t)
	a.cfg.StartAtLogin = true
	if !itemFor(a.trayMenu(), "Start with Windows").Checked {
		t.Error("the startup entry is unticked even though the setting is on")
	}

	a.cfg.StartAtLogin = false
	if itemFor(a.trayMenu(), "Start with Windows").Checked {
		t.Error("the startup entry is ticked even though the setting is off")
	}
}

// TestTabNamesAreReachable guards the tray's tab shortcuts: they name tabs that
// must actually exist, or the entries open nothing.
func TestTabNamesAreReachable(t *testing.T) {
	a := newTestApp(t)
	for _, name := range []string{"Inbox", "New message"} {
		if a.tabIndex(name) < 0 {
			t.Errorf("the tray offers %q but there is no such tab", name)
		}
	}
}

func itemFor(m *fyne.Menu, label string) *fyne.MenuItem {
	for _, i := range m.Items {
		if i.Label == label {
			return i
		}
	}
	return &fyne.MenuItem{Label: "<missing: " + label + ">"}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
