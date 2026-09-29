package ui

import (
	"strings"
	"testing"

	"modemphone/internal/router"
)

// The fixtures here are invented. Nothing in this file is taken from a real
// inbox, and the numbers are ones no network would route.
const (
	faNumber   = "09101234567"  //
	faNumber2  = "09101234568"  //
	faSender   = "A SENDER"     //
	faUSSD     = "*100#"
	faIndex    = "1"
	faTime     = "2026-01-01 10:00:00"
	faTestBody = "hello there friend"
)

func newTestViewer(t *testing.T, onCopied func(string)) *MessageViewer {
	t.Helper()
	fonts := testFonts(t)
	// No window: the clipboard needs one, but these tests cover extraction and
	// the controls, not the clipboard itself.
	return NewMessageViewer(nil, fonts, nil, nil, onCopied)
}

// TestViewerOffersCopyableNumbersFromTheBody is the feature: numbers written in
// a message body become their own clickable controls.
func TestViewerOffersCopyableNumbersFromTheBody(t *testing.T) {
	v := newTestViewer(t, nil)

	v.Show(router.FolderInbox, router.Message{
		Index: faIndex, Number: faSender, Time: faTime,
		Content: "a free day of calls on the network: *10*411#" + "\n" +
			"to activate the code, dial " + faNumber,
	})

	if len(v.numbers) == 0 {
		t.Fatal("no numbers were extracted from a body containing a USSD code and a number")
	}
	joined := strings.Join(v.numbers, " ")
	if !strings.Contains(joined, "*10*411#") {
		t.Errorf("the USSD code is missing from %v", v.numbers)
	}
	if !strings.Contains(joined, faNumber) {
		t.Errorf("the phone number is missing from %v", v.numbers)
	}
	if !v.numbersBox.Visible() {
		t.Error("the numbers row is hidden even though numbers were found")
	}
	if len(v.numbersBox.Objects) == 0 {
		t.Error("the numbers row has no controls in it")
	}
}

// TestViewerHidesNumbersRowWhenThereAreNone checks the row does not leave an
// empty gap on a message with nothing to copy.
func TestViewerHidesNumbersRowWhenThereAreNone(t *testing.T) {
	v := newTestViewer(t, nil)
	v.Show(router.FolderInbox, router.Message{
		Index: faIndex, Number: faSender, Time: faTime,
		Content: "a greeting with no numbers in it",
	})
	if v.numbersBox.Visible() {
		t.Error("the numbers row is visible for a message with no numbers")
	}
	if len(v.numbersBox.Objects) != 0 {
		t.Errorf("the numbers row has %d stray controls", len(v.numbersBox.Objects))
	}
}

// TestViewerKeepsSenderAsPlainText guards the earlier misreading: the sender is
// not a copy control, the numbers in the body are.
func TestViewerKeepsSenderAsPlainText(t *testing.T) {
	v := newTestViewer(t, nil)
	v.Show(router.FolderInbox, router.Message{
		Index: faIndex, Number: faNumber, Time: faTime, Content: faTestBody,
	})
	if v.sender == nil {
		t.Fatal("no sender text")
	}
	if !strings.Contains(v.sender.Text, faNumber) {
		t.Errorf("sender = %q, want the number", v.sender.Text)
	}
}

// TestExtractNumbersHandlesRealShapes covers the kinds of text that turn up in
// SMS: a USSD code, a local number, an international one, and none at all.
func TestExtractNumbersHandlesRealShapes(t *testing.T) {
	cases := map[string]int{
		faUSSD:                               1,
		"code: 12345 a brand name here #12345": 1,
		"call +98910273751 today":            1,
		"no numbers here at all":              0,
		"short 1234":                          0,
		"a date 2026/01/01 in the text":       0,
	}
	for body, want := range cases {
		if got := len(ExtractNumbers(body)); got != want {
			t.Errorf("%q: got %d numbers, want %d (%v)", body, got, want, ExtractNumbers(body))
		}
	}
}
