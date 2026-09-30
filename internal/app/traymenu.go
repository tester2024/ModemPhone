package app

import (
	"fyne.io/fyne/v2"
)

// trayMenu builds the system tray menu.
//
// It is separate from installTray so the menu can be built and inspected on its
// own, which is what the tests do: a menu that cannot be described is a menu
// that cannot be trusted.
func (a *App) trayMenu() *fyne.Menu {
	// A separator needs the IsSeparator flag. An item with an empty label and
	// no action draws a blank clickable row instead.
	sep := func() *fyne.MenuItem { return &fyne.MenuItem{IsSeparator: true} }

	return fyne.NewMenu(
		"ModemPhone",

		&fyne.MenuItem{Label: "Open", Action: func() { a.show() }},
		&fyne.MenuItem{Label: "Inbox", Action: func() { a.showTab("Inbox") }},
		&fyne.MenuItem{Label: "New message", Action: func() { a.showTab("New message") }},
		sep(),
		&fyne.MenuItem{Label: "Check for messages now", Action: func() { a.refreshAll() }},
		&fyne.MenuItem{
			Label:   "Start with Windows",
			Checked: a.cfg.StartAtLogin,
			Action:  func() { a.toggleStartAtLogin() },
		},
		sep(),
		&fyne.MenuItem{Label: "Quit", Action: func() { a.quit() }},
	)
}
