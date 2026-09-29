package ui

import (
	"testing"

	"fyne.io/fyne/v2/widget"

	"modemphone/internal/router"
)

// TestDeliveryStateNamesKnownStatuses covers the mapping from the modem's
// numeric status to words.
func TestDeliveryStateNamesKnownStatuses(t *testing.T) {
	cases := map[string]string{
		"2":   "Sending",
		"3":   "Sent",
		"4":   "Failed",
		" 3 ": "Sent",
		// Unknown values stay blank rather than being guessed at.
		"0": "",
		"1": "",
		"9": "",
		"":  "",
	}
	for in, want := range cases {
		if got := DeliveryState(in); got != want {
			t.Errorf("DeliveryState(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRowLabelNamesTheFolder checks the outbox says who a message went to,
// rather than showing a recipient where the inbox shows a sender.
func TestRowLabelNamesTheFolder(t *testing.T) {
	fonts := testFonts(t)
	msg := router.Message{Index: "86", Number: "091012345674", Status: "3"}

	inbox := NewMsgList(fonts, nil, true, "", false)
	if got := inbox.rowLabel(msg); got != "091012345674" {
		t.Errorf("inbox label = %q, want the bare number", got)
	}

	outbox := NewMsgList(fonts, nil, false, "To", true)
	if got := outbox.rowLabel(msg); got != "To 091012345674" {
		t.Errorf("outbox label = %q, want it to name the recipient", got)
	}

	// A message with no number still gets the role, not a bare "To".
	empty := NewMsgList(fonts, nil, false, "To", true)
	if got := empty.rowLabel(router.Message{}); got != "To (no sender)" {
		t.Errorf("empty outbox label = %q", got)
	}
}

// TestUSSDHelpChipsRunTheSamePath checks the suggested codes are wired to the
// send handler rather than being decorative.
func TestUSSDHelpChipsRunTheSamePath(t *testing.T) {
	fonts := testFonts(t)

	var sent []string
	p := NewUSSDPanel(fonts,
		func(code string) (router.USSDResponse, error) {
			sent = append(sent, code)
			return router.USSDResponse{State: router.USSDResult, Text: "ok"}, nil
		}, nil, func() error { return nil })

	if p.hintRow == nil || len(p.hintRow.Objects) != len(USSDHelp) {
		t.Fatalf("expected %d suggested codes, got %v", len(USSDHelp), p.hintRow)
	}

	// Press the first chip the way the user would.
	if b, ok := p.hintRow.Objects[0].(*widget.Button); ok {
		b.OnTapped()
	}
	if len(sent) != 1 || sent[0] != USSDHelp[0].Code {
		t.Errorf("pressing a chip sent %v, want [%s]", sent, USSDHelp[0].Code)
	}
	// The field is filled in so the code stays visible.
	if p.code.Text != USSDHelp[0].Code {
		t.Errorf("code field = %q, want %q", p.code.Text, USSDHelp[0].Code)
	}
}
