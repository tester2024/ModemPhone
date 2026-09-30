// Package app wires the router client to the user interface.
//
// Everything that touches the network runs on a worker goroutine so the window
// never blocks. The firmware's session expires after 170 seconds and a page
// load can take a moment, and a UI that froze on either would be unusable.
package app

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	ftheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"modemphone/internal/applog"
	"modemphone/internal/buildinfo"
	"modemphone/internal/config"
	"modemphone/internal/router"
	"modemphone/internal/startup"
	"modemphone/internal/ui"
	"modemphone/internal/ui/theme"
)

// App is the main window.
type App struct {
	win    fyne.Window
	cfg    config.Config
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
	search   *widget.Entry
	// filters holds the per-pane filter fields, which all share one query.
	filters []*widget.Entry
	// query is the filter text shared by every folder.
	query string
	// syncingFilter stops the mirror write from re-entering setQuery.
	syncingFilter bool
	status        *widget.Label

	// shownUnread is the count currently drawn on the inbox tab, so the badge is
	// only rewritten when it actually changes.
	shownUnread int
	// failures counts consecutive failed polls, which drives the backoff and
	// the "modem unreachable" message.
	failures int
	// device is the last identity read, for the diagnostics panel.
	device router.DeviceInfo
	// lastStatus is the last connection state read, for diagnostics.
	lastStatus router.Status

	folder router.Folder

	mu           sync.Mutex
	inFlight     bool
	closing      bool
	notified     map[string]bool
	sawFirstPass bool
}

// New builds the application window.
func New(win fyne.Window, cfg config.Config, fonts theme.Fonts) *App {
	a := &App{
		win:      win,
		cfg:      cfg,
		fonts:    fonts,
		folder:   router.FolderInbox,
		notified: map[string]bool{},
	}
	a.client = router.New(cfg.Host, cfg.Username, cfg.Password)
	a.build()
	return a
}

func (a *App) build() {
	a.bar = ui.NewStatusBar(a.fonts)

	a.inbox = ui.NewMsgList(a.fonts, a.openMessage, true, "", false)
	a.outbox = ui.NewMsgList(a.fonts, a.openMessage, false, "To", true)
	a.drafts = ui.NewMsgList(a.fonts, a.openMessage, false, "To", true)
	a.viewer = ui.NewMessageViewer(a.win, a.fonts, a.replyTo, a.deleteMessage, a.numberCopied)
	a.composer = ui.NewComposer(a.fonts, a.cfg.CountryCode, a.cfg.DefaultNumber, a.sendMessage)
	a.ussd = ui.NewUSSDPanel(a.fonts, a.sendUSSD, a.chooseUSSD, a.cancelUSSD)

	a.status = widget.NewLabel("")
	a.status.Importance = widget.MediumImportance

	// The list is given most of the width; the reader is a detail pane beside
	// it, and the split can be dragged to change the proportion.
	split := container.NewHSplit(a.inbox.Root, a.viewer.Root)
	split.Offset = 0.55

	// A border layout is used rather than a vertical box because a box sizes
	// its children to their minimum height: the list would collapse to a couple
	// of rows and leave the rest of the pane empty.
	messages := container.NewBorder(
		container.NewVBox(a.newFilterRow(), ui.Divider()),
		a.status,
		nil, nil,
		split,
	)

	// Outbox and drafts get their own toolbar, so the folder is usable on its
	// own. Each pane needs its own entry: a Fyne object can belong to only
	// one parent, so sharing one across three panes quietly breaks the layout.
	outboxPane := container.NewBorder(
		container.NewVBox(a.newFilterRow(), ui.Divider()),
		a.status,
		nil, nil,
		a.outbox.Root,
	)

	draftsPane := container.NewBorder(
		container.NewVBox(a.newFilterRow(), ui.Divider()),
		a.status,
		nil, nil,
		a.drafts.Root,
	)

	a.buildTabs(messages, outboxPane, draftsPane)

	a.win.SetContent(container.NewBorder(a.bar.Root, nil, nil, nil, a.tabs))
	a.win.Resize(fyne.NewSize(float32(a.cfg.WindowWidth), float32(a.cfg.WindowHeight)))
}

// numberCopied confirms a copy in the status line, so the click has a visible
// result.
func (a *App) numberCopied(number string) {
	a.setStatus("Copied " + number + " to the clipboard")
}

