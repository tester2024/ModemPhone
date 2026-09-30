package ui

import "testing"

// The fixtures here are invented. Nothing in this file is taken from a real
// inbox, and the numbers are ones no network would route.

// digitCounts checks that a body yields the expected number of copyable
// entries, which is all these cases are really asserting.
var digitCounts = map[string]int{
	// a USSD code
	"*100#": 1,
	// a code with an asterisk inside and a trailing hash
	"*10*411#": 1,
	// a short reference number, which is worth offering
	"code: 12345 a brand name here": 1,
	// a hashtag is not a USSD code: only a leading asterisk marks one
	"code: 12345 a name #12345": 1,
	// an international number keeps its plus
	"call +98910273751 today": 1,
	// a local number with spaces and dashes
	"dial 0910 123 4567 now": 1,
	// Persian digits, the range an Arabic-only class would miss
	"شماره ۰۹۱۰۱۲۳۴۵۶۷ را تماس بگیرید": 1,
	// nothing worth offering
	"no numbers here at all":      0,
	"short 1234":                  0,
	"a date 2026/01/01 in a text": 0,
}

// TestExtractNumbersFindsTheWorthwhileThings walks the shapes that turn up in
// SMS and checks each yields the expected count.
func TestExtractNumbersFindsTheWorthwhileThings(t *testing.T) {
	for body, want := range digitCounts {
		if got := len(ExtractNumbers(body)); got != want {
			t.Errorf("%q: got %d numbers, want %d (%v)", body, got, want, ExtractNumbers(body))
		}
	}
}

// TestExtractNumbersKeepsPersianDigits checks the Persian digit range is
// recognised, and that the digits are preserved as written.
func TestExtractNumbersKeepsPersianDigits(t *testing.T) {
	const body = "شماره ۰۹۱۰۱۲۳۴۵۶۷ را تماس بگیرید"
	got := ExtractNumbers(body)
	if len(got) != 1 {
		t.Fatalf("got %v, want one number", got)
	}
	if !containsDigits(got[0], "۰۹۱۰۱۲۳۴۵۶۷") {
		t.Errorf("got %q, want the Persian digits preserved", got[0])
	}
}

// TestExtractNumbersKeepsReadingOrder checks numbers come back in the order they
// appear, so the chips read the way the message does.
func TestExtractNumbersKeepsReadingOrder(t *testing.T) {
	body := "first 09101234567 then *100# and last 09101234568"
	got := ExtractNumbers(body)
	seen := map[string]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, want := range []string{"*100#", "09101234567", "09101234568"} {
		if !seen[want] {
			t.Errorf("missing %q from %v", want, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("got %v, want three entries", got)
	}
}

func containsDigits(s, want string) bool {
	return len(s) >= len(want) && indexOf(s, want) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
