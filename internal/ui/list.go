// Package ui contains the app's reusable widgets.
//
// Persian text is handed to Fyne untouched. Fyne shapes Arabic joining forms and
// applies the Unicode Bidirectional Algorithm itself, through go-text's
// segmenter, so pre-processing the text only fights it. The widgets here supply
// the Vazirmatn font through the theme, which is what makes Persian render
// inside Fyne's own widgets too.
package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"modemphone/internal/ui/theme"
)

// Label is a text object in the regular face.
func Label(s string, col color.NRGBA, size float32, fonts theme.Fonts) *canvas.Text {
	t := canvas.NewText(s, col)
	t.TextSize = size
	t.FontSource = fonts.Regular
	return t
}

// fontsMedium returns a font set whose regular face is the medium one, so text
// that needs more weight can still go through the shared Label helper. Persian
// body text reads better this way: the regular face is too thin at small sizes.
func fontsMedium(f theme.Fonts) theme.Fonts {
	if f.Medium == nil {
		return f
	}
	out := f
	out.Regular = f.Medium
	return out
}

// Bold is a text object in the bold face.
func Bold(s string, col color.NRGBA, size float32, fonts theme.Fonts) *canvas.Text {
	t := Label(s, col, size, fonts)
	t.FontSource = fonts.Bold
	return t
}

// Muted is a secondary-colour label.
func Muted(s string, size float32, fonts theme.Fonts) *canvas.Text {
	return Label(s, theme.TextMuted, size, fonts)
}

// IsRTL reports whether a string should be laid out right to left, decided by
// its first strongly directional character.
//
// Fyne does the real work: it shapes Arabic joining forms and applies the
// Unicode Bidirectional Algorithm through go-text's segmenter, so message text
// is passed to it untouched. Earlier this app pre-shaped the text itself and
// pre-reversed it, which fought Fyne's own pass and scrambled the result.
func IsRTL(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x0590 && r <= 0x05FF, // Hebrew
			r >= 0x0600 && r <= 0x08FF, // Arabic, Syriac, Thaana
			r >= 0xFB1D && r <= 0xFEFC: // Hebrew and Arabic presentation forms
			return true
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
			return false
		}
	}
	return false
}

// AlignFor returns the alignment a body of text should use: right for
// right-to-left text, left otherwise, so a Persian message sits on the right as
// its sender wrote it.
func AlignFor(s string) fyne.TextAlign {
	if IsRTL(s) {
		return fyne.TextAlignTrailing
	}
	return fyne.TextAlignLeading
}

// displayNumber makes a sender readable. A message with no sender shows a
// placeholder rather than an empty gap.
func displayNumber(number string) string {
	if strings.TrimSpace(number) == "" {
		return "(no sender)"
	}
	return number
}

// SpacerBox expands to fill the free space in a row, pushing later siblings to
// the far end.
func SpacerBox() fyne.CanvasObject {
	return canvas.NewRectangle(color.Transparent)
}

// Divider is a thin rule used between list rows.
func Divider() fyne.CanvasObject {
	r := canvas.NewRectangle(theme.Border)
	r.Resize(fyne.NewSize(0, 1))
	return r
}

// Chip is a small filled label used for status badges.
func Chip(text string, bg, fg color.NRGBA, fonts theme.Fonts) fyne.CanvasObject {
	t := Label(text, fg, 12, fonts)
	r := canvas.NewRectangle(bg)
	r.CornerRadius = 6
	return container.NewStack(r, container.NewPadded(t))
}

// Elide shortens s to n runes, marking the cut with an ellipsis. Newlines
// become spaces so a multi-line body still fits on one line.
func Elide(s string, n int) string { return ElideOnRunes(s, n) }

// ElideOnRunes shortens s to at most n runes, appending an ellipsis when it
// had to cut. Working in runes rather than bytes matters for Persian, where one
// character is two bytes.
func ElideOnRunes(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " "))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// ElideForPreview shortens a message body for the one-line list preview,
// cutting at a word boundary so the reader never sees half a word. Persian
// letters join across a cut, which leaves a dangling joining form, so this
// avoids cutting inside a word.
func ElideForPreview(s string, n int) string {
	flat := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " "))
	if len([]rune(flat)) <= n {
		return flat
	}
	r := []rune(flat)
	// Walk back from the limit to the nearest space.
	for i := n; i > 0; i-- {
		if r[i] == ' ' {
			return strings.TrimSpace(string(r[:i])) + "…"
		}
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// Match reports whether query appears in either field. An empty query matches
// everything.
func Match(query, sender, body string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(sender), q) ||
		strings.Contains(strings.ToLower(body), q)
}
