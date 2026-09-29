package ui

import (
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	fapp "fyne.io/fyne/v2/app"

	"modemphone/internal/router"
	"modemphone/internal/ui/theme"
)

var appOnce sync.Once

// testFonts returns the app font, starting a headless Fyne application first:
// measuring text needs a driver, and the list builds rows on every refresh.
func testFonts(t *testing.T) theme.Fonts {
	t.Helper()
	f, err := theme.Load("../../assets/fonts")
	if err != nil {
		t.Skipf("fonts unavailable: %v", err)
	}
	appOnce.Do(func() {
		a := fapp.NewWithID("com.modemphone.uitest")
		a.Settings().SetTheme(theme.Dark(f))
	})
	if a := fyne.CurrentApp(); a != nil {
		a.Settings().SetTheme(theme.Dark(f))
	}
	return f
}

// TestMatchIsCaseInsensitiveAcrossSenderAndBody covers the filter predicate the
// list uses.
func TestMatchIsCaseInsensitiveAcrossSenderAndBody(t *testing.T) {
	cases := []struct {
		query, sender, body string
		want                bool
	}{
		{"", "MCI", "anything", true},
		{"bank", "Bank Mellat", "text", true},
		{"BANK", "Bank Mellat", "text", true},
		{"mellat", "Bank Mellat", "text", true},
		{"mellat", "Other", "call Mellat", true},
		{"zzz", "Bank Mellat", "text", false},
		{"  bank  ", "Bank Mellat", "text", true},
		{"09101", "091012345674", "text", true},
	}
	for _, c := range cases {
		if got := Match(c.query, c.sender, c.body); got != c.want {
			t.Errorf("Match(%q, %q, %q) = %v, want %v", c.query, c.sender, c.body, got, c.want)
		}
	}
}

// TestMsgListFiltersBySenderAndBody checks the list actually narrows.
func TestMsgListFiltersBySenderAndBody(t *testing.T) {
	fonts := testFonts(t)
	l := NewMsgList(fonts, nil, true, "", false)
	l.SetMessages([]router.Message{
		{Index: "1", Number: "Bank Mellat", Content: "your statement"},
		{Index: "2", Number: "AsiaTech", Content: "special discount"},
		{Index: "3", Number: "9850003006", Content: "gift from the bank"},
	})

	if got := l.shownCount(); got != 3 {
		t.Fatalf("expected 3 rows unfiltered, got %d", got)
	}

	l.SetQuery("mellat")
	if got := l.shownCount(); got != 1 {
		t.Errorf("query 'mellat' matched %d rows, want 1", got)
	}

	l.SetQuery("bank")
	if got := l.shownCount(); got != 2 {
		t.Errorf("query 'bank' matched %d rows, want 2", got)
	}

	l.SetQuery("no-such-thing")
	if got := l.shownCount(); got != 0 {
		t.Errorf("query with no match showed %d rows, want 0", got)
	}

	// Clearing the query restores everything.
	l.SetQuery("")
	if got := l.shownCount(); got != 3 {
		t.Errorf("clearing the query showed %d rows, want 3", got)
	}
}

// TestMsgListFilterIgnoresLeadingAndTrailingSpace guards the common case of a
// pasted number with a stray space.
func TestMsgListFilterIgnoresSpace(t *testing.T) {
	fonts := testFonts(t)
	l := NewMsgList(fonts, nil, true, "", false)
	l.SetMessages([]router.Message{{Index: "1", Number: "Bank Mellat", Content: "x"}})
	l.SetQuery("  mellat ")
	if got := l.shownCount(); got != 1 {
		t.Errorf("padded query matched %d rows, want 1", got)
	}
}

// TestElideForPreviewCutsOnWordBoundary protects Persian: cutting mid-word
// leaves a half-formed joining form.
func TestElideForPreviewCutsOnWordBoundary(t *testing.T) {
	in := "hello there friend this is a long message body that must be shortened"
	got := ElideForPreview(in, 20)

	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected a trailing ellipsis, got %q", got)
	}
	// Whatever survived must be whole words, so the last thing before the
	// ellipsis is a space.
	trimmed := strings.TrimSuffix(got, "…")
	if trimmed != strings.TrimRight(trimmed, " ") {
		t.Errorf("cut landed mid-word: %q", got)
	}
	if strings.HasSuffix(trimmed, " ") {
		t.Errorf("a trailing space was left before the ellipsis: %q", got)
	}
}

// TestElideForPreviewLeavesShortTextAlone checks it does not add an ellipsis
// when nothing needed cutting.
func TestElideForPreviewLeavesShortTextAlone(t *testing.T) {
	const in = "short"
	if got := ElideForPreview(in, 72); got != in {
		t.Errorf("short text was altered: %q", got)
	}
}
