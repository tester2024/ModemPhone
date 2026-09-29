package ui

import (
	"regexp"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	ftheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Numbers in a message are the things worth copying: a phone number, a USSD
// code, an order number. The modem does not mark them up, so they are found
// with patterns for the shapes that actually turn up in SMS.

// digitRanges matches a digit in any of the three ranges that turn up: ASCII,
// Arabic-Indic and Persian. It is written without brackets so it can be
// composed into a larger character class.
const digitRanges = `0-9٠-٩۰-۹`

// Patterns are compiled once. A USSD code must start with an asterisk, so a
// leading hash — "#18364" in a hashtag — is not mistaken for one.
var (
	ussdPattern = regexp.MustCompile(`\*[` + digitRanges + `][` + digitRanges + `*]{0,15}#?`)
	// A phone number: an optional plus, then digits that may be separated by
	// spaces or dashes.
	phonePattern = regexp.MustCompile(`\+?[` + digitRanges + `][` + digitRanges + ` \-–—]{3,18}[` + digitRanges + `]`)
)

// ExtractNumbers returns the copyable numbers in a body, in the order they
// appear, with duplicates removed. Digits are kept as written, so a
// Persian-digit number stays in Persian digits.
func ExtractNumbers(body string) []string {
	type hit struct {
		at        int
		text      string
		minDigits int
	}
	var hits []hit

	// USSD first, and blank out what it claims, so a code is not also read as
	// a phone number. A USSD code needs only a couple of digits to be useful
	// — "*100#" is three — so it gets a lower bar than a phone number.
	rest := body
	for _, loc := range ussdPattern.FindAllStringIndex(rest, -1) {
		hits = append(hits, hit{at: loc[0], text: rest[loc[0]:loc[1]], minDigits: 2})
		rest = rest[:loc[0]] + strings.Repeat(" ", loc[1]-loc[0]) + rest[loc[1]:]
	}
	for _, loc := range phonePattern.FindAllStringIndex(rest, -1) {
		hits = append(hits, hit{at: loc[0], text: rest[loc[0]:loc[1]], minDigits: 5})
	}

	// Reading order, so numbers appear in the chips the way they read.
	sort.Slice(hits, func(i, j int) bool { return hits[i].at < hits[j].at })

	var out []string
	seen := map[string]bool{}
	for _, h := range hits {
		v := tidyNumber(h.text, h.minDigits)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// tidyNumber trims the separators a greedy match may have swept up, and
// rejects anything with too few digits to be worth offering.
func tidyNumber(s string, minDigits int) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, " -–—")
	s = strings.TrimSpace(s)
	// Count digits, ignoring spaces, dashes and the leading sign, so a string
	// like "12 - - 34" is judged on its digits rather than its length.
	digits := 0
	for _, r := range s {
		if r >= '0' && r <= '9' || r >= '٠' && r <= '٩' || r >= '۰' && r <= '۹' {
			digits++
		}
	}
	if digits < minDigits {
		return ""
	}
	return s
}

// CopyChips builds a row of buttons, one per number, each putting its own text
// on the clipboard when tapped.
//
// The numbers are offered as their own controls rather than as inline spans in
// the message, because Fyne cannot make a run of text clickable, and because
// the shapes differ enough between Persian and Latin text that splicing them
// inline would fight the bidirectional layout.
func CopyChips(numbers []string, w fyne.Window, onCopied func(string)) fyne.CanvasObject {
	if len(numbers) == 0 {
		return nil
	}
	row := container.NewHBox()
	for _, n := range numbers {
		num := n
		b := widget.NewButtonWithIcon(num, ftheme.ContentCopyIcon(), func() {
			if Copy(w, num) && onCopied != nil {
				onCopied(num)
			}
		})
		b.Importance = widget.LowImportance
		b.Alignment = widget.ButtonAlignLeading
		row.Add(b)
	}
	return container.NewPadded(row)
}

// Copy places text on the system clipboard.
//
// The clipboard is reached through the window, the same way a manual Ctrl+C
// inside the app would be.
func Copy(w fyne.Window, text string) bool {
	if w == nil || strings.TrimSpace(text) == "" {
		return false
	}
	w.Clipboard().SetContent(strings.TrimSpace(text))
	return true
}
