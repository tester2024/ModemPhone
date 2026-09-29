//go:build sendtest

// The send test exercises the app's send path against the real modem.
//
// It is behind a build tag because it sends a real SMS, which costs money and
// has a real effect on the SIM, so it is run deliberately rather than with the
// rest of the suite:
//
//	$env:SEND_TEST = "1"
//	$env:SEND_TEST_TO = "09101234567"
//	$env:SEND_TEST_BODY = "UI test"
//	$env:MODEM_PASS = "<your modem password>"
//	go test -tags sendtest ./internal/app/ -run TestLiveSend -v
package app

import (
	"os"
	"strings"
	"testing"
	"time"

	"modemphone/internal/router"
)

// liveClient signs in to the modem, skipping the test when it is unreachable.
func liveClient(t *testing.T) *router.Client {
	t.Helper()
	c := router.New("192.168.0.1", "admin", strings.TrimSpace(os.Getenv("MODEM_PASS")))
	if err := c.Login(); err != nil {
		t.Skipf("modem not reachable: %v", err)
	}
	return c
}

// TestLiveSend sends one short message to the modem's own number and waits for
// it to come back, proving the send and receive paths end to end.
func TestLiveSend(t *testing.T) {
	if os.Getenv("SEND_TEST") == "" {
		t.Skip("set SEND_TEST=1 to allow a real SMS to be sent")
	}
	to := strings.TrimSpace(os.Getenv("SEND_TEST_TO"))
	if to == "" {
		t.Fatal("set SEND_TEST_TO to the recipient number")
	}
	country := strings.TrimSpace(os.Getenv("SEND_TEST_CC"))
	if country == "" {
		country = "+98"
	}
	body := strings.TrimSpace(os.Getenv("SEND_TEST_BODY"))
	if body == "" {
		body = "ModemPhoneTest"
	}
	t.Logf("sending %q to %s%s", body, country, to)

	c := liveClient(t)

	inBefore, err := c.Inbox()
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	outBefore, _ := c.Messages(router.FolderOutbox)
	t.Logf("before: inbox=%d outbox=%d", len(inBefore), len(outBefore))

	if _, err := c.Send(country, to, body); err != nil {
		t.Fatalf("Send: %v", err)
	}
	t.Log("submitted to the modem")

	// The network needs a moment to hand the message over and loop it back, so
	// both boxes are polled for a couple of minutes.
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)

		if out, err := c.Messages(router.FolderOutbox); err == nil && len(out) > len(outBefore) {
			t.Logf("outbox now has %d, newest: [%s] to %s %q",
				len(out), out[0].Index, out[0].Number, out[0].Content)
		}
		in, err := c.Inbox()
		if err != nil {
			continue
		}
		for _, m := range in {
			if strings.Contains(m.Content, body) {
				t.Logf("received it back: [%s] from %s at %s %q",
					m.Index, m.Number, m.Time, m.Content)
				return
			}
		}
	}

	out, _ := c.Messages(router.FolderOutbox)
	in, _ := c.Inbox()
	t.Logf("final: inbox=%d outbox=%d", len(in), len(out))
	if len(out) > len(outBefore) {
		t.Logf("the modem recorded the send in the outbox: %q", out[0].Content)
		t.Error("the message reached the outbox but did not loop back to the inbox")
		return
	}
	t.Error("neither box changed, so the modem did not accept the message")
}
