package app

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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

// diagnostics gathers the facts worth having in a bug report: what the app is
// pointed at, what the modem reports about itself, and the live link state.
func (a *App) diagnostics() string {
	var b strings.Builder
	fmt.Fprintf(&b, "ModemPhone %s\n", buildinfo.Describe())
	fmt.Fprintf(&b, "  settings file: %s\n", mustPath())
	fmt.Fprintf(&b, "  log file:      %s\n", applog.Describe())
	fmt.Fprintf(&b, "  router:        %s (user %s)\n", a.cfg.Host, a.cfg.Username)
	fmt.Fprintf(&b, "  poll interval: %ds\n", a.cfg.PollSeconds)
	fmt.Fprintf(&b, "  storage:       %s\n", a.cfg.StorageMode)
	fmt.Fprintf(&b, "  failures:      %d consecutive\n", a.failures)
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

// refreshStatus reads the link state and the modem's identity.
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

// refreshInbox reloads the inbox and raises notifications for anything new.
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

// refreshAll reloads everything, used by the Refresh button and the tray.
func (a *App) refreshAll() {
	a.do(func() {
		a.refreshStatus()
		a.refreshInbox()
		a.refreshOutbox()
		a.refreshDrafts()
	})
}

// openMessage shows a message and marks it read.
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

// currentFolder reports which message tab is on screen, so a delete or a read
// applies to the right box.
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

// settingsPane builds the settings tab.
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

	save := widget.NewButtonWithIcon("Save", ftheme.DocumentSaveIcon(), func() {
		a.cfg.Host = strings.TrimSpace(host.Text)
		a.cfg.Username = strings.TrimSpace(user.Text)
		a.cfg.Password = pass.Text
		a.cfg.CountryCode = strings.TrimSpace(country.Text)
		a.cfg.DefaultNumber = strings.TrimSpace(defaultNum.Text)
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

	// A caption on the left, the field taking the rest. The field must be the
	// only object in the variadic list: Fyne's border layout dereferences every
	// entry, so a stray nil here would panic the first time the pane is drawn.
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
