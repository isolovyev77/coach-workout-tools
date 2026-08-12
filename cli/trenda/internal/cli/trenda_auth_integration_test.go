// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// These tests pin the bug this file exists for: `trenda-pp-cli auth login`
// succeeded, wrote cookies, and left the direct CLI unauthenticated, because
// the two halves used different credential locations. Every case below fails
// against the old behaviour.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"trenda-pp-cli/internal/config"
)

// signedInStore writes the session exactly as the Node sign-in helper does,
// and points the CLI at it.
func signedInStore(t *testing.T, jar map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	payload := map[string]any{
		"jar": jar,
		// The helper stores more than the jar; these must survive a save.
		"savedAt": "2026-08-13T00:00:00.000Z",
		"coach":   map[string]any{"id": 48},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRENDA_CREDENTIALS_PATH", path)
	t.Setenv("TRENDA_SESSION", "")
	return path
}

// runTrenda executes the command tree against a given API, with an empty config
// file so nothing but the cookie store can authenticate it.
func runTrenda(t *testing.T, baseURL string, args ...string) (string, error) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("base_url = \""+baseURL+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"--config", cfgPath}, args...))
	err := root.Execute()
	return out.String(), err
}

// 1. The command exists at all. It was added in the release that introduced
// this bug, so a regression that removes it would look like "login broken".
func TestAuthLoginIsInTheCommandTree(t *testing.T) {
	root := RootCmd()
	var auth, login bool
	for _, c := range root.Commands() {
		if c.Name() != "auth" {
			continue
		}
		auth = true
		for _, sub := range c.Commands() {
			if sub.Name() == "login" {
				login = true
			}
		}
	}
	if !auth || !login {
		t.Fatalf("auth=%v login=%v; `trenda-pp-cli auth login` must exist", auth, login)
	}
}

// 2. The heart of the bug: a session written by the sign-in helper must
// authenticate the direct CLI. Before the fix, this store was ignored entirely.
func TestSessionFromTheLoginStoreIsUsed(t *testing.T) {
	signedInStore(t, map[string]string{"session": "abc", "csrf": "xyz"})

	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Write([]byte(`{"id":48}`))
	}))
	t.Cleanup(srv.Close)

	out, err := runTrenda(t, srv.URL, "coach", "get-current", "--json")
	if err != nil {
		t.Fatalf("direct call failed with a session on disk: %v\n%s", err, out)
	}
	if !strings.Contains(gotCookie, "session=abc") || !strings.Contains(gotCookie, "csrf=xyz") {
		t.Errorf("Cookie sent = %q, want both cookies from the login store", gotCookie)
	}
}