// trayApp is the part of Fyne's application that owns the system tray. Those
// methods live on an unexported concrete type, so they are reached through this
// interface; on a build without tray support the assertion simply fails and the
// app carries on without a tray icon.
type trayApp interface {
	SetSystemTrayIcon(fyne.Resource)
	SetSystemTrayMenu(*fyne.Menu)
}

// trayIcon loads the tray icon. The Windows tray needs a raster image, so the
// bundled SVG icons Fyne ships will not do.
func trayIcon() fyne.Resource {
	raw, err := trayIconFS.ReadFile("assets/icons/tray.png")
	if err != nil {
		return nil
	}
	return fyne.NewStaticResource("tray.png", raw)
}

// installTray sets up the system tray, which is the only way back into the
// window once it has been closed to the background.
//
// The notification area is not ready until the event loop is running, so the
// install is retried for a few seconds. Failing that the app simply carries on
// without a tray icon rather than refusing to start.
func (a *App) installTray() {
	fa := fyne.CurrentApp()
	if fa == nil {
		return
	}
	tray, ok := fa.(trayApp)
	if !ok {
		return
	}

	a.setTrayMenu(tray, a.trayMenu())
}

// setTrayMenu installs the tray menu, exactly once.
//
// The driver starts the tray on the first call to SetSystemTrayMenu, and every
// later call resets the live menu and spawns another goroutine watching each
// item's click channel. Calling it repeatedly leaves a menu whose entries are
// wired to abandoned channels, so the buttons appear but do nothing. Once is
// correct.
//
// The icon needs no separate call: the driver falls back to the application's own
// icon when the tray starts, and that is set before the window is created.
func (a *App) setTrayMenu(tray trayApp, menu *fyne.Menu) {
	tray.SetSystemTrayMenu(menu)

	// Closing the window hides it and keeps polling, so a new message still
	// raises a notification. Quitting is done from the tray menu.
	a.win.SetCloseIntercept(func() { a.hide() })
}

// show brings the window back from the background.
func (a *App) show() {
	a.win.Show()
	a.win.RequestFocus()
}

// showTab opens the window on a named tab, which is what the tray's Inbox and
// New message entries do.
func (a *App) showTab(name string) {
	if i := a.tabIndex(name); i >= 0 {
		a.tabs.SelectIndex(i)
	}
	a.show()
}

// toggleStartAtLogin flips the autostart registration and reports the outcome.
func (a *App) toggleStartAtLogin() {
	want := !a.cfg.StartAtLogin
	now, err := startup.Set(want)
	a.cfg.StartAtLogin = now
	if err != nil {
		a.setStatus(err.Error())
	} else if now {
		a.setStatus("Added to Windows startup")
	} else {
		a.setStatus("Removed from Windows startup")
	}
	if err := config.Save(a.cfg); err != nil {
		log.Println("saving settings:", err)
	}
	// The menu is deliberately not rebuilt here. The driver resets the live
	// menu on every SetSystemTrayMenu call, so refreshing it would leave the
	// entries wired to abandoned click channels. The tick reflects the saved
	// setting from the next launch instead.
}

// hide sends the window to the background without stopping.
func (a *App) hide() {
	a.win.Hide()
}

// quit stops the app for good.
func (a *App) quit() {
	a.mu.Lock()
	a.closing = true
	a.mu.Unlock()
	if err := config.Save(a.cfg); err != nil {
		log.Println("saving settings:", err)
	}
	fyne.CurrentApp().Quit()
}

// tabGap is the space between the tab bar and a tab's content. Without it the
// selected tab's blue underline sits flush against the top of the pane, which
// on the Inbox tab looked as though it collided with the Refresh button.
const tabGap = 6

// withTabGap puts a small gap above a tab's content.
func withTabGap(o fyne.CanvasObject) fyne.CanvasObject {
	c := container.NewWithoutLayout(o)
	c.Layout = layout.NewCustomPaddedLayout(tabGap, 0, 0, 0)
	return c
}

// newFilterRow builds a pane's toolbar: a Refresh button and a filter field.
//
// The field is the middle object of a border layout. In a horizontal box every
// child is sized to its own minimum, which left the field about forty pixels
// wide and unusable; as the middle object it takes what the button leaves over.
//
// Every pane gets its own field, because a Fyne object may belong to only one
// parent. They all drive the same query, so typing in one narrows all three
// folders, and the fields are kept in step.
func (a *App) newFilterRow() fyne.CanvasObject {
	refresh := widget.NewButtonWithIcon("Refresh", ftheme.ViewRefreshIcon(), a.refreshAll)
	refresh.Importance = widget.HighImportance

	e := widget.NewEntry()
	e.SetPlaceHolder("Filter by sender or text")
	// Guard against the re-entrant SetText that syncing the other fields causes.
	if !a.syncingFilter {
		a.search = e
	}
	a.filters = append(a.filters, e)
	e.OnChanged = func(s string) {
		a.setQuery(s)
	}
	return container.NewBorder(nil, nil, refresh, nil, e)
}

