package app

import (
	"log"
	"time"

	"modemphone/internal/config"
)

// do runs fn on a worker goroutine. Only one refresh runs at a time, so a slow
// page cannot pile up behind the interface.
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

// pollLoop refreshes on an interval for as long as the app is running.
func (a *App) pollLoop() {
	for {
		a.mu.Lock()
		busy := a.inFlight
		a.mu.Unlock()
		if !busy {
			time.Sleep(pollWait(a.cfg, a.failures))
		} else {
			time.Sleep(500 * time.Millisecond)
		}

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
// in a row. Backing off avoids hammering a modem that is off or rebooting, and
// the floor stops a short interval from becoming a request flood.
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
