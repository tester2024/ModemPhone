package ui

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	ftheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"modemphone/internal/router"
	"modemphone/internal/ui/theme"
)

// SendIcon is the glyph on the send button.
func SendIcon() fyne.Resource { return ftheme.MailComposeIcon() }

// Composer is the send form.
//
// The modem accepts digits and semicolons only in the recipient field, so
// entry is filtered as the user types rather than being validated after a
// round trip to the hardware.
type Composer struct {
	Root *fyne.Container

	fonts       theme.Fonts
	country     *widget.Entry
	recipients  *widget.Entry
	body        *widget.Entry
	send        *widget.Button
	status      *widget.Label
	counter     *widget.Label
	onSend      func(cc, number, body string) error
	countryCode string
}

// NewComposer builds the form. onSend is called with the composed values and
// may return an error to show inline.
func NewComposer(fonts theme.Fonts, countryCode, defaultNumber string, onSend func(cc, number, body string) error) *Composer {
	c := &Composer{
		fonts:       fonts,
		countryCode: countryCode,
		onSend:      onSend,
	}

	c.country = widget.NewEntry()
	c.country.Text = countryCode
	c.country.SetPlaceHolder("+98")
	c.country.OnChanged = func(s string) { c.countryCode = sanitizeCountry(s) }

	c.recipients = widget.NewEntry()
	c.recipients.Text = defaultNumber
	c.recipients.SetPlaceHolder("09101234567 or 0910;0911 for a group")
	c.recipients.OnChanged = func(s string) {
		clean := sanitizeNumbers(s)
		if clean != s {
			c.recipients.Text = clean
		}
	}

	c.body = widget.NewMultiLineEntry()
	c.body.SetPlaceHolder("Message")
	c.body.Wrapping = fyne.TextWrapWord
	c.body.OnChanged = func(string) { c.updateCounter() }

	c.send = widget.NewButtonWithIcon("Send", SendIcon(), c.submit)
	c.send.Importance = widget.HighImportance

	c.status = widget.NewLabel("")
	c.status.Importance = widget.MediumImportance
	c.counter = widget.NewLabel("")
	c.counter.Importance = widget.MediumImportance

	c.updateCounter()
	c.updateSendEnabled()

	// The message field takes the remaining height so a long draft is visible
	// while it is written.
	c.Root = container.NewBorder(
		container.NewPadded(container.NewVBox(
			rowLabelled("Country code", c.country, fonts),
			rowLabelled("To", c.recipients, fonts),
		)),
		container.NewPadded(container.NewHBox(c.send, c.counter, c.status)),
		nil, nil,
		container.NewPadded(container.NewVBox(
			rowLabelled("Message", c.body, fonts),
		)),
	)
	return c
}

// SetCountryCode updates the default and the visible field.
func (c *Composer) SetCountryCode(cc string) {
	c.countryCode = cc
	c.country.Text = cc
}

// SetRecipients replaces the recipient field, used when replying.
func (c *Composer) SetRecipients(n string) {
	c.recipients.Text = sanitizeNumbers(n)
}

func (c *Composer) updateCounter() {
	n := len([]rune(c.body.Text))
	limit := router.MaxBodyRunes
	c.counter.SetText(plural(n, "character") + "/ " + strconv.Itoa(limit))
	if n > limit {
		c.counter.Importance = widget.WarningImportance
	} else {
		c.counter.Importance = widget.MediumImportance
	}
	c.updateSendEnabled()
}

func (c *Composer) updateSendEnabled() {
	ok := strings.TrimSpace(c.body.Text) != "" &&
		strings.TrimSpace(c.recipients.Text) != "" &&
		len([]rune(c.body.Text)) <= router.MaxBodyRunes
	if ok {
		c.send.Enable()
	} else {
		c.send.Disable()
	}
}

func (c *Composer) submit() {
	if c.onSend == nil {
		return
	}
	cc := sanitizeCountry(c.country.Text)
	if cc == "" {
		cc = c.countryCode
	}
	number := sanitizeNumbers(c.recipients.Text)
	text := strings.TrimSpace(c.body.Text)

	if number == "" {
		c.showError("Enter a recipient number")
		return
	}
	if text == "" {
		c.showError("Enter a message")
		return
	}
	if len([]rune(text)) > router.MaxBodyRunes {
		c.showError("Message is longer than the modem accepts")
		return
	}

	c.send.Disable()
	c.status.SetText("Sending…")

	// The send blocks while the modem is asked whether it recorded the message,
	// so it runs on a worker goroutine and the result comes back to the UI.
	go func() {
		err := c.onSend(cc, number, text)
		if err != nil {
			c.showError(err.Error())
		} else {
			c.status.SetText("Sent")
		}
		c.send.Enable()
	}()

	// The body is cleared straight away so the next message can be typed while
	// this one is on its way.
	c.body.SetText("")
	c.updateSendEnabled()
}

func (c *Composer) showError(msg string) {
	c.status.SetText(msg)
	c.status.Importance = widget.DangerImportance
	c.status.Refresh()
}

// sanitizeCountry keeps a leading plus and digits, matching what the modem
// accepts in this field.
func sanitizeCountry(s string) string {
	var b strings.Builder
	for i, r := range strings.TrimSpace(s) {
		if r == '+' && i == 0 {
			b.WriteRune(r)
			continue
		}
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sanitizeNumbers keeps digits and the semicolon the modem uses to separate
// bulk recipients.
func sanitizeNumbers(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ';':
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), ";")
}

// rowLabelled puts a muted caption above a field.
func rowLabelled(caption string, field fyne.CanvasObject, fonts theme.Fonts) fyne.CanvasObject {
	return container.NewVBox(Muted(caption, 12, fonts), field)
}