// setQuery applies a filter to every folder and mirrors the value across the
// per-pane fields.
func (a *App) setQuery(s string) {
	a.query = s
	a.inbox.SetQuery(s)
	a.outbox.SetQuery(s)
	a.drafts.SetQuery(s)

	a.syncingFilter = true
	for _, e := range a.filters {
		if e.Text != s {
			e.SetText(s)
		}
	}
	a.syncingFilter = false
}

// buildTabs creates the tab bar. Each pane arrives already laid out, with its
// own toolbar and content, with a gap under the bar so the selected tab's
// underline does not sit against the content.
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

// setUnreadBadge puts the unread count on the inbox tab, so it is visible from
// any tab rather than only from the inbox itself.
func (a *App) setUnreadBadge(n int) {
	if a.inboxTab == nil || a.shownUnread == n {
		return
	}
	a.shownUnread = n
	if n > 0 {
		a.inboxTab.Text = "Inbox (" + strconv.Itoa(n) + ")"
	} else {
		a.inboxTab.Text = "Inbox"
	}
	if a.tabs != nil {
		a.tabs.Refresh()
	}
}

// installShortcuts adds the window-level keys. They are registered on the
// canvas, and Fyne gives the focused widget first refusal, so typing in a
// message field is unaffected.
func (a *App) installShortcuts() {
	c := a.win.Canvas()

	refresh := func(fyne.Shortcut) { a.refreshAll() }
	clearFilter := func(fyne.Shortcut) { a.setQuery("") }
	focusFilter := func(fyne.Shortcut) {
		if a.search != nil {
			c.Focus(a.search)
		}
	}
	newMessage := func(fyne.Shortcut) { a.tabs.SelectIndex(3) }

	ctrl := fyne.KeyModifierControl
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF5}, refresh)
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: ctrl}, refresh)
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: ctrl}, focusFilter)
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: ctrl}, newMessage)
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEscape}, clearFilter)
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

// that tab is shown first.
func (a *App) Start(startTab string) {
	if i := a.tabIndex(startTab); i >= 0 {
		a.tabs.SelectIndex(i)
	}
	// Without a saved password the modem cannot be reached, so there is no
	// point polling; the app waits on the settings form instead.
	if !a.cfg.Ready() {
		a.tabs.SelectIndex(a.tabIndex("settings"))
		a.bar.SetIdle("Not connected")
		a.setStatus("Enter the modem password in Settings, then save")
		return
	}
	a.installShortcuts()
	a.refreshAll()
	go a.pollLoop()
}

// EnableTray installs the system tray icon and the close-to-background
// behaviour.
//
// It is deliberately separate from Start: the tray reaches into the window
// manager, which a headless test application cannot answer, so only main calls
// it.
func (a *App) EnableTray() { a.installTray() }

// TabNames lists the tabs in the order they appear, which the command line
// argument is matched against.
func (a *App) TabNames() []string {
	if a.tabs == nil {
		return nil
	}
	out := make([]string, 0, len(a.tabs.Items))
	for _, item := range a.tabs.Items {
		out = append(out, item.Text)
	}
	return out
}

