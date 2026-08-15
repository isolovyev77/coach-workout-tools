// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Accessors and persistence for btwb's two credentials.

package config

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pelletier/go-toml/v2"
)

const (
	btwbSessionCookieName       = "_btwb_session_id"
	btwbLegacySessionCookieName = "_btwb_session"
)

// SessionValue returns the member session cookie, preferring the environment.
func (c *Config) SessionValue() string {
	if v := os.Getenv("BTWB_SESSION_COOKIE"); v != "" {
		return v
	}
	return c.BtwbSessionCookie
}

// SessionCookies returns every cookie needed to continue the browser session.
// Older configs contain only the Rails session cookie; newer logins also keep
// the long-lived remember-me cookie that lets btwb issue a replacement session.
func (c *Config) SessionCookies() map[string]string {
	if v := os.Getenv("BTWB_SESSION_COOKIE"); v != "" {
		return map[string]string{btwbSessionCookieName: v}
	}
	cookies := make(map[string]string, len(c.BtwbSessionCookies)+1)
	for name, value := range c.BtwbSessionCookies {
		if name != "" && value != "" {
			cookies[name] = value
		}
	}
	if c.BtwbSessionCookie != "" {
		if _, ok := cookies[btwbSessionCookieName]; !ok {
			cookies[btwbSessionCookieName] = c.BtwbSessionCookie
		}
	}
	return cookies
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
	return c.SaveSessionCookies(map[string]string{btwbSessionCookieName: cookie}, memberID)
}

// SaveSessionCookies persists the complete browser session with owner-only
// permissions. Keeping every cookie matters because `remember_me` can renew a
// short Rails session without asking for the user's password again.
func (c *Config) SaveSessionCookies(cookies map[string]string, memberID int) error {
	clean := make(map[string]string, len(cookies))
	for name, value := range cookies {
		if name != "" && value != "" {
			clean[name] = value
		}
	}
	c.BtwbSessionCookies = clean
	c.BtwbSessionCookie = ""
	for _, name := range []string{btwbSessionCookieName, btwbLegacySessionCookieName} {
		if value := clean[name]; value != "" {
			c.BtwbSessionCookie = value
			break
		}
	}
	if memberID != 0 {
		c.MemberID = memberID
	}
	return c.writeFile()
}

// MergeSessionCookies saves a server-rotated browser session for future CLI
// processes. An explicitly exported session remains caller-managed and is
// never copied back into the config file.
func (c *Config) MergeSessionCookies(cookies []*http.Cookie) (bool, error) {
	if os.Getenv("BTWB_SESSION_COOKIE") != "" || len(cookies) == 0 {
		return false, nil
	}
	merged := c.SessionCookies()
	changed := false
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		if cookie.Value == "" || cookie.MaxAge < 0 {
			if _, ok := merged[cookie.Name]; ok {
				delete(merged, cookie.Name)
				changed = true
			}
			continue
		}
		if merged[cookie.Name] != cookie.Value {
			merged[cookie.Name] = cookie.Value
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	if err := c.SaveSessionCookies(merged, c.MemberIDValue()); err != nil {
		return false, err
	}
	return true, nil
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
