package app

import "embed"

// trayIconFS carries the tray icon into the binary. The Windows notification
// area needs a raster image, so an SVG from Fyne's icon set will not work.
//
//go:embed assets/icons/tray.png
var trayIconFS embed.FS
