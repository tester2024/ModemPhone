// Package app wires the router client to the user interface.
//
// Everything that touches the network runs on a worker goroutine so the window
// never blocks. The firmware's session expires after 170 seconds and a page load
// can take a moment, and a UI that froze on either would be unusable.
//
// The code is split by concern: this file holds the application itself and its
// lifecycle, window.go the window, tray and keyboard handling, actions.go the
// message and settings actions, and poll.go the background refresh.
package app

import (
	"log"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	ftheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"modemphone/internal/config"
	"modemphone/internal/router"
	"modemphone/internal/ui"
	"modemphone/internal/ui/theme"
)

// App is the main window.
type App struct {
	win fyne.Window
	cfg config.Config
	// client talks to the modem. Everything here is safe for concurrent use.
	client *router.Client
	fonts  theme.Fonts

	bar      *ui.StatusBar
	tabs     *container.AppTabs
	inboxTab *container.TabItem
	inbox    *ui.MsgList
	outbox   *ui.MsgList
	drafts   *ui.MsgList
	viewer   *ui.MessageViewer
	composer *ui.Composer
	ussd     *ui.USSDPanel
	status   *widget.Label

	// search is the filter field the Ctrl+F shortcut focuses. There is one field
	// per message pane; this is whichever was created last.
	search *widget.Entry
	// filters holds every per-pane filter field, which all share one query.
	filters []*widget.Entry
	// query is the filter text shared by every folder.
	query string
	// syncingFilter stops the mirror write across fields re-entering setQuery.
	syncingFilter bool

	// shownUnread is the count currently drawn on the inbox tab, so the badge is
	// only rewritten when it actually changes.
	shownUnread int
	// failures counts consecutive failed polls, which drives the backoff and the
	// "modem unreachable" message.
	failures int
	// notified records which messages have already been announced, and
	// sawFirstInbox marks the first pass, so starting the app does not announce
	// the whole existing backlog at once.
	notified      map[string]bool
	sawFirstInbox bool
	// device and lastStatus are the last values read, for the diagnostics block.
	device     router.DeviceInfo
	lastStatus router.Status

	mu sync.Mutex
	// inFlight keeps two refreshes from running at once, and closing stops both
	// the poll loop and any work already under way.
	inFlight bool
	closing  bool
}

// New builds the application window.
func New(win fyne.Window, cfg config.Config, fonts theme.Fonts) *App {
	a := &App{
		win:      win,
		cfg:      cfg,
		fonts:    fonts,
		client:   router.New(cfg.Host, cfg.Username, cfg.Password),
		notified: map[string]bool{},
	}
	a.build()
	return a
}

// build assembles the window's contents.
func (a *App) build() {
	a.bar = ui.NewStatusBar(a.fonts)

	a.inbox = ui.NewMsgList(a.fonts, a.openMessage, true, "", false)
	a.outbox = ui.NewMsgList(a.fonts, a.openMessage, false, "To", true)
	a.drafts = ui.NewMsgList(a.fonts, a.openMessage, false, "", false)
	a.viewer = ui.NewMessageViewer(a.win, a.fonts, a.replyTo, a.deleteMessage, a.numberCopied)
	a.composer = ui.NewComposer(a.fonts, a.cfg.CountryCode, a.cfg.DefaultNumber, a.sendMessage)
	a.ussd = ui.NewUSSDPanel(a.fonts, a.sendUSSD, a.chooseUSSD, a.cancelUSSD)

	a.status = widget.NewLabel("")
	a.status.Importance = widget.MediumImportance

	// The list is given most of the width; the reader is a detail pane beside
	// it, and the split can be dragged to change the proportion.
	split := container.NewHSplit(a.inbox.Root, a.viewer.Root)
	split.Offset = 0.55

	// A border layout is used rather than a vertical box because a box sizes its
	// children to their minimum height: the list would collapse to a couple of
	// rows and leave the rest of the pane empty.
	messages := container.NewBorder(
		container.NewVBox(a.newFilterRow(), ui.Divider()),
		a.status,
		nil, nil,
		split,
	)

	// Outbox and drafts get their own toolbar, so each folder is usable on its
	// own.
	outboxPane := container.NewBorder(
		container.NewVBox(a.newFilterRow(), ui.Divider()),
		a.status, nil, nil,
		a.outbox.Root,
	)
	draftsPane := container.NewBorder(
		container.NewVBox(a.newFilterRow(), ui.Divider()),
		a.status, nil, nil,
		a.drafts.Root,
	)

	a.buildTabs(messages, outboxPane, draftsPane)
	a.win.SetContent(container.NewBorder(a.bar.Root, nil, nil, nil, a.tabs))
	a.win.Resize(fyne.NewSize(float32(a.cfg.WindowWidth), float32(a.cfg.WindowHeight)))
}

