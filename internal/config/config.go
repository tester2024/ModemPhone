// Package config persists the app's settings.
//
// Settings live in a JSON file beside the user's config directory. The router
// password is stored there too, which is a deliberate trade-off: the user asked
// for the app to "just work" with their modem, and there is no keyring on the
// target platform worth the extra dependency. The file is written with owner
// only permissions where the OS supports it.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the saved application state.
type Config struct {
	// Router connection.
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`

	// Defaults for the compose form.
	CountryCode   string `json:"countryCode"`
	DefaultNumber string `json:"defaultNumber"`

	// Behaviour.
	PollSeconds  int    `json:"pollSeconds"`
	NotifyOnSMS  bool   `json:"notifyOnSMS"`
	StartAtLogin bool   `json:"startAtLogin"`
	StorageMode  string `json:"storageMode"`
	WindowWidth  int    `json:"windowWidth"`
	WindowHeight int    `json:"windowHeight"`
}

// Default returns the settings used on first run.
func Default() Config {
	return Config{
		Host:         "192.168.0.1",
		Username:     "admin",
		Password:     "",
		CountryCode:  "+98",
		PollSeconds:  30,
		NotifyOnSMS:  true,
		StorageMode:  "ME",
		WindowWidth:  1080,
		WindowHeight: 760,
	}
}

// ErrNotFound is returned when no settings file exists yet.
var ErrNotFound = errors.New("config: no settings file")

// Path returns the file the settings are stored in.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", fmt.Errorf("config: no config directory: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "modemphone", "config.json"), nil
}

// Load reads the settings, returning Default when the file does not exist.
func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return Default(), err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), fmt.Errorf("config: read: %w", err)
	}
	cfg := Default()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		// A corrupt file should not stop the app from starting.
		return Default(), fmt.Errorf("config: parse %s: %w", p, err)
	}
	cfg.normalise()
	return cfg, nil
}

// Save writes the settings, creating the directory if needed.
func Save(c Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("config: create dir: %w", err)
	}
	c.normalise()
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		return fmt.Errorf("config: write: %w", err)
	}
	return nil
}

func (c *Config) normalise() {
	c.Host = strings.TrimSpace(c.Host)
	c.Username = strings.TrimSpace(c.Username)
	c.CountryCode = strings.TrimSpace(c.CountryCode)
	c.DefaultNumber = strings.TrimSpace(c.DefaultNumber)
	if c.CountryCode == "" {
		c.CountryCode = "+98"
	}
	if c.PollSeconds < 5 {
		c.PollSeconds = 5
	}
	if c.PollSeconds > 600 {
		c.PollSeconds = 600
	}
	switch strings.ToUpper(c.StorageMode) {
	case "SM", "ME":
		c.StorageMode = strings.ToUpper(c.StorageMode)
	default:
		c.StorageMode = "ME"
	}
	if c.WindowWidth < 640 {
		c.WindowWidth = 1080
	}
	if c.WindowHeight < 480 {
		c.WindowHeight = 760
	}
}

// Ready reports whether the settings are complete enough to connect.
func (c Config) Ready() bool { return c.Host != "" && c.Username != "" && c.Password != "" }
