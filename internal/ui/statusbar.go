package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"modemphone/internal/router"
	"modemphone/internal/ui/theme"
)

// StatusBar is the strip along the top of the window showing the modem's
// connection state.
type StatusBar struct {
	Root *fyne.Container

	fonts   theme.Fonts
	row     *fyne.Container
	conn    fyne.CanvasObject
	carrier fyne.CanvasObject
	ip      fyne.CanvasObject
	uptime  fyne.CanvasObject
	dev     fyne.CanvasObject
}

// SetIdle marks the bar as not connected, used before the first status has been
// read and when the app has nothing to connect to yet.
func (s *StatusBar) SetIdle(text string) {
	s.conn = Chip(text, theme.SurfaceHigh, theme.TextMuted, s.fonts)
	replace(s.row.Objects, 0, s.conn)
	s.carrier = Muted("", 12, s.fonts)
	replace(s.row.Objects, 1, s.carrier)
	s.ip = Muted("", 12, s.fonts)
	replace(s.row.Objects, 2, s.ip)
	s.uptime = Muted("", 12, s.fonts)
	replace(s.row.Objects, 3, s.uptime)
}

// NewStatusBar builds an empty bar.
func NewStatusBar(fonts theme.Fonts) *StatusBar {
	s := &StatusBar{
		fonts:   fonts,
		conn:    Chip("Connecting…", theme.WarnBg, theme.Warn, fonts),
		carrier: Muted("", 12, fonts),
		ip:      Muted("", 12, fonts),
		uptime:  Muted("", 12, fonts),
		dev:     Muted("", 12, fonts),
	}
	s.row = container.NewHBox(s.conn, s.carrier, s.ip, s.uptime, SpacerBox(), s.dev)
	s.Root = container.NewStack(
		canvas.NewRectangle(theme.Surface),
		container.NewPadded(s.row),
	)
	return s
}

// SetStatus shows the live link state.
func (s *StatusBar) SetStatus(st router.Status) {
	bg, fg := theme.DangerBg, theme.Danger
	label := "Offline"
	switch {
	case st.Connected():
		bg, fg, label = theme.SuccessBg, theme.Success, "Connected"
	case st.LTEErr != "":
		fg, label = theme.Warn, "Not registered"
	}
	s.conn = Chip(label, bg, fg, s.fonts)
	replace(s.row.Objects, 0, s.conn)

	s.carrier = Muted(st.LTEConnType, 12, s.fonts)
	replace(s.row.Objects, 1, s.carrier)

	if st.LTEIP != "" {
		s.ip = Muted(st.LTEIP, 12, s.fonts)
	} else {
		s.ip = Muted("no IPv4", 12, s.fonts)
	}
	replace(s.row.Objects, 2, s.ip)

	if st.LTEUpTime > 0 {
		s.uptime = Muted("up "+ShortDuration(time.Duration(st.LTEUpTime)*time.Second), 12, s.fonts)
	} else {
		s.uptime = Muted("", 12, s.fonts)
	}
	replace(s.row.Objects, 3, s.uptime)
}

// SetDevice shows the modem identity.
func (s *StatusBar) SetDevice(d router.DeviceInfo) {
	bits := []string{}
	if d.Model != "" {
		bits = append(bits, d.Model)
	}
	if d.ModuleName != "" {
		bits = append(bits, d.ModuleName)
	}
	if d.Firmware != "" {
		bits = append(bits, d.Firmware)
	}
	s.dev = Muted(joinNonEmpty(bits, " · "), 12, s.fonts)
	replace(s.row.Objects, len(s.row.Objects)-1, s.dev)
}

func replace(objs []fyne.CanvasObject, i int, o fyne.CanvasObject) {
	if i < 0 || i >= len(objs) {
		return
	}
	objs[i] = o
}

func joinNonEmpty(parts []string, sep string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += p
	}
	return out
}

// ShortDuration renders a duration compactly, for example "2d 4h" or "12m".
func ShortDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h < 24 {
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh %dm", h, m)
	}
	days := h / 24
	hours := h % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, hours)
}
