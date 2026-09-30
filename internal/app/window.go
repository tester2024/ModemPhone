package app

import (
	"log"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	ftheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"modemphone/internal/config"
	"modemphone/internal/startup"
)

// trayApp is the system tray part of Fyne's desktop application interface.
// Using the toolkit's own interface, rather than a home-made one, keeps this
// working when the underlying implementation changes.
type trayApp = desktop.App

// EnableTray installs the tray icon and menu.
//
// It must be called before the event loop starts. Registering the tray adds the
// icon immediately, so a late call still shows an icon, but the loop that
// delivers tray clicks is started by the toolkit as it runs and is skipped when
// the tray was not registered yet. The result is an icon that right-clicks to
// nothing, which is why this is not done from a goroutine.
func (a *App) EnableTray() {
	fa := fyne.CurrentApp()
	if fa == nil {
		return
	}
	desk, ok := fa.(trayApp)
	if !ok {
		return
	}
	desk.SetSystemTrayMenu(a.trayMenu())

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

// hide sends the window to the background without stopping.
func (a *App) hide() { a.win.Hide() }

// quit stops the app for good.
func (a *App) quit() {
	a.mu.Lock()
	a.closing = true
	a.mu.Unlock()
	if err := config.Save(a.cfg); err != nil {
		log.Println("saving settings:", err)
	}
	// The menu is deliberately not rebuilt. The driver resets the live menu on
	// every SetSystemTrayMenu call, so refreshing it would leave the entries
	// wired to abandoned click channels. The start-up tick reflects the saved
	// setting from the next launch instead.
	fyne.CurrentApp().Quit()
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
}

// tabGap is the space between the tab bar and a tab's content. Without it the
// selected tab's underline sits flush against the pane below.
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

// tabIndex maps a tab name to its position, ignoring case and spaces, so
// "newmessage" finds "New message". It returns -1 when there is no such tab.
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