// 3. Trenda's cookies are short lived. On 401 the CLI must renew them at the
// refresh endpoint, save the new ones to the same store, and replay the
// request - otherwise the direct CLI works right after signing in and fails an
// hour later.
func TestExpiredSessionIsRefreshedAndSaved(t *testing.T) {
	path := signedInStore(t, map[string]string{"session": "stale"})

	var refreshCalls, dataCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/coach/refresh-token" {
			refreshCalls++
			w.Header().Add("Set-Cookie", "session=fresh; Path=/; HttpOnly")
			w.Write([]byte(`{"ok":true}`))
			return
		}
		dataCalls++
		if r.Header.Get("Cookie") == "session=stale" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"expired"}`))
			return
		}
		w.Write([]byte(`{"id":48}`))
	}))
	t.Cleanup(srv.Close)

	out, err := runTrenda(t, srv.URL, "coach", "get-current", "--json")
	if err != nil {
		t.Fatalf("expired session was not recovered: %v\n%s", err, out)
	}
	if refreshCalls != 1 {
		t.Errorf("refresh called %d times, want exactly 1", refreshCalls)
	}
	if dataCalls < 2 {
		t.Errorf("request was not replayed after the refresh (%d data calls)", dataCalls)
	}

	// The renewed cookie must be persisted, or every process would refresh again.
	saved, err := config.LoadTrendaStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Jar["session"] != "fresh" {
		t.Errorf("stored session = %q, want the refreshed value", saved.Jar["session"])
	}
	// Fields the Node helper keeps must survive a save from Go.
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "savedAt") || !strings.Contains(string(raw), "\"id\": 48") {
		t.Errorf("saving from Go dropped fields the helper wrote:\n%s", raw)
	}
}

// 4. `auth status` must consult the store `auth login` writes. It reported
// "not authenticated" right after a successful login, which is what sent
// everyone looking in the wrong place.
func TestAuthStatusSeesTheLoginSession(t *testing.T) {
	signedInStore(t, map[string]string{"session": "abc"})

	out, err := runTrenda(t, "https://example.invalid", "auth", "status", "--json")
	if err != nil {
		t.Fatalf("auth status errored with a session on disk: %v\n%s", err, out)
	}
	var report struct {
		Authenticated bool   `json:"authenticated"`
		Source        string `json:"source"`
	}
	if jErr := json.Unmarshal([]byte(out), &report); jErr != nil {
		t.Fatalf("status output is not JSON: %v\n%s", jErr, out)
	}
	if !report.Authenticated {
		t.Errorf("authenticated = false after signing in; source=%q", report.Source)
	}
}

// 5. The end the user actually reported: a direct API command answering 200
// instead of 401 after login.
func TestDirectCoachCallSucceedsAfterLogin(t *testing.T) {
	signedInStore(t, map[string]string{"session": "abc"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"id":48}`))
	}))
	t.Cleanup(srv.Close)

	out, err := runTrenda(t, srv.URL, "coach", "get-current", "--json", "--select", "id")
	if err != nil {
		t.Fatalf("coach get-current failed after login: %v\n%s", err, out)
	}
	if !strings.Contains(out, "48") {
		t.Errorf("output does not carry the coach id:\n%s", out)
	}
}

// 6. Logging out must remove the session that is actually in use. Clearing only
// the config file would leave the CLI signed in through the cookie store.
func TestLogoutClearsTheSessionInUse(t *testing.T) {
	path := signedInStore(t, map[string]string{"session": "abc"})

	if out, err := runTrenda(t, "https://example.invalid", "auth", "logout"); err != nil {
		t.Fatalf("logout failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the session file survived logout (%v)", err)
	}

	out, err := runTrenda(t, "https://example.invalid", "auth", "status", "--json")
	if err == nil {
		t.Errorf("still authenticated after logout:\n%s", out)
	}
}

// 7. Local commands must not demand credentials: a coach with no session still
// needs --version and --help to work.
func TestLocalCommandsNeedNoSession(t *testing.T) {
	t.Setenv("TRENDA_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "absent.json"))
	t.Setenv("TRENDA_SESSION", "")

	for _, args := range [][]string{{"--version"}, {"--help"}, {"auth", "--help"}} {
		if out, err := runTrenda(t, "https://example.invalid", args...); err != nil {
			t.Errorf("%v needed a session: %v\n%s", args, err, out)
		}
	}
}

// 8. There must be exactly one credential store. The first attempt at this fix
// copied the cookie into the Go config file, which works until the session
// refreshes and then serves a stale credential nobody updates.
func TestNoSecondCredentialStoreIsWritten(t *testing.T) {
	path := signedInStore(t, map[string]string{"session": "abc"})
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("base_url = \"https://example.invalid\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--config", cfgPath, "auth", "status", "--json"})
	_ = root.Execute()

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "abc") {
		t.Errorf("the session was copied into the config file, creating a second store:\n%s", data)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the real store went missing: %v", err)
	}
}

// An explicitly exported session stays in charge, so CI and debugging can
// override whatever is on disk.
func TestExplicitSessionOverridesTheStore(t *testing.T) {
	signedInStore(t, map[string]string{"session": "from-store"})
	t.Setenv("TRENDA_SESSION", "from-env")

	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Write([]byte(`{"id":48}`))
	}))
	t.Cleanup(srv.Close)

	if out, err := runTrenda(t, srv.URL, "coach", "get-current", "--json"); err != nil {
		t.Fatalf("call failed: %v\n%s", err, out)
	}
	if gotCookie != "from-env" {
		t.Errorf("Cookie = %q, want the explicit TRENDA_SESSION to win", gotCookie)
	}
}