// tabIndex maps a tab name to its position, or -1 when the name is empty or
// unknown. Spaces and case are ignored, so "newmessage" finds "New message".
func (a *App) tabIndex(name string) int {
	squash := func(s string) string {
		return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	}
	want := squash(name)
	if want == "" || a.tabs == nil {
		return -1
	}
	for i, item := range a.tabs.Items {
		if squash(item.Text) == want {
			return i
		}
	}
	return -1
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

// diagnostics gathers the facts worth having in a bug report: what the app is
// pointed at, what the modem reports about itself, and the live link state.
func (a *App) diagnostics() string {
	var b strings.Builder
	fmt.Fprintf(&b, "ModemPhone %s", buildinfo.Describe())
	if c := buildinfo.Short(); c != "" {
		fmt.Fprintf(&b, " (%s)", c)
	}
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "  settings file: %s\n", mustPath())
	fmt.Fprintf(&b, "  log file:    %s\n", applog.Describe())
	fmt.Fprintf(&b, "  router:        %s (user %s)\n", a.cfg.Host, a.cfg.Username)
	fmt.Fprintf(&b, "  poll interval: %ds\n", a.cfg.PollSeconds)
	fmt.Fprintf(&b, "  storage:       %s\n", a.cfg.StorageMode)
	fmt.Fprintf(&b, "  consecutive failures: %d\n", a.failures)
	fmt.Fprintf(&b, "  link:          %s\n", a.lastStatus.LTEStatus)
	fmt.Fprintf(&b, "  carrier:       %s\n", a.lastStatus.LTEConnType)
	fmt.Fprintf(&b, "  lte address:   %s\n", a.lastStatus.LTEIP)
	fmt.Fprintf(&b, "  ipv4:          connected=%v %s\n", a.lastStatus.IPv4Connected, a.lastStatus.IPv4IP)
	fmt.Fprintf(&b, "  ipv6:          connected=%v %s\n", a.lastStatus.IPv6Connected, a.lastStatus.IPv6IP)
	fmt.Fprintf(&b, "  model:         %s %s\n", a.device.Model, a.device.Hardware)
	fmt.Fprintf(&b, "  module:        %s\n", a.device.ModuleName)
	fmt.Fprintf(&b, "  imei:          %s\n", a.device.IMEI)
	return b.String()
}

// mustPath is the settings file location, for the diagnostics text.
func mustPath() string {
	p, err := config.Path()
	if err != nil {
		return "unknown"
	}
	return p
}

// do runs fn on a worker goroutine. Only one network call runs at a time so a
// slow page cannot pile up behind the UI.
func (a *App) do(fn func()) {
	a.mu.Lock()
	if a.inFlight || a.closing {
		a.mu.Unlock()
		return
	}
	a.inFlight = true
	a.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Println("worker panic:", r)
			}
			a.mu.Lock()
			a.inFlight = false
			a.mu.Unlock()
		}()
		fn()
	}()
}

func (a *App) pollLoop() {
	for {
		a.mu.Lock()
		busy := a.inFlight
		a.mu.Unlock()
		if busy {
			continue
		}
		time.Sleep(pollWait(a.cfg, a.failures))

		a.mu.Lock()
		stop := a.closing
		a.mu.Unlock()
		if stop {
			return
		}

		a.do(func() {
			a.refreshInbox()
			a.refreshStatus()
		})
	}
}

// pollWait is the delay before the next poll, given how many polls have failed
// in a row. It is separate from pollLoop so the backoff can be tested.
func pollWait(cfg config.Config, failures int) time.Duration {
	base := time.Duration(cfg.PollSeconds) * time.Second
	if base < 5*time.Second {
		base = 5 * time.Second
	}
	switch {
	case failures <= 0:
		return base
	case failures < 3:
		return 15 * time.Second
	default:
		return 60 * time.Second
	}
}

func (a *App) setStatus(s string) {
	if a.status != nil && s != "" {
		a.status.SetText(s)
	}
}

func (a *App) refreshAll() {
	a.do(func() {
		a.refreshStatus()
		a.refreshInbox()
		a.refreshOutbox()
		a.refreshDrafts()
	})
}

func (a *App) refreshStatus() {
	st, err := a.client.Status()
	if err != nil {
		a.failures++
		a.setStatus(fmt.Sprintf("Cannot reach the modem at %s: %v (retrying in %s)",
			a.cfg.Host, err, pollWait(a.cfg, a.failures)))
		a.bar.SetStatus(router.Status{})
		return
	}
	if a.failures > 0 {
		a.failures = 0
		a.setStatus("")
	}
	a.lastStatus = st
	a.bar.SetStatus(st)
	if d, err := a.client.Device(); err == nil {
		a.device = d
		a.bar.SetDevice(d)
	}
}

func (a *App) refreshInbox() {
	msgs, err := a.client.Inbox()
	if err != nil {
		// refreshStatus reports the failure, so this stays quiet rather than
		// writing two errors over one another.
		return
	}
	a.inbox.SetMessages(msgs)
	a.notifyNew(msgs)

	unread := 0
	for _, m := range msgs {
		if !m.Read {
			unread++
		}
	}
	a.setUnreadBadge(unread)
}

func (a *App) refreshOutbox() {
	if msgs, err := a.client.Messages(router.FolderOutbox); err == nil {
		a.outbox.SetMessages(msgs)
	}
}

