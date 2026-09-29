# ModemPhone

A native desktop app for reading, sending and managing SMS on a 4G modem, with
USSD support. Written in Go, no webview, single executable.

![Inbox](shots/inbox.png)

## What this works with

Any 4G CPE whose admin interface exposes a conventional web UI. It was
developed against a consumer LTE router whose firmware keeps its **Short
Message Service** pages out of the visible navigation menu but still serves
them — the app drives those pages directly.

To point it at your own hardware, open **Settings** and enter the router's
address and password.

## Features

- **Inbox, outbox and drafts**, with a filter box, unread markers, and an unread
  count on the inbox tab
- **Reading pane** where the message text is selectable: drag with the mouse,
  `Ctrl+A`, `Ctrl+C`, or press **Copy all**
- **Numbers in a message become their own copyable controls**, so a phone
  number or a USSD code in the text is one click away
- **Compose** with bulk send (semicolon-separated recipients), a live character
  counter, and input filtering to what the modem accepts
- **USSD** terminal with interactive menu support and suggested codes
- **Background running**: closing the window hides it and keeps polling, with a
  system tray icon holding Show, Check now and Quit
- **Desktop notifications** for newly arrived messages
- **Start with Windows** (optional, from Settings)
- **Reconnect backoff**, so a powered-off modem is not hammered with requests
- **Diagnostics** you can copy straight into a bug report

### Keys

| Key | Action |
|---|---|
| `F5` or `Ctrl+R` | Refresh |
| `Ctrl+F` | Focus the filter |
| `Ctrl+N` | New message |
| `Esc` | Clear the filter |
| `Ctrl+A` / `Ctrl+C` | Select and copy the open message |

## About calling

The LTE module inside a router of this kind is usually voice-capable on paper —
VoLTE, caller ID, call hold. **The router is generally what prevents calls:**
the firmware often compiles the voice stack out, and the box has no microphone,
speaker or analogue port. The module's audio is routed into a chip that will
not pass it anywhere, so there is nothing to talk into or listen to.

This app therefore does not attempt calls.

Calling *is* achievable as **VoIP over the modem's data connection**, using a
SIP account and a softphone. That is a separate feature and needs an account
from a provider that can terminate to real phone numbers.

## Known limitations

**USSD needs a data bearer.** If the modem is running an IPv6-only data
context, USSD codes will time out even though SMS works fine, because SMS rides
signalling rather than IP. Changing the APN's IP version to IPv4 or IPv4v6 on
the router's own configuration page resolves it. The app says so on the
Settings tab.

**The firmware gives no feedback on a send.** It answers with an empty page
whether or not the message went out, so rather than report a silent failure as
success the app checks the outbox afterwards and tells you when the modem did
not record the message. A refused send is usually a SIM or network quota.

## Building

The UI toolkit needs cgo, so a C compiler must be on `PATH`. On this machine
that is the MSYS2 gcc; `build.ps1` puts it there for you.

```powershell
.\build.ps1           # build
.\build.ps1 -Test     # build and run the tests
.\build.ps1 -Run      # build and launch
.\install.ps1         # build and install to Program Files
```

The result is a single self-contained executable with the fonts embedded.

## Installing

```powershell
.\install.ps1
```

This builds the app, copies it to `%ProgramFiles%\ModemPhone`, and offers to
register it to start with Windows. The copy in `bin\` can be deleted
afterwards; the installed one is what runs.

## Running

```powershell
modemphone                 # opens on the inbox
modemphone ussd            # opens on USSD
modemphone settings        # opens on Settings
```

The first run has no password saved, so it opens on the compose form. Enter the
modem's password in Settings; it is stored locally, and only there.

## Layout

```
cmd/modemphone/       entry point, embedded fonts, fonts and icon
internal/router/      the modem's web interface client
  client.go           login handshake, session keepalive
  sms.go              inbox, outbox, drafts, send, delete, storage
  ussd.go             USSD codes and interactive menus
  status.go           connection state and device identity
  parse.go            the embedded data formats
internal/config/      settings file
internal/ui/          widgets
  copynumber.go       numbers in a message, as copyable controls
  msglist.go          virtualised message list
  viewer.go           reading pane
  ussd.go             USSD terminal
  theme/              palette and font plumbing
internal/app/         wiring, tray, shortcuts, polling, notifications
tools/                asset generators (icon, fonts table)
```

### Two things worth knowing about the firmware

**Login** is usually a challenge/response handshake: the device issues a
challenge and a pseudo public key, the password is folded in with HMAC-MD5, and
a second HMAC-MD5 is posted back. The session is often tied to the connection,
so the login page, the key request and the setup request must share one
connection. Sessions also expire after a couple of minutes of inactivity, so a
client needs to re-authenticate transparently.

**Right-to-left text needs nothing special.** The UI toolkit shapes Arabic
joining forms and applies the Unicode Bidirectional Algorithm itself, so message
text is passed to it untouched. An earlier version of this app pre-shaped the
text and pre-reversed it, which fought the toolkit's own pass and scrambled the
words; that code is gone. What the app does supply is the font, through the
theme, which is what makes Persian render inside the toolkit's own widgets.

One detail does matter: a one-line preview must be elided on a **word
boundary**, since cutting mid-word leaves a dangling joining form.