// buildTabs creates the tab bar. Each pane arrives already laid out, with its
// own toolbar and content, and with a gap under the bar.
func (a *App) buildTabs(messages, outboxPane, draftsPane fyne.CanvasObject) {
	a.inboxTab = container.NewTabItemWithIcon("Inbox", ftheme.DownloadIcon(), withTabGap(messages))
	a.tabs = container.NewAppTabs(
		a.inboxTab,
		container.NewTabItemWithIcon("Outbox", ftheme.UploadIcon(), withTabGap(outboxPane)),
		container.NewTabItemWithIcon("Drafts", ftheme.DocumentCreateIcon(), withTabGap(draftsPane)),
		container.NewTabItemWithIcon("New message", ftheme.MailComposeIcon(), withTabGap(a.composer.Root)),
		container.NewTabItemWithIcon("USSD", ftheme.ComputerIcon(), withTabGap(a.ussd.Root)),
		container.NewTabItemWithIcon("Settings", ftheme.SettingsIcon(), withTabGap(a.settingsPane())),
	)
}

// numberCopied confirms a copy in the status line, so the click has a visible
// result.
func (a *App) numberCopied(number string) {
	a.setStatus("Copied " + number + " to the clipboard")
}

// Start performs the first load and begins polling. If startTab names a tab,
// that tab is shown first.
//
// Start must run before the event loop, and after EnableTray, so the tray is
// registered in time for its message loop to be started.
func (a *App) Start(startTab string) {
	if i := a.tabIndex(startTab); i >= 0 {
		a.tabs.SelectIndex(i)
	}
	a.installShortcuts()

	// Without a saved password the modem cannot be reached, so there is no
	// point polling; the app waits on the settings form instead.
	if !a.cfg.Ready() {
		a.tabs.SelectIndex(a.tabIndex("Settings"))
		a.bar.SetIdle("Not connected")
		a.setStatus("Enter the modem password in Settings, then save")
		return
	}
	a.refreshAll()
	go a.pollLoop()
}

// Close stops background work and saves settings.
func (a *App) Close() {
	a.mu.Lock()
	a.closing = true
	a.mu.Unlock()
	a.rememberGeometry()
	if err := config.Save(a.cfg); err != nil {
		log.Println("saving settings:", err)
	}
}

// rememberGeometry stores the window size so the app reopens at the size it was
// left at, rather than snapping back to the default every launch.
func (a *App) rememberGeometry() {
	w, h := a.win.Content().Size().Width, a.win.Content().Size().Height
	if w >= 640 && h >= 480 {
		a.cfg.WindowWidth = int(w)
		a.cfg.WindowHeight = int(h)
	}
}

// setStatus writes a one-line message under the panes, for errors and
// confirmations.
func (a *App) setStatus(s string) {
	if a.status != nil && s != "" {
		a.status.SetText(s)
	}
}

// notifyNew announces messages that were not present the last time the inbox was
// read. The first pass only records what is already there, so starting the app
// does not fire a burst of notifications for the existing backlog.
func (a *App) notifyNew(msgs []router.Message) {
	if !a.cfg.NotifyOnSMS {
		return
	}
	first := !a.sawFirstInbox
	for _, m := range msgs {
		if a.notified[m.Index] {
			continue
		}
		a.notified[m.Index] = true
		if !first {
			a.notify(m)
		}
	}
	a.sawFirstInbox = true
}

// notify raises a desktop notification for a newly arrived message.
func (a *App) notify(m router.Message) {
	fyne.CurrentApp().SendNotification(fyne.NewNotification(
		"New message from "+displaySender(m.Number),
		ui.Elide(firstLine(m.Content), 140),
	))
}

// displaySender makes a sender readable in a notification title.
func displaySender(number string) string {
	if strings.TrimSpace(number) == "" {
		return "(no sender)"
	}
	return number
}

// firstLine returns the first line of a message body, which is what fits in a
// notification.
func firstLine(body string) string {
	if i := strings.IndexAny(body, "\r\n"); i >= 0 {
		return strings.TrimSpace(body[:i])
	}
	return strings.TrimSpace(body)
}