func (a *App) refreshDrafts() {
	if msgs, err := a.client.Messages(router.FolderDrafts); err == nil {
		a.drafts.SetMessages(msgs)
	}
}

// notifyNew announces messages that were not present the last time the inbox
// was read. The first pass only records what is already there, so starting the
// app does not fire a burst of notifications for the existing backlog.
func (a *App) notifyNew(msgs []router.Message) {
	if !a.cfg.NotifyOnSMS {
		return
	}
	first := !a.sawFirstPass
	for _, m := range msgs {
		if a.notified[m.Index] {
			continue
		}
		if first {
			a.notified[m.Index] = true
			continue
		}
		a.notified[m.Index] = true
		a.notify(m)
	}
	a.sawFirstPass = true
}

// notify raises a desktop notification for a newly arrived message, and brings
// the window forward if it is already open. When the app is in the background
// the notification is the only sign of it, so it carries the sender and a
// readable slice of the body.
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

// openMessage shows a message and marks it read.
//
// The read flag is set on the copy held by the list as well, so the row's dot
// changes without waiting for the next poll.
func (a *App) openMessage(m router.Message) {
	folder := a.currentFolder()
	a.viewer.Show(folder, m)
	a.inbox.MarkShown(m.Index)

	if m.Read {
		return
	}
	// Marking read talks to the modem, so it happens off the UI thread.
	index := m.Index
	go func() {
		if err := a.client.MarkRead(folder, index); err != nil {
			log.Println("mark read:", err)
		}
	}()
}

// currentFolder reports which message tab the user is looking at, so a delete
// or read applies to the right box.
func (a *App) currentFolder() router.Folder {
	if a.tabs == nil || a.tabs.Selected() == nil {
		return router.FolderInbox
	}
	switch a.tabs.Selected().Text {
	case "Outbox":
		return router.FolderOutbox
	case "Drafts":
		return router.FolderDrafts
	}
	return router.FolderInbox
}

func (a *App) replyTo(number string) {
	a.composer.SetRecipients(strings.TrimPrefix(number, "+"))
	if a.tabs != nil {
		a.tabs.SelectIndex(3)
	}
}

func (a *App) deleteMessage(f router.Folder, index string) {
	a.do(func() {
		if err := a.client.Delete(f, index); err != nil {
			a.setStatus("Delete failed: " + err.Error())
			return
		}
		a.setStatus("Message deleted")
		a.refreshInbox()
		a.refreshOutbox()
		a.refreshDrafts()
	})
}

func (a *App) sendMessage(cc, number, body string) error {
	if _, err := a.client.Send(cc, number, body); err != nil {
		return err
	}
	go func() {
		time.Sleep(2 * time.Second)
		a.refreshOutbox()
		a.refreshInbox()
	}()
	return nil
}

func (a *App) sendUSSD(code string) (router.USSDResponse, error) {
	return a.client.SendUSSD(code)
}

func (a *App) chooseUSSD(choice string) (router.USSDResponse, error) {
	return a.client.SelectUSSDOption(choice)
}

func (a *App) cancelUSSD() error { return a.client.CancelUSSD() }

