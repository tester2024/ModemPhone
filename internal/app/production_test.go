package app

import (
	"strings"
	"testing"
	"time"

	fapp "fyne.io/fyne/v2/app"

	"modemphone/internal/config"
	"modemphone/internal/ui/theme"
)

// newTestApp builds a window and an App without touching the network, so the
// presentation and bookkeeping can be checked on their own.
func newTestApp(t *testing.T) *App {
	t.Helper()
	test := fapp.NewWithID("com.modemphone.apptest")
	fonts, err := theme.Load("../../assets/fonts")
	if err != nil {
		t.Skipf("fonts unavailable: %v", err)
	}
	test.Settings().SetTheme(theme.Dark(fonts))
	win := test.NewWindow("ModemPhone")
	cfg := config.Default()
	cfg.Password = "x"
	return New(win, cfg, fonts)
}

// TestPollWaitBacksOff covers the reconnect behaviour: once the modem stops
// answering, the app waits longer rather than retrying at the poll interval.
func TestPollWaitBacksOff(t *testing.T) {
	cfg := config.Default()
	cfg.PollSeconds = 30

	if got := pollWait(cfg, 0); got != 30*time.Second {
		t.Errorf("healthy poll = %s, want 30s", got)
	}
	if got := pollWait(cfg, 1); got != 15*time.Second {
		t.Errorf("after one failure = %s, want 15s", got)
	}
	if got := pollWait(cfg, 2); got != 15*time.Second {
		t.Errorf("after two failures = %s, want 15s", got)
	}
	if got := pollWait(cfg, 3); got != 60*time.Second {
		t.Errorf("after three failures = %s, want 60s", got)
	}
	if got := pollWait(cfg, 50); got != 60*time.Second {
		t.Errorf("after many failures = %s, want 60s", got)
	}
}

// TestPollWaitFloorsTheInterval guards against a config of zero or one second
// turning the poll into a request flood.
func TestPollWaitFloorsTheInterval(t *testing.T) {
	cfg := config.Default()
	cfg.PollSeconds = 0
	if got := pollWait(cfg, 0); got < 5*time.Second {
		t.Errorf("a zero interval gave %s, want at least 5s", got)
	}
	cfg.PollSeconds = 1
	if got := pollWait(cfg, 0); got < 5*time.Second {
		t.Errorf("a one second interval gave %s, want at least 5s", got)
	}
}

// TestUnreadBadgeTitle checks the inbox tab is labelled with the unread count,
// which is the only place it is visible from outside the inbox.
func TestUnreadBadgeTitle(t *testing.T) {
	a := newTestApp(t)
	if a.inboxTab == nil {
		t.Fatal("no inbox tab")
	}
	a.setUnreadBadge(3)
	if !strings.Contains(a.inboxTab.Text, "3") {
		t.Errorf("tab text = %q, want it to show 3", a.inboxTab.Text)
	}
	a.setUnreadBadge(0)
	if strings.Contains(a.inboxTab.Text, "(") {
		t.Errorf("tab text = %q, want no count once everything is read", a.inboxTab.Text)
	}
}

// TestDiagnosticsMentionsTheLink checks the diagnostics block carries the facts
// a bug report needs.
func TestDiagnosticsMentionsTheLink(t *testing.T) {
	a := newTestApp(t)
	a.lastStatus.LTEStatus = "Connected"
	a.lastStatus.LTEConnType = "TEST-CARRIER-1"
	a.lastStatus.LTEIP = "203.0.113.7"
	a.device.Model = "TEST-MODEL-1"
	a.device.ModuleName = "TEST-MODULE-1"

	got := a.diagnostics()
	for _, want := range []string{
		"TEST-CARRIER-1", "203.0.113.7", "TEST-MODEL-1", "TEST-MODULE-1",
		a.cfg.Host, "poll interval",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("diagnostics is missing %q:\n%s", want, got)
		}
	}
}
