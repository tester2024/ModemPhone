package ui

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"modemphone/internal/router"
	"modemphone/internal/ui/theme"
)

// MsgList shows one folder of messages.
//
// Fyne's List widget is used so rows are virtualised: only the visible rows
// exist, which keeps scrolling smooth and the cost independent of how many
// messages the modem is holding.
type MsgList struct {
	Root *fyne.Container

	fonts   theme.Fonts
	all     []router.Message
	shown   []router.Message
	list    *widget.List
	summary *widget.Label
	query   string
	onOpen  func(router.Message)
	// showUnread drives the dot in the first column. It is on for the inbox,
	// where an unread message is the useful signal, and off for the outbox and
	// drafts, where a message has already been read by being there.
	showUnread bool
	// role names the folder in the row, so the outbox reads "To 0912…" rather
	// than showing a recipient where an inbox shows a sender. It is empty for
	// the inbox.
	role string
	// showStatus puts the delivery state where the inbox would show a time:
	// the modem records no timestamp for a message it has already sent.
	showStatus bool

	// parts maps each row object to the widgets it owns.
	parts map[fyne.CanvasObject]*rowParts
}

// NewMsgList builds an empty list. onOpen fires when a row is activated.
//
// showUnread drives the unread dot. role and showStatus describe the folder:
// the inbox has neither, the outbox says who a message was sent to and whether
// it went, and drafts say neither.
func NewMsgList(fonts theme.Fonts, onOpen func(router.Message), showUnread bool, role string, showStatus bool) *MsgList {
	m := &MsgList{
		fonts:      fonts,
		onOpen:     onOpen,
		showUnread: showUnread,
		role:       role,
		showStatus: showStatus,
		parts:      map[fyne.CanvasObject]*rowParts{},
	}
	m.summary = widget.NewLabel("")
	m.summary.Importance = widget.MediumImportance

	m.list = widget.NewList(
		func() int { return len(m.shown) },
		m.createRow,
		m.updateRow,
	)
	m.list.OnSelected = func(id widget.ListItemID) {
		if id < 0 || int(id) >= len(m.shown) {
			return
		}
		// The selection is deliberately left in place. Calling Unselect from
		// inside this callback re-enters the list while it is still handling
		// the tap, which swallowed the click on every row but the first.
		if m.onOpen != nil {
			m.onOpen(m.shown[id])
		}
	}

	m.Root = container.NewBorder(m.summary, nil, nil, nil, m.list)
	return m
}

// SetMessages replaces the folder's contents and reapplies the filter.
func (m *MsgList) SetMessages(msgs []router.Message) {
	m.all = msgs
	m.applyFilter()
}

// SetQuery filters the visible rows. Changing the filter does return the list
// to the top, since the row the user was looking at is usually gone.
func (m *MsgList) SetQuery(q string) {
	m.query = q
	m.applyFilter()
	m.list.ScrollToTop()
}

// MarkShown flags a message as read in the list, so its dot clears straight
// away instead of waiting for the next poll.
func (m *MsgList) MarkShown(index string) {
	found := false
	for i := range m.all {
		if m.all[i].Index == index {
			m.all[i].Read = true
			found = true
			break
		}
	}
	if !found {
		return
	}
	// The unread tally in the summary has to be recomputed, but the rows
	// themselves are refreshed in place to keep the scroll position.
	unread := 0
	for _, msg := range m.all {
		if !msg.Read {
			unread++
		}
	}
	m.summary.SetText(m.summaryFor(unread))
	m.summary.Refresh()
	m.list.Refresh()
}

// summaryFor builds the count line, given the number of unread messages.
func (m *MsgList) summaryFor(unread int) string {
	label := plural(len(m.shown), "message")
	if unread > 0 {
		label += " · " + plural(unread, "unread")
	}
	if m.query != "" {
		label += " · filtered"
	}
	return label
}

// rowLabel builds the leading text of a row. The inbox shows who wrote it; the
// outbox shows who it was sent to, which is the useful fact there.
func (m *MsgList) rowLabel(msg router.Message) string {
	n := displayNumber(msg.Number)
	if m.role != "" {
		return m.role + " " + n
	}
	return n
}

