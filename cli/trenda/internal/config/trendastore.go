// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. The Trenda session lives in a cookie store written by the
// Node sign-in helper, and this is how the Go CLI reads and updates it.
//
// The two halves used to disagree: `trenda-pp-cli auth login` shells out to the
// helper, which saves cookies to ~/.config/pp-trenda/credentials.json, while
// the Go CLI only ever looked at TRENDA_SESSION or its own config.toml. So a
// successful login left the direct CLI unauthenticated, and only the `trenda`
// wrapper - which fetched the cookie separately - worked.
//
// The fix is to read that same store rather than to copy out of it. Copying the
// cookie into a second file would work until the session refreshed, and then
// the copy would be a stale credential nobody updates. There is one store, and
// both halves read and write it.

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TrendaStorePath is where the Node sign-in helper keeps the session. The path
// is fixed by that helper (apps/trenda/lib/store.mjs); it is not configurable
// on either side, and the two must not drift apart.
func TrendaStorePath() string {
	if v := os.Getenv("TRENDA_CREDENTIALS_PATH"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "pp-trenda", "credentials.json")
}

// TrendaStore is the on-disk session. Only the cookie jar matters to this CLI;
// other fields the helper writes are preserved on save so nothing is lost.
type TrendaStore struct {
	// Jar maps cookie name to value. The server chooses the names, so they are
	// deliberately not enumerated here.
	Jar map[string]string `json:"jar"`

	// rest keeps every other field the helper stored, so a save from Go does
	// not silently drop what Node put there.
	rest map[string]json.RawMessage
}

// LoadTrendaStore reads the session, or reports that there is none. A missing
// file is not an error: it means "not signed in".
func LoadTrendaStore(path string) (*TrendaStore, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading the Trenda session at %s: %w", path, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("the Trenda session at %s is not valid JSON: %w", path, err)
	}
	store := &TrendaStore{Jar: map[string]string{}, rest: map[string]json.RawMessage{}}
	for k, v := range raw {
		if k == "jar" {
			if err := json.Unmarshal(v, &store.Jar); err != nil {
				return nil, fmt.Errorf("the Trenda session at %s has an unreadable cookie jar: %w", path, err)
			}
			continue
		}
		store.rest[k] = v
	}
	return store, nil
}

// Save writes the store back with owner-only permissions, preserving whatever
// fields the Node helper keeps alongside the jar.
func (s *TrendaStore) Save(path string) error {
	if path == "" {
		return fmt.Errorf("no path for the Trenda session")
	}
	out := map[string]json.RawMessage{}
	for k, v := range s.rest {
		out[k] = v
	}
	jar, err := json.Marshal(s.Jar)
	if err != nil {
		return fmt.Errorf("encoding the cookie jar: %w", err)
	}
	out["jar"] = jar

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the Trenda session: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	// Write through a temporary file so an interrupted save cannot leave a
	// truncated session behind, and never widen the permissions.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing the Trenda session: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("saving the Trenda session: %w", err)
	}
	return os.Chmod(path, 0o600)
}

// CookieHeader renders the jar as a Cookie header value. Cookies are sorted so
// the header is stable between runs, which keeps dry-run output and test
// assertions from depending on Go's map ordering.
func (s *TrendaStore) CookieHeader() string {
	if s == nil || len(s.Jar) == 0 {
		return ""
	}
	names := make([]string, 0, len(s.Jar))
	for name := range s.Jar {
		if name != "" && s.Jar[name] != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+s.Jar[name])
	}
	return strings.Join(parts, "; ")
}

// MergeSetCookie folds a response's Set-Cookie headers into the jar and reports
// whether anything changed, so a caller only writes to disk when it must.
//
// Cookie names are whatever the server chose, so nothing is filtered by name.
// A cookie whose value is empty is a deletion.
func (s *TrendaStore) MergeSetCookie(headers []string) bool {
	if s.Jar == nil {
		s.Jar = map[string]string{}
	}
	changed := false
	for _, h := range headers {
		pair := h
		if i := strings.IndexByte(pair, ';'); i >= 0 {
			pair = pair[:i]
		}
		eq := strings.IndexByte(pair, '=')
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(pair[:eq])
		value := strings.TrimSpace(pair[eq+1:])
		if name == "" {
			continue
		}
		if value == "" {
			if _, had := s.Jar[name]; had {
				delete(s.Jar, name)
				changed = true
			}
			continue
		}
		if s.Jar[name] != value {
			s.Jar[name] = value
			changed = true
		}
	}
	return changed
}

// ClearTrendaStore removes the session file. A missing file is success: the
// caller asked for there to be no session, and there is none.
func ClearTrendaStore(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing the Trenda session at %s: %w", path, err)
	}
	return nil
}
