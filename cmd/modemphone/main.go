// Command modemphone is a desktop client for the SMS and USSD features of a 4G modem.
//
// The modem's firmware keeps its message pages out of the visible menu but
// serves them anyway, so this app drives them directly. The modem itself is a
// LTE module, which on this class of router is data and messaging only:
// the box has no audio path, so the app does not attempt calls.
package main

import (
	"embed"
	"log"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	fapp "fyne.io/fyne/v2/app"

	"modemphone/internal/app"
	"modemphone/internal/applog"
	"modemphone/internal/config"
	"modemphone/internal/ui/theme"
)

// fonts are embedded so the app is a single self-contained executable.
//
//go:embed all:assets/fonts/*.ttf
var fontFS embed.FS

// iconFS carries the artwork. The window icon has to be a raster image, so the
// PNG is used there; the .ico is only embedded for the executable's own
// resources.
//
//go:embed assets/icon.png
var iconFS embed.FS

// appIcon is the window and taskbar icon, set before any window is created so
// the first frame already shows it.
func appIcon() fyne.Resource {
	raw, err := iconFS.ReadFile("assets/icon.png")
	if err != nil {
		log.Println("icon:", err)
		return nil
	}
	return fyne.NewStaticResource("icon.png", raw)
}

// startTab is set from the command line to open the app on a given tab. It
// defaults to the inbox, or to the compose form when no password is saved yet so
// the first run goes somewhere useful.
var startTab string

func main() {
	// The executable is a GUI app, so there is no console to log to. Send the
	// log to a file instead, and keep the handle for the life of the process.
	if w := applog.Open(); w != nil {
		defer w.Close()
		log.SetOutput(w)
	}

	// An argument naming a tab opens the app there: modemphone.exe ussd
	startTab = firstArg()

	cfg, err := config.Load()
	if err != nil {
		log.Println("settings:", err)
	}
	if startTab == "" && !cfg.Ready() {
		startTab = "New message"
	}

	ui := fapp.NewWithID("com.modemphone.app")
	if icon := appIcon(); icon != nil {
		ui.SetIcon(icon)
	}

	// Fonts are read from the embedded set first, then from disk next to the
	// executable, which makes it easy to swap in a different typeface.
	fonts, err := loadFonts()
	if err != nil {
		// Without a font the app still runs; Fyne falls back to its default.
		log.Println("fonts:", err)
		fonts = theme.Fonts{}
	}
	ui.Settings().SetTheme(theme.Dark(fonts))

	win := ui.NewWindow("ModemPhone")

	a := app.New(win, cfg, fonts)
	win.SetOnClosed(a.Close)

	// Start is called before the event loop so the first load is already
	// under way when the window appears. The work happens on a goroutine, so
	// this does not delay the first frame.
	a.Start(startTab)

	// The tray is installed from a goroutine after the event loop starts, because
	// it needs a real window manager and would hang a headless test application.
	go a.EnableTray()

	win.ShowAndRun()
}

// firstArg returns the first command line argument, or the empty string.
func firstArg() string {
	if len(os.Args) < 2 {
		return ""
	}
	a := os.Args[1]
	if strings.HasPrefix(a, "-") {
		return ""
	}
	return a
}

// loadFonts prefers the fonts embedded in the binary, then looks beside the
// executable.
func loadFonts() (theme.Fonts, error) {
	var f theme.Fonts
	read := func(name string) ([]byte, error) {
		b, err := fontFS.ReadFile("assets/fonts/" + name)
		if err == nil {
			return b, nil
		}
		exe, exeErr := os.Executable()
		if exeErr != nil {
			return nil, err
		}
		return os.ReadFile(dirJoin(dirOf(exe), "assets", "fonts", name))
	}

	regular, err := read("Vazirmatn-Regular.ttf")
	if err != nil {
		return f, err
	}
	f.Regular = fyne.NewStaticResource("Vazirmatn-Regular.ttf", regular)

	if b, err := read("Vazirmatn-Bold.ttf"); err == nil {
		f.Bold = fyne.NewStaticResource("Vazirmatn-Bold.ttf", b)
	} else {
		f.Bold = f.Regular
	}
	if b, err := read("Vazirmatn-Medium.ttf"); err == nil {
		f.Medium = fyne.NewStaticResource("Vazirmatn-Medium.ttf", b)
	} else {
		f.Medium = f.Regular
	}
	return f, nil
}
