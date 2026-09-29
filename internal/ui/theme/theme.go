// Package theme provides the app's dark colour scheme and font plumbing.
package theme

import (
	"image/color"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	ftheme "fyne.io/fyne/v2/theme"
)

// The palette: a near-black blue-grey base, one blue accent, and a small set of
// semantic colours for connection state.
var (
	Bg          = color.NRGBA{0x0D, 0x11, 0x17, 0xFF}
	Surface     = color.NRGBA{0x16, 0x1B, 0x22, 0xFF}
	SurfaceHigh = color.NRGBA{0x1F, 0x26, 0x2E, 0xFF}
	Border      = color.NRGBA{0x2A, 0x33, 0x3D, 0xFF}

	Text      = color.NRGBA{0xE6, 0xED, 0xF3, 0xFF}
	TextMuted = color.NRGBA{0x8B, 0x94, 0x9E, 0xFF}
	TextFaint = color.NRGBA{0x6E, 0x76, 0x81, 0xFF}
	// Disabled is the foreground of a widget that cannot be acted on, most
	// visibly the read-only message body.
	Disabled = color.NRGBA{0xB2, 0xBB, 0xC5, 0xFF}

	Accent     = color.NRGBA{0x58, 0xA6, 0xFF, 0xFF}
	AccentWeak = color.NRGBA{0x14, 0x27, 0x3D, 0xFF}

	Success = color.NRGBA{0x3F, 0xB9, 0x50, 0xFF}
	Warn    = color.NRGBA{0xD2, 0x99, 0x22, 0xFF}
	Danger  = color.NRGBA{0xF8, 0x51, 0x49, 0xFF}

	SuccessBg = color.NRGBA{0x12, 0x2B, 0x1B, 0xFF}
	WarnBg    = color.NRGBA{0x2E, 0x24, 0x0C, 0xFF}
	DangerBg  = color.NRGBA{0x33, 0x16, 0x16, 0xFF}
)

// Fonts holds the faces loaded at startup. Vazirmatn covers both Latin and
// Arabic script, so one family serves the whole app.
type Fonts struct {
	Regular fyne.Resource
	Medium  fyne.Resource
	Bold    fyne.Resource
}

// Load reads the font files from dir. The regular face is required; the others
// are optional and fall back to it.
func Load(dir string) (Fonts, error) {
	var f Fonts
	var err error
	if f.Regular, err = loadFont(dir, "Vazirmatn-Regular.ttf"); err != nil {
		return f, err
	}
	if f.Bold, err = loadFont(dir, "Vazirmatn-Bold.ttf"); err == nil {
		_ = f.Bold
	} else {
		f.Bold = f.Regular
	}
	if f.Medium, err = loadFont(dir, "Vazirmatn-Medium.ttf"); err == nil {
		_ = f.Medium
	} else {
		f.Medium = f.Regular
	}
	return f, nil
}

func loadFont(dir, name string) (fyne.Resource, error) {
	p := filepath.Join(dir, name)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return fyne.NewStaticResource(name, data), nil
}

// Dark returns a Fyne theme using this palette and the given fonts.
//
// Supplying the font through the theme rather than per object is what makes
// Persian render everywhere, including inside Fyne's own widgets: entries,
// buttons and list items all ask the theme for a font.
func Dark(fonts Fonts) fyne.Theme { return &appTheme{fonts: fonts} }

type appTheme struct{ fonts Fonts }

func (t *appTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case ftheme.ColorNameBackground:
		return Bg
	case ftheme.ColorNameButton:
		return SurfaceHigh
	case ftheme.ColorNameDisabled:
		// Used for a disabled widget's own foreground, most visibly the
		// read-only message body. Too faint and the message becomes hard to
		// read, so this is a muted grey rather than the usual near-invisible
		// one.
		return Disabled
	case ftheme.ColorNameDisabledButton:
		return Surface
	case ftheme.ColorNameError:
		return Danger
	case ftheme.ColorNameFocus:
		return Accent
	case ftheme.ColorNameForeground:
		return Text
	case ftheme.ColorNameHover:
		return SurfaceHigh
	case ftheme.ColorNameInputBackground:
		return Surface
	case ftheme.ColorNameInputBorder:
		return Border
	case ftheme.ColorNameMenuBackground:
		return Surface
	case ftheme.ColorNameOverlayBackground:
		return Bg
	case ftheme.ColorNamePlaceHolder:
		return TextFaint
	case ftheme.ColorNamePressed:
		return AccentWeak
	case ftheme.ColorNamePrimary:
		return Accent
	case ftheme.ColorNameSelection:
		return AccentWeak
	case ftheme.ColorNameSeparator:
		return Border
	case ftheme.ColorNameScrollBar:
		return Border
	case ftheme.ColorNameShadow:
		return color.Transparent
	case ftheme.ColorNameSuccess:
		return Success
	case ftheme.ColorNameWarning:
		return Warn
	case ftheme.ColorNameHeaderBackground:
		return Surface
	case ftheme.ColorNameForegroundOnPrimary:
		return Bg
	case ftheme.ColorNameForegroundOnError:
		return Text
	case ftheme.ColorNameForegroundOnSuccess:
		return Text
	case ftheme.ColorNameForegroundOnWarning:
		return Text
	case ftheme.ColorNameHyperlink:
		return Accent
	}
	return ftheme.DefaultTheme().Color(name, variant)
}

// Font hands out the app font, choosing the face that matches the style.
func (t *appTheme) Font(style fyne.TextStyle) fyne.Resource {
	if t.fonts.Regular == nil {
		return ftheme.DefaultTheme().Font(style)
	}
	switch {
	case style.Bold:
		return t.fonts.Bold
	case style.Monospace, style.Symbol:
		return ftheme.DefaultTheme().Font(style)
	}
	return t.fonts.Regular
}

func (t *appTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return ftheme.DefaultTheme().Icon(name)
}

func (t *appTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case ftheme.SizeNameText:
		return 14
	case ftheme.SizeNameHeadingText:
		return 20
	case ftheme.SizeNameSubHeadingText:
		return 15
	case ftheme.SizeNameCaptionText:
		return 12
	case ftheme.SizeNamePadding:
		return 4
	case ftheme.SizeNameInnerPadding:
		return 8
	case ftheme.SizeNameInlineIcon:
		return 18
	case ftheme.SizeNameScrollBar:
		return 8
	case ftheme.SizeNameSeparatorThickness:
		return 1
	}
	return ftheme.DefaultTheme().Size(name)
}
