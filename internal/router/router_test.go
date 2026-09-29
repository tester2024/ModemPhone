package router

import (
	"os"
	"strings"
	"testing"
)

// liveClient signs in to a real device, skipping the test when one is not
// reachable.
//
// Credentials come from the environment on purpose, so a real password never
// has to live in the repository:
//
//	MODEM_HOST=192.0.2.10  MODEM_USER=admin  MODEM_PASS=...  go test ./internal/router/
func liveClient(t *testing.T) *Client {
	t.Helper()
	host := strings.TrimSpace(os.Getenv("MODEM_HOST"))
	if host == "" {
		t.Skip("set MODEM_HOST to run the live tests against a real device")
	}
	user := strings.TrimSpace(os.Getenv("MODEM_USER"))
	if user == "" {
		user = "admin"
	}
	c := New(host, user, strings.TrimSpace(os.Getenv("MODEM_PASS")))
	if err := c.Login(); err != nil {
		t.Skipf("device not reachable: %v", err)
	}
	return c
}

func TestLiveStatus(t *testing.T) {
	c := liveClient(t)
	s, err := c.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	t.Logf("LTE status=%q carrier=%q uptime=%ds ip=%s signal=%s",
		s.LTEStatus, s.LTEConnType, s.LTEUpTime, s.LTEIP, s.LTESignal)
	t.Logf("ipv4 connected=%v ip=%s | ipv6 connected=%v ip=%s",
		s.IPv4Connected, s.IPv4IP, s.IPv6Connected, s.IPv6IP)
	t.Logf("system uptime=%ds clients=%d", s.UptimeSeconds, s.ClientCount)
}

func TestLiveDevice(t *testing.T) {
	c := liveClient(t)
	d, err := c.Device()
	if err != nil {
		t.Fatalf("Device: %v", err)
	}
	t.Logf("model=%q module=%q imei=%q", d.Model, d.ModuleName, d.IMEI)
}

func TestLiveInboxParsesPersian(t *testing.T) {
	c := liveClient(t)
	msgs, err := c.Inbox()
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	t.Logf("inbox has %d messages", len(msgs))
	if len(msgs) == 0 {
		t.Skip("inbox is empty, nothing to parse")
	}
	for i, m := range msgs {
		if i >= 3 {
			break
		}
		t.Logf("  [%s] from=%s at=%s read=%v\n      %s",
			m.Index, m.Number, m.Time, m.Read,
			strings.ReplaceAll(truncate(m.Content, 90), "\n", " / "))
	}
	// A Persian body must survive as valid UTF-8 with its letters intact.
	var found bool
	for _, m := range msgs {
		for _, r := range m.Content {
			if r >= 0x0600 && r <= 0x06FF {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Error("no Persian characters survived parsing")
	}
}

func TestLiveOutboxAndDrafts(t *testing.T) {
	c := liveClient(t)
	for _, f := range []Folder{FolderOutbox, FolderDrafts} {
		msgs, err := c.Messages(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		t.Logf("%s has %d messages", f, len(msgs))
	}
}

func TestLiveStorage(t *testing.T) {
	c := liveClient(t)
	got, err := c.Storage()
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}
	if got != "SM" && got != "ME" {
		t.Errorf("unexpected storage value %q", got)
	}
	t.Logf("storage=%s", got)
}

// TestLiveUSSD checks whether a USSD code answers. It is expected to time out
// on a modem with no IPv4 data context, so both outcomes are acceptable; the
// test exists to confirm the call returns rather than hanging.
func TestLiveUSSD(t *testing.T) {
	c := liveClient(t)
	_, err := c.SendUSSD("*100#")
	if err == nil {
		t.Log("USSD answered, so an IPv4 data context must be available")
		return
	}
	t.Logf("USSD unavailable as expected: %v", err)
}

func TestLiveValidationRejectsBadInput(t *testing.T) {
	c := New("192.0.2.10", "u", "p")
	if _, err := c.Send("+98", "0910abc", "hi"); err == nil {
		t.Error("expected an error for a number with letters")
	}
	if _, err := c.Send("98x", testPhone, "hi"); err == nil {
		t.Error("expected an error for a bad country code")
	}
	if _, err := c.Send("+98", "", "hi"); err == nil {
		t.Error("expected an error for an empty number")
	}
	if _, err := c.Send("+98", testPhone, ""); err == nil {
		t.Error("expected an error for an empty body")
	}
	if _, err := c.Send("+98", testPhone, strings.Repeat("x", MaxBodyRunes+1)); err == nil {
		t.Error("expected an error for an over-long body")
	}
}

func TestValidators(t *testing.T) {
	ok := []string{testPhone, "0910;0912;0913", "123"}
	for _, n := range ok {
		if err := validateNumber(n); err != nil {
			t.Errorf("validateNumber(%q) = %v, want nil", n, err)
		}
	}
	bad := []string{"0910abc", "+091012345674", "0910;", ";0910", "0910 0911"}
	for _, n := range bad {
		if err := validateNumber(n); err == nil {
			t.Errorf("validateNumber(%q) = nil, want an error", n)
		}
	}
	okCC := []string{"+98", "98", "+1"}
	for _, c := range okCC {
		if err := validateCountryCode(c); err != nil {
			t.Errorf("validateCountryCode(%q) = %v, want nil", c, err)
		}
	}
	badCC := []string{"", "9a8", "+", "++98"}
	for _, c := range badCC {
		if err := validateCountryCode(c); err == nil {
			t.Errorf("validateCountryCode(%q) = nil, want an error", c)
		}
	}
}

func TestParseRecordsSkipsEmptyTail(t *testing.T) {
	recs := parseRecords("1}-{0}-{+09101234567}-{2026-01-01 10:00}-{hello}-{|,|")
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1: %q", len(recs), recs)
	}
	if got := recs[0][4]; got != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}
}

func TestExtractJSStringHandlesEscapes(t *testing.T) {
	page := `var smsListInfo = "a\"b\\c\nd\u0641";`
	got, ok := extractJSString(page, "smsListInfo")
	if !ok {
		t.Fatal("variable not found")
	}
	want := "a\"b\\c\nd\u0641"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestCleanBodyConvertsBreaks(t *testing.T) {
	in := "first<br><br>second<br>third"
	got := cleanBody(in)
	want := "first\nsecond\nthird"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
