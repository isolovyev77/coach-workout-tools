// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Accessors and persistence for btwb's two credentials.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pelletier/go-toml/v2"
)

// SessionValue returns the member session cookie, preferring the environment.
func (c *Config) SessionValue() string {
	if v := os.Getenv("BTWB_SESSION_COOKIE"); v != "" {
		return v
	}
	return c.BtwbSessionCookie
}

// WidgetKeyValue returns the gym's Web Widgets key, preferring the environment.
func (c *Config) WidgetKeyValue() string {
	if v := os.Getenv("BTWB_WIDGET_KEY"); v != "" {
		return v
	}
	return c.WidgetKey
}

// MemberIDValue returns the member id cached at login, or 0 when unknown.
func (c *Config) MemberIDValue() int {
	if v := os.Getenv("BTWB_MEMBER_ID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return c.MemberID
}

// SaveSession persists the session cookie and the member it belongs to.
func (c *Config) SaveSession(cookie string, memberID int) error {
	c.BtwbSessionCookie = cookie
	if memberID != 0 {
		c.MemberID = memberID
	}
	return c.writeFile()
}

// SaveWidgetKey persists the gym's Web Widgets key.
func (c *Config) SaveWidgetKey(key string) error {
	c.WidgetKey = key
	return c.writeFile()
}

// writeFile writes the config with owner-only permissions. It mirrors the
// generated save() but is kept separate so a regen cannot drop it.
func (c *Config) writeFile() error {
	path := c.Path
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".config", "btwb-pp-cli", "config.toml")
		c.Path = path
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	// Credentials live here; keep it unreadable to other users.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return os.Chmod(path, 0o600)
}