// DeliveryState turns the modem's numeric status into words.
//
// The firmware reuses the field for meaning it does not document, so unknown
// values are left blank rather than guessed at. An empty string is not a bug: it
// means this build reported something the app does not recognise.
func DeliveryState(status string) string {
	switch strings.TrimSpace(status) {
	case "2":
		return "Sending"
	case "3":
		return "Sent"
	case "4":
		return "Failed"
	}
	return ""
}

// shownCount reports how many rows are currently visible, which the tests use
// to check filtering.
func (m *MsgList) shownCount() int { return len(m.shown) }

func (m *MsgList) applyFilter() {
	m.shown = m.shown[:0]
	unread := 0
	for _, msg := range m.all {
		if !msg.Read {
			unread++
		}
		if Match(m.query, msg.Number, msg.Content) {
			m.shown = append(m.shown, msg)
		}
	}

	m.summary.SetText(m.summaryFor(unread))
	m.summary.Refresh()
	// The scroll position and the selection are deliberately left alone. This
	// runs on every poll, and jumping to the top each time threw away where the
	// user had scrolled to and dropped the row they were about to click.
	m.list.Refresh()
}

// rowParts are the widgets making up one row, so the update callback can reach
// them without searching the tree.
type rowParts struct {
	dot     *canvas.Rectangle
	sender  *canvas.Text
	when    *canvas.Text
	preview *canvas.Text
}

// previewSize is the font size of the one-line preview. Persian needs slightly
// more room than Latin: at 12 points the strokes of Vazirmatn's Regular weight
// render thin on a non-HiDPI screen.
const previewSize = 14

// createRow builds a row. The dot marks unread messages, the sender and time
// share the first line, and a single elided preview sits below.
func (m *MsgList) createRow() fyne.CanvasObject {
	dot := canvas.NewRectangle(theme.TextFaint)
	dot.CornerRadius = 3
	dot.Resize(fyne.NewSize(6, 6))

	sender := Bold("", theme.Text, 14, m.fonts)
	when := Muted("", 12, m.fonts)
	// The preview uses the Medium face at a larger size than the rest of the
	// secondary text: Persian strokes are thin at small sizes in the Regular
	// weight, and message bodies are the thing the app exists to show.
	preview := Label("", theme.Text, previewSize, fontsMedium(m.fonts))
	// Only real objects go in the variadic list: Fyne's border layout
	// dereferences every entry, so a nil here would panic when the row is
	// measured.
	header := container.NewBorder(nil, nil,
		container.NewHBox(dot, sender),
		nil, when,
	)
	row := container.NewVBox(header, preview)

	// The parts are attached to the row itself, so the update callback can
	// reach them without a side table: Fyne reuses a row object, and each one
	// owns its widgets for as long as the list lives.
	m.parts[row] = &rowParts{dot: dot, sender: sender, when: when, preview: preview}
	return row
}

// remember stores the widgets belonging to a row.
func (m *MsgList) remember(row fyne.CanvasObject, p *rowParts) { m.parts[row] = p }

func (m *MsgList) updateRow(id widget.ListItemID, obj fyne.CanvasObject) {
	if id < 0 || int(id) >= len(m.shown) {
		return
	}
	msg := m.shown[id]
	p := m.parts[obj]
	if p == nil {
		// A row that Fyne created without going through createRow; build its
		// parts now so the update is not silently dropped.
		m.remember(obj, &rowParts{})
		p = m.parts[obj]
	}
	if p.sender != nil {
		p.sender.Text = ElideOnRunes(m.rowLabel(msg), 30)
		p.sender.Refresh()
	}
	if p.when != nil {
		if m.showStatus {
			p.when.Text = DeliveryState(msg.Status)
		} else {
			p.when.Text = msg.Time
		}
		p.when.Refresh()
	}
	if p.preview != nil {
		// Elide on a word boundary before shaping, so a cut never leaves a
		// half-joined Persian letter.
		p.preview.Text = ElideForPreview(msg.Content, 72)
		p.preview.Refresh()
	}
	// The row is refreshed as a whole so a container holding these widgets
	// re-measures and re-lays out its children.
	if c, ok := obj.(*fyne.Container); ok {
		c.Refresh()
	}
	if p.dot != nil {
		switch {
		case !m.showUnread:
			// Nothing to show: the dot would only say what the folder already says.
			p.dot.Hide()
		case msg.Read:
			p.dot.FillColor = theme.TextFaint
			p.dot.Show()
		default:
			p.dot.FillColor = theme.Accent
			p.dot.Show()
		}
		p.dot.Refresh()
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
