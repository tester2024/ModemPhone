package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	ftheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"modemphone/internal/router"
	"modemphone/internal/ui/theme"
)

// USSDPanel drives USSD codes against the modem.
//
// USSD replies are often interactive menus, so the panel keeps the last
// exchange on screen and offers the network's options as buttons.
type USSDPanel struct {
	Root *fyne.Container

	fonts    theme.Fonts
	code     *widget.Entry
	send     *widget.Button
	cancel   *widget.Button
	output   *widget.Label
	optsBox  *fyne.Container
	status   *widget.Label
	hint     fyne.CanvasObject
	hintRow  *fyne.Container
	busy     bool
	onSend   func(code string) (router.USSDResponse, error)
	onChoose func(choice string) (router.USSDResponse, error)
	onCancel func() error
}

// NewUSSDPanel builds the panel.
func NewUSSDPanel(fonts theme.Fonts,
	onSend func(string) (router.USSDResponse, error),
	onChoose func(string) (router.USSDResponse, error),
	onCancel func() error) *USSDPanel {

	p := &USSDPanel{
		fonts:    fonts,
		onSend:   onSend,
		onChoose: onChoose,
		onCancel: onCancel,
	}

	p.code = widget.NewEntry()
	p.code.SetPlaceHolder("*100#")
	p.code.OnSubmitted = func(string) { p.submit() }

	p.send = widget.NewButtonWithIcon("Send", ftheme.ConfirmIcon(), p.submit)
	p.send.Importance = widget.HighImportance
	p.cancel = widget.NewButtonWithIcon("Cancel", ftheme.CancelIcon(), p.cancelSession)

	p.output = widget.NewLabel("")
	p.output.Wrapping = fyne.TextWrapWord
	p.output.Importance = widget.HighImportance

	p.optsBox = container.NewVBox()
	p.status = widget.NewLabel("")
	p.status.Importance = widget.MediumImportance

	// The code entry takes the space left over from the buttons. In a
	// horizontal box every child is sized to its own minimum, which left the
	// input about forty pixels wide and impossible to use.
	inputRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(p.send, p.cancel), p.code)

	// The suggested codes are shortcuts, so they are shown as chips that run the
	// same path as typing one.
	p.hint = uiLabel("Try a code")
	p.hintRow = container.NewHBox()
	for _, s := range USSDHelp {
		code := s.Code
		b := widget.NewButton(code, func() { p.run(code) })
		b.Importance = widget.LowImportance
		p.hintRow.Add(b)
	}

	// A border layout is used for the pane so the output takes the remaining
	// height; a box would size it to its minimum.
	p.Root = container.NewBorder(
		container.NewVBox(
			container.NewPadded(inputRow),
			container.NewPadded(container.NewVBox(p.hint, p.hintRow)),
			uiDivider(),
		),
		container.NewPadded(p.status),
		nil, nil,
		container.NewPadded(container.NewVScroll(
			container.NewVBox(p.output, p.optsBox),
		)),
	)
	return p
}

// run submits a code, from the field or from one of the suggested chips.
func (p *USSDPanel) run(code string) {
	if p.busy || p.onSend == nil {
		return
	}
	code = strings.TrimSpace(code)
	if code == "" {
		p.setStatus("Enter a USSD code such as *100#", widget.DangerImportance)
		return
	}
	p.code.SetText(code)
	p.setBusy(true)
	resp, err := p.onSend(code)
	p.setBusy(false)
	if err != nil {
		p.setStatus(err.Error(), widget.DangerImportance)
		return
	}
	p.show(resp)
	p.setStatus("Code sent", widget.MediumImportance)
}

func (p *USSDPanel) submit() { p.run(p.code.Text) }

func (p *USSDPanel) cancelSession() {
	if p.onCancel == nil {
		return
	}
	if err := p.onCancel(); err != nil {
		p.setStatus(err.Error(), widget.DangerImportance)
		return
	}
	p.showOptions(nil)
	p.setStatus("Session cancelled", widget.MediumImportance)
}

func (p *USSDPanel) choose(opt string) {
	if p.busy || p.onChoose == nil {
		return
	}
	p.setBusy(true)
	resp, err := p.onChoose(opt)
	p.setBusy(false)
	if err != nil {
		p.setStatus(err.Error(), widget.DangerImportance)
		return
	}
	p.show(resp)
}

func (p *USSDPanel) show(resp router.USSDResponse) {
	text := strings.TrimSpace(resp.Text)
	if text == "" {
		switch resp.State {
		case router.USSDResult:
			text = "(no reply)"
		case router.USSDMenu:
			text = "Choose an option:"
		default:
			text = "(no reply)"
		}
	}
	p.output.SetText(text)
	p.output.Refresh()
	p.showOptions(resp.Options)
}

func (p *USSDPanel) showOptions(opts []string) {
	p.optsBox.Objects = nil
	for i, o := range opts {
		opt := o
		b := widget.NewButton(Elide(opt, 60), func() { p.choose(opt) })
		b.Alignment = widget.ButtonAlignLeading
		if i == 0 {
			b.Importance = widget.HighImportance
		} else {
			b.Importance = widget.MediumImportance
		}
		p.optsBox.Add(b)
	}
	p.optsBox.Refresh()
}

func (p *USSDPanel) setStatus(s string, imp widget.Importance) {
	p.status.SetText(s)
	p.status.Importance = imp
	p.status.Refresh()
}

func (p *USSDPanel) setBusy(b bool) {
	p.busy = b
	if b {
		p.send.Disable()
		p.cancel.Disable()
		return
	}
	p.send.Enable()
	p.cancel.Enable()
}

// USSDHelp lists the codes worth trying on this network.
var USSDHelp = []struct{ Code, Note string }{
	{"*100#", "balance and validity"},
	{"*610#", "data and minutes"},
	{"*200#", "add a package"},
	{"*140#", "account services"},
}

// uiLabel is a plain caption used inside this package.
func uiLabel(s string) fyne.CanvasObject {
	l := widget.NewLabel(s)
	l.Importance = widget.MediumImportance
	return l
}

// uiDivider is a thin rule between the input area and the output.
func uiDivider() fyne.CanvasObject { return Divider() }