func (a *App) settingsPane() fyne.CanvasObject {
	host := widget.NewEntry()
	host.Text = a.cfg.Host
	host.SetPlaceHolder("192.168.0.1")

	user := widget.NewEntry()
	user.Text = a.cfg.Username

	pass := widget.NewPasswordEntry()
	pass.Text = a.cfg.Password
	pass.SetPlaceHolder("Modem password")

	country := widget.NewEntry()
	country.Text = a.cfg.CountryCode

	defaultNum := widget.NewEntry()
	defaultNum.Text = a.cfg.DefaultNumber
	defaultNum.SetPlaceHolder("09101234567")

	poll := widget.NewSelect([]string{"10", "15", "30", "60", "120"}, nil)
	poll.SetSelected(strconv.Itoa(a.cfg.PollSeconds))

	const (
		optME = "ME — module memory"
		optSM = "SM — SIM card"
	)
	storage := widget.NewSelect([]string{optME, optSM}, nil)
	if a.cfg.StorageMode == "SM" {
		storage.SetSelected(optSM)
	} else {
		storage.SetSelected(optME)
	}

	// Start with Windows, backed by the per-user Run key.
	atLogin := widget.NewCheck("Start when Windows signs in", func(on bool) {
		now, err := startup.Set(on)
		a.cfg.StartAtLogin = now
		if err != nil {
			a.setStatus(err.Error())
		} else if now {
			a.setStatus("Added to Windows startup")
		} else {
			a.setStatus("Removed from Windows startup")
		}
		if err := config.Save(a.cfg); err != nil {
			log.Println("saving settings:", err)
		}
	})
	atLogin.SetChecked(startup.IsEnabled())
	a.cfg.StartAtLogin = startup.IsEnabled()

	loginNote := widget.NewLabel("Uses the per-user startup list, so no administrator rights are needed. " +
		"It can also be changed later in Task Manager's Startup tab.")
	loginNote.Wrapping = fyne.TextWrapWord
	loginNote.Importance = widget.MediumImportance

	note := widget.NewLabel("USSD needs a data connection. This modem runs an IPv6-only " +
		"context, so USSD codes can time out until the APN is set to IPv4 or IPv4v6 " +
		"on the router's LTE page.")
	note.Wrapping = fyne.TextWrapWord
	note.Importance = widget.MediumImportance

	// Diagnostics, for a bug report: everything needed to reproduce a problem
	// without asking the user to go and read it off six different screens.
	diag := widget.NewLabel(a.diagnostics())
	diag.Wrapping = fyne.TextWrapWord

	copyDiag := widget.NewButtonWithIcon("Copy diagnostics", ftheme.ContentCopyIcon(), func() {
		if ui.Copy(a.win, a.diagnostics()) {
			a.setStatus("Diagnostics copied to the clipboard")
		}
	})

	shortcuts := widget.NewLabel(
		"Keys:  F5 or Ctrl+R refresh  ·  Ctrl+F filter  ·  Ctrl+N new message  ·  Esc clear filter\n" +
			"Closing the window keeps the app running in the tray; use its Quit menu to exit.")
	shortcuts.Wrapping = fyne.TextWrapWord
	shortcuts.Importance = widget.MediumImportance

	save := widget.NewButtonWithIcon("Save", ftheme.DocumentSaveIcon(), func() {
		a.cfg.Host = strings.TrimSpace(host.Text)
		a.cfg.Username = strings.TrimSpace(user.Text)
		a.cfg.Password = pass.Text
		a.cfg.CountryCode = strings.TrimSpace(country.Text)
		a.cfg.DefaultNumber = defaultNum.Text
		if v, err := strconv.Atoi(poll.Selected); err == nil {
			a.cfg.PollSeconds = v
		}
		if storage.Selected == optSM {
			a.cfg.StorageMode = "SM"
		} else {
			a.cfg.StorageMode = "ME"
		}
		if err := config.Save(a.cfg); err != nil {
			a.setStatus("Could not save settings: " + err.Error())
			return
		}
		a.composer.SetCountryCode(a.cfg.CountryCode)
		mode := a.cfg.StorageMode
		a.client.Logout()
		a.do(func() {
			if err := a.client.SetStorage(mode); err != nil {
				log.Println("set storage:", err)
			}
			a.refreshAll()
		})
		a.setStatus("Settings saved")
	})
	save.Importance = widget.HighImportance

	// row puts a caption on the left and the field in the remaining space. The
	// field must be the only object in the variadic list: Fyne's border layout
	// dereferences each entry without a nil check, so a stray nil here would
	// panic the first time the window is drawn.
	row := func(label string, w fyne.CanvasObject) fyne.CanvasObject {
		return container.NewBorder(nil, nil, ui.Muted(label, 12, a.fonts), nil, w)
	}

	return container.NewVScroll(container.NewVBox(
		ui.Bold("Router", theme.Text, 16, a.fonts),
		row("Address", host),
		row("Username", user),
		row("Password", pass),
		ui.Divider(),
		ui.Bold("Messages", theme.Text, 16, a.fonts),
		row("Country code", country),
		row("Default recipient", defaultNum),
		row("Store new messages in", storage),
		ui.Divider(),
		ui.Bold("Startup", theme.Text, 16, a.fonts),
		atLogin,
		loginNote,
		ui.Divider(),
		ui.Bold("Polling", theme.Text, 16, a.fonts),
		row("Check every (seconds)", poll),
		container.NewHBox(save),
		ui.Divider(),
		note,
		shortcuts,
		ui.Divider(),
		ui.Bold("Diagnostics", theme.Text, 16, a.fonts),
		diag,
		container.NewHBox(copyDiag),
	))
}
