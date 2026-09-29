package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	ftheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"modemphone/internal/router"
	"modemphone/internal/ui/theme"
)

// MessageViewer shows one message in full, with reply and delete actions.
type MessageViewer struct {
	Root *fyne.Container

	fonts theme.Fonts
	win   fyne.Window
	// sender is the sender line; the numbers found in the body are offered as
	// their own copyable controls below it.
	sender       *canvas.Text
	numbersBox   *fyne.Container
	numbers      []string
	when         *widget.Label
	body         *widget.Entry
	replyBtn     *widget.Button
	copyBtn      *widget.Button
	delBtn       *widget.Button
	placeholder  *widget.Label
	senderNumber string

	// currentIndex is the modem's id for the message on screen, needed by the
	// delete action.
	currentIndex string
	folder       router.Folder

	onReply  func(string)
	onDelete func(router.Folder, string)
	onCopied func(string)
}

// NewMessageViewer builds an empty viewer. onReply receives the sender's
// number; onDelete receives the folder and the message index; onCopied fires
// when a sender is copied to the clipboard.
//
// win is used for the clipboard, which is reached through the window.
func NewMessageViewer(win fyne.Window, fonts theme.Fonts,
	onReply func(string), onDelete func(router.Folder, string), onCopied func(string)) *MessageViewer {

	v := &MessageViewer{
		fonts:    fonts,
		win:      win,
		onReply:  onReply,
		onDelete: onDelete,
		onCopied: onCopied,
	}
	v.when = widget.NewLabel("")
	v.when.Importance = widget.MediumImportance

	// The body is a read-only multi-line entry rather than a label, because an
	// entry supports selecting text with the mouse along with Ctrl+A and Ctrl+C.
	// Only editing is gated on Disabled, so the copy shortcuts still work while
	// the message itself cannot be typed into.
	v.body = widget.NewMultiLineEntry()
	v.body.Wrapping = fyne.TextWrapWord
	v.body.Disable()
	v.body.OnChanged = nil

	// Button captions stay short: a long caption such as "Reply to <number>"
	// would give this pane a large minimum width and squeeze the message list
	// when the split is laid out.
	v.replyBtn = widget.NewButtonWithIcon("Reply", ftheme.MailComposeIcon(), v.doReply)
	v.delBtn = widget.NewButtonWithIcon("Delete", ftheme.DeleteIcon(), v.doDelete)
	v.delBtn.Importance = widget.DangerImportance

	// One click copies the whole message, for the times when selecting the
	// text by hand is more trouble than it is worth.
	v.copyBtn = widget.NewButtonWithIcon("Copy all", ftheme.ContentCopyIcon(), v.doCopyAll)
	v.copyBtn.Hide()

	// The reply and delete actions are hidden until a message is open, so the
	// empty state does not show buttons that would do nothing.
	v.replyBtn.Hide()
	v.delBtn.Hide()

	v.placeholder = widget.NewLabel("Select a message to read it here.")
	v.placeholder.Importance = widget.MediumImportance

	v.sender = Bold("", theme.Text, 15, fonts)
	v.numbersBox = container.NewVBox()
	v.numbersBox.Hide()

	// The body scrolls so a long message does not push the actions off screen,
	// and the whole pane is padded away from the split handle beside it.
	v.Root = container.NewBorder(
		container.NewVBox(
			container.NewPadded(container.NewHBox(v.sender, SpacerBox(), v.when)),
			Divider(),
		),
		container.NewVBox(
			v.numbersBox,
			container.NewPadded(container.NewHBox(v.replyBtn, v.copyBtn, v.delBtn)),
		),
		nil, nil,
		container.NewStack(
			container.NewPadded(v.placeholder),
			container.NewPadded(container.NewVScroll(v.body)),
		),
	)
	v.syncVisibility()
	return v
}

// syncVisibility shows the empty state until a message is loaded, and the
// message and its actions afterwards.
// syncVisibility shows the empty state until a message is loaded, and the
// message and its actions afterwards.
// syncVisibility shows the empty state until a message is loaded, and the
// message and its actions afterwards.
//
// Both children of the stack are toggled: the body is a disabled entry, which
// paints an opaque background, so leaving it visible would hide the
// placeholder behind it.
func (v *MessageViewer) syncVisibility() {
	empty := v.currentIndex == ""
	if empty {
		v.placeholder.Show()
		v.body.Hide()
		v.replyBtn.Hide()
		v.copyBtn.Hide()
		v.delBtn.Hide()
		return
	}
	v.placeholder.Hide()
	v.body.Show()
	v.replyBtn.Show()
	v.copyBtn.Show()
	v.delBtn.Show()
}

// doCopyAll puts the whole message body on the clipboard, which is the common
// case when a message is mostly one number or one link.
func (v *MessageViewer) doCopyAll() {
	if v.currentIndex == "" || v.body == nil {
		return
	}
	if Copy(v.win, v.body.Text) && v.onCopied != nil {
		v.onCopied(v.body.Text)
	}
}

// Show displays a message from the given folder.
func (v *MessageViewer) Show(f router.Folder, m router.Message) {
	v.folder = f
	v.currentIndex = m.Index
	v.when.SetText(m.Time)
	v.body.SetText(m.Content)
	v.body.Refresh()
	v.refreshSender(m.Number)
	v.refreshNumbers(m.Content)
	v.syncVisibility()

	// The pane is refreshed as a whole: showing the message and hiding the
	// placeholder changes which child of the stack is visible, and the
	// container must re-measure for the text to appear at its new size.
	v.Root.Refresh()
}

// refreshSender shows the sender as plain text. Copying the number that opened
// a message is handled by the number chips under the body, so the sender itself
// is not a control.
func (v *MessageViewer) refreshSender(number string) {
	if v.sender == nil {
		return
	}
	v.senderNumber = number
	v.sender.Text = ElideOnRunes(displayNumber(number), 30)
	v.sender.Refresh()
}

// refreshNumbers rebuilds the row of copyable numbers found in the body.
func (v *MessageViewer) refreshNumbers(body string) {
	if v.numbersBox == nil {
		return
	}
	v.numbers = ExtractNumbers(body)
	if chips := CopyChips(v.numbers, v.win, v.onCopied); chips != nil {
		v.numbersBox.Objects = []fyne.CanvasObject{chips}
		v.numbersBox.Show()
	} else {
		v.numbersBox.Objects = nil
		v.numbersBox.Hide()
	}
	v.numbersBox.Refresh()
}

// ReplyNumber is the sender of the message on screen, or the empty string.
func (v *MessageViewer) ReplyNumber() string {
	if v.currentIndex == "" {
		return ""
	}
	return v.senderNumber
}

func (v *MessageViewer) doReply() {
	if n := v.ReplyNumber(); n != "" && v.onReply != nil {
		v.onReply(n)
	}
}

func (v *MessageViewer) doDelete() {
	if v.onDelete != nil && v.currentIndex != "" {
		v.onDelete(v.folder, v.currentIndex)
	}
}
