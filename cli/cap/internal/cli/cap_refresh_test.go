// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// These pin the session renewal: a coach signs in once, and ordinary commands
// keep the session alive by trading the stored refresh token for a fresh access
// token - proactively when the stored expiry has passed, reactively when the
// API answers 401 - and replaying the original request. Before this existed the
// refresh hook was an empty stub and every expiry meant typing the password
// again.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cap-pp-cli/internal/client"
)

// refreshReplay answers the content endpoint only for the token the refresh
// issues, and counts every visit, so a test can tell a replayed request from a
// never-sent one.
type refreshReplay struct {
	contentCalls  int
	refreshCalls  int
	refreshGrants []map[string]string
	// alwaysReject makes the content endpoint 401 even a fresh token, the shape
	// of a revoked account - the client must not refresh in a loop over it.
	alwaysReject bool
	// rotate controls whether the refresh answer carries a new refresh token.
	rotate bool
}

func (r *refreshReplay) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/subscriptions/v1/content", func(w http.ResponseWriter, req *http.Request) {
		r.contentCalls++
		if r.alwaysReject || req.Header.Get("Authorization") != "Bearer fresh-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Unauthorized"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"count":1,"tiles":[{"title":"ok"}]}`))
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, req *http.Request) {
		r.refreshCalls++
		grant := map[string]string{}
		_ = json.NewDecoder(req.Body).Decode(&grant)
		r.refreshGrants = append(r.refreshGrants, grant)
		if grant["refresh_token"] != "ref-1" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token expired"}`))
			return
		}
		answer := map[string]any{"access_token": "fresh-tok", "expires_in": 3600}
		if r.rotate {
			answer["refresh_token"] = "ref-2"
		}
		_ = json.NewEncoder(w).Encode(answer)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	oldURL := client.TokenURL
	client.TokenURL = srv.URL + "/token"
	t.Cleanup(func() { client.TokenURL = oldURL })
	return srv
}

// runCapWith runs a command against a config whose token state the test
// controls, and returns the config path so what got persisted can be checked.
func runCapWith(t *testing.T, base, accessToken, refreshToken string, expiry time.Time) (string, string, error) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := "base_url = \"" + base + "\"\n" +
		"access_token = \"" + accessToken + "\"\n" +
		"refresh_token = \"" + refreshToken + "\"\n"
	if !expiry.IsZero() {
		cfg += "token_expiry = " + expiry.Format(time.RFC3339) + "\n"
	}
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--config", cfgPath, "--json", "--no-cache",
		"subscriptions", "get-content", "--urn", "content_api:///programming/affiliate/daily-class-plan/20260810"})
	err := root.Execute()
	return out.String(), cfgPath, err
}

func savedTokens(t *testing.T, cfgPath string) (access, refresh string) {
	t.Helper()
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(line, "access_token = "); ok {
			access = strings.Trim(strings.TrimSpace(v), `"'`)
		}
		if v, ok := strings.CutPrefix(line, "refresh_token = "); ok {
			refresh = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return access, refresh
}

// The stored expiry has passed: the client must renew before the request, and
// the request itself must then succeed on the first try.
func TestExpiredTokenRenewsBeforeTheRequest(t *testing.T) {
	replay := &refreshReplay{rotate: true}
	srv := replay.start(t)

	out, cfgPath, err := runCapWith(t, srv.URL,
		"stale-tok", "ref-1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, out)
	}
	if replay.refreshCalls != 1 {
		t.Errorf("refresh calls = %d, want exactly 1", replay.refreshCalls)
	}
	if replay.contentCalls != 1 {
		t.Errorf("content calls = %d, want 1 (renewed before asking)", replay.contentCalls)
	}
	access, refresh := savedTokens(t, cfgPath)
	if access != "fresh-tok" || refresh != "ref-2" {
		t.Errorf("persisted pair = %q / %q, want the rotated one", access, refresh)
	}
}

// The stored expiry still looks fine but the server says 401 - the truth is the
// server's. One refresh, then the original request is replayed.
func TestServerSide401RefreshesAndReplays(t *testing.T) {
	replay := &refreshReplay{}
	srv := replay.start(t)

	out, cfgPath, err := runCapWith(t, srv.URL,
		"stale-tok", "ref-1", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, out)
	}
	if replay.contentCalls != 2 {
		t.Errorf("content calls = %d, want 2 (rejected, then replayed)", replay.contentCalls)
	}
	if replay.refreshCalls != 1 {
		t.Errorf("refresh calls = %d, want 1", replay.refreshCalls)
	}
	if !strings.Contains(out, "tiles") && !strings.Contains(out, "ok") {
		t.Errorf("the replayed request's answer did not reach the output:\n%s", out)
	}
	// No rotation in this replay: the old refresh token must survive the save.
	if _, refresh := savedTokens(t, cfgPath); refresh != "ref-1" {
		t.Errorf("refresh token = %q, want the original kept when none was issued", refresh)
	}
}

// A dead refresh token must come back as an auth failure that names the fix,
// not as a generic API error - and must not clear the day for a retry storm.
func TestDeclinedRefreshSaysSignInAgain(t *testing.T) {
	replay := &refreshReplay{}
	srv := replay.start(t)

	out, _, err := runCapWith(t, srv.URL,
		"stale-tok", "ref-dead", time.Now().Add(-time.Hour))
	if err == nil {
		t.Fatalf("a declined refresh was reported as success:\n%s", out)
	}
	if code := ExitCode(err); code != 4 {
		t.Errorf("exit code = %d, want 4 (auth)", code)
	}
	if !strings.Contains(err.Error(), "auth login") {
		t.Errorf("error = %q, want it to say how to restart the session", err)
	}
	if replay.contentCalls != 0 {
		t.Errorf("content calls = %d, want 0 (nothing to ask with)", replay.contentCalls)
	}
}

// A revoked account answers 401 to fresh tokens too. That must cost one refresh
// and end, not spin the token endpoint.
func TestARejectedFreshTokenDoesNotLoop(t *testing.T) {
	replay := &refreshReplay{alwaysReject: true}
	srv := replay.start(t)

	out, _, err := runCapWith(t, srv.URL,
		"stale-tok", "ref-1", time.Now().Add(time.Hour))
	if err == nil {
		t.Fatalf("expected the 401 to surface:\n%s", out)
	}
	if replay.refreshCalls != 1 {
		t.Errorf("refresh calls = %d, want exactly 1", replay.refreshCalls)
	}
	if replay.contentCalls != 2 {
		t.Errorf("content calls = %d, want 2 (once each side of the refresh)", replay.contentCalls)
	}
}

// Without a stored refresh token the old behaviour stands: the 401 surfaces and
// the token endpoint is never bothered.
func TestNoRefreshTokenMeansAPlain401(t *testing.T) {
	replay := &refreshReplay{}
	srv := replay.start(t)

	_, _, err := runCapWith(t, srv.URL,
		"stale-tok", "", time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("expected a 401")
	}
	if replay.refreshCalls != 0 {
		t.Errorf("refresh calls = %d, want 0", replay.refreshCalls)
	}
	if code := ExitCode(err); code != 4 {
		t.Errorf("exit code = %d, want 4 (auth)", code)
	}
}

// The refresh request must be the one the toolkit's frontend sends: JSON body,
// the grant, the stored token, and the toolkit's client id.
func TestRefreshSendsTheFrontendGrant(t *testing.T) {
	replay := &refreshReplay{}
	srv := replay.start(t)

	if out, _, err := runCapWith(t, srv.URL,
		"stale-tok", "ref-1", time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("command failed: %v\n%s", err, out)
	}
	if len(replay.refreshGrants) != 1 {
		t.Fatalf("refresh grants recorded = %d", len(replay.refreshGrants))
	}
	g := replay.refreshGrants[0]
	if g["grant_type"] != "refresh_token" || g["refresh_token"] != "ref-1" || g["client_id"] == "" {
		t.Errorf("grant = %v, want grant_type/refresh_token/client_id the way the frontend sends them", g)
	}
}

// The code exchange speaks JSON first - the encoding the frontend uses and the
// one that came with a refresh token - and only falls back to form encoding
// when JSON is refused.
func TestExchangeSpeaksJSONFirstAndFallsBackToForm(t *testing.T) {
	jsonReplay := &crossfitReplay{codeIn: "location"}
	withAPI(t, jsonReplay.start(t))
	if _, refresh, _, _, err := crossfitSignIn("me@example.com", "correct", 5*time.Second); err != nil {
		t.Fatalf("sign-in failed: %v", err)
	} else if refresh != "ref-xyz" {
		t.Errorf("refresh = %q", refresh)
	}
	if len(jsonReplay.tokenTypes) != 1 || !strings.HasPrefix(jsonReplay.tokenTypes[0], "application/json") {
		t.Errorf("token attempts = %v, want a single JSON one", jsonReplay.tokenTypes)
	}

	formReplay := &crossfitReplay{codeIn: "location", rejectJSONExchange: true}
	withAPI(t, formReplay.start(t))
	if _, _, _, _, err := crossfitSignIn("me@example.com", "correct", 5*time.Second); err != nil {
		t.Fatalf("sign-in with a form-only server failed: %v", err)
	}
	if n := len(formReplay.tokenTypes); n != 2 {
		t.Fatalf("token attempts = %d, want JSON then form", n)
	}
	if !strings.HasPrefix(formReplay.tokenTypes[1], "application/x-www-form-urlencoded") {
		t.Errorf("second attempt content type = %q, want form encoding", formReplay.tokenTypes[1])
	}
	if formReplay.tokenGrant["code"] != "auth-code-123" {
		t.Errorf("form fallback exchanged %q", formReplay.tokenGrant["code"])
	}
}

var _ = fmt.Sprintf // keep fmt imported if assertions above change

// --- Silent reauthorization -------------------------------------------------
//
// The refresh grant is disabled server-side (unauthorized_client, verified live
// even for the toolkit frontend's own token). What actually renews a session is
// the identity cookie signing a fresh authorization code. These tests replay
// that server.

type silentReplay struct {
	contentCalls   int
	grantAttempts  int
	authorizeCalls int
	exchanged      []string
	cookieSeen     string
}

func (r *silentReplay) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/subscriptions/v1/content", func(w http.ResponseWriter, req *http.Request) {
		r.contentCalls++
		if req.Header.Get("Authorization") != "Bearer fresh-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Unauthorized"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"count":1,"tiles":[{"title":"ok"}]}`))
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, req *http.Request) {
		r.authorizeCalls++
		r.cookieSeen = req.Header.Get("Cookie")
		if !strings.Contains(r.cookieSeen, "cf_session=sess-1") {
			// A dead identity session bounces to the login page, codeless.
			w.Header().Set("Location", "https://affiliate.crossfit.com/tools/login")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Location",
			"https://affiliate.crossfit.com/tools/redirect?code=renewed-code-9")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, req *http.Request) {
		grant := map[string]string{}
		_ = json.NewDecoder(req.Body).Decode(&grant)
		switch grant["grant_type"] {
		case "refresh_token":
			// The live server's answer, for everyone.
			r.grantAttempts++
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"unauthorized_client"}`))
		case "authorization_code":
			r.exchanged = append(r.exchanged, grant["code"])
			if grant["code"] != "renewed-code-9" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "fresh-tok", "refresh_token": "ref-next", "expires_in": 3600})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	oldTok, oldAuth := client.TokenURL, client.AuthorizeURL
	client.TokenURL = srv.URL + "/token"
	client.AuthorizeURL = srv.URL + "/authorize"
	t.Cleanup(func() { client.TokenURL, client.AuthorizeURL = oldTok, oldAuth })
	return srv
}

func runCapWithSession(t *testing.T, base, accessToken, refreshToken, sessionCookie string, expiry time.Time) (string, string, error) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := "base_url = \"" + base + "\"\n" +
		"access_token = \"" + accessToken + "\"\n" +
		"refresh_token = \"" + refreshToken + "\"\n"
	if !expiry.IsZero() {
		cfg += "token_expiry = " + expiry.Format(time.RFC3339) + "\n"
	}
	if sessionCookie != "" {
		cfg += "[auth_cookies]\ncf_session = \"" + sessionCookie + "\"\n"
	}
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--config", cfgPath, "--json", "--no-cache",
		"subscriptions", "get-content", "--urn", "content_api:///programming/affiliate/daily-class-plan/20260810"})
	err := root.Execute()
	return out.String(), cfgPath, err
}

// The everyday case on today's server: the grant is refused, the cookie route
// renews, the original request is replayed, and the fresh pair is persisted.
func TestSilentAuthorizeRenewsWhenTheGrantIsRefused(t *testing.T) {
	replay := &silentReplay{}
	srv := replay.start(t)

	out, cfgPath, err := runCapWithSession(t, srv.URL,
		"stale-tok", "ref-1", "sess-1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, out)
	}
	if replay.grantAttempts != 1 {
		t.Errorf("grant attempts = %d, want 1 (tried first, cheaper)", replay.grantAttempts)
	}
	if replay.authorizeCalls != 1 {
		t.Errorf("authorize calls = %d, want 1", replay.authorizeCalls)
	}
	if len(replay.exchanged) != 1 || replay.exchanged[0] != "renewed-code-9" {
		t.Errorf("exchanged codes = %v, want the renewed one", replay.exchanged)
	}
	access, refresh := savedTokens(t, cfgPath)
	if access != "fresh-tok" || refresh != "ref-next" {
		t.Errorf("persisted pair = %q / %q", access, refresh)
	}
}

// Today's stored state on real installs: no refresh token at all, only the
// identity cookies. The cookie route must carry the renewal alone.
func TestCookiesAloneRenewTheSession(t *testing.T) {
	replay := &silentReplay{}
	srv := replay.start(t)

	out, _, err := runCapWithSession(t, srv.URL,
		"stale-tok", "", "sess-1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, out)
	}
	if replay.grantAttempts != 0 {
		t.Errorf("grant attempts = %d, want 0 (nothing to send)", replay.grantAttempts)
	}
	if replay.authorizeCalls != 1 {
		t.Errorf("authorize calls = %d, want 1", replay.authorizeCalls)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("renewed request's answer missing:\n%s", out)
	}
}

// A dead identity session bounces the authorize to the login page. That must
// read as "sign in again", not as a mystery.
func TestDeadIdentitySessionSaysSignInAgain(t *testing.T) {
	replay := &silentReplay{}
	srv := replay.start(t)

	out, _, err := runCapWithSession(t, srv.URL,
		"stale-tok", "", "sess-DEAD", time.Now().Add(-time.Hour))
	if err == nil {
		t.Fatalf("a dead session was reported as success:\n%s", out)
	}
	if code := ExitCode(err); code != 4 {
		t.Errorf("exit code = %d, want 4 (auth)", code)
	}
	if !strings.Contains(err.Error(), "auth login") {
		t.Errorf("error = %q, want the fix named", err)
	}
}

// Sign-in must keep the identity cookies it is handed - they are the renewal.
func TestSignInKeepsTheIdentityCookies(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/v2/auth/signin", func(w http.ResponseWriter, req *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "cf_session", Value: "sess-77"})
		w.Header().Set("Location",
			"https://affiliate.crossfit.com/tools/redirect?code=auth-code-123")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/users/v2/auth/token", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"access_token":"tok-abc","expires_in":3600}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	withAPI(t, srv.URL)

	_, _, _, cookies, err := crossfitSignIn("me@example.com", "correct", 5*time.Second)
	if err != nil {
		t.Fatalf("sign-in failed: %v", err)
	}
	if cookies["cf_session"] != "sess-77" {
		t.Errorf("cookies = %v, want the identity session kept", cookies)
	}
}

// A token supplied by the environment belongs to its caller: the CLI must not
// spend the stored session renewing what it does not own, nor overwrite the
// stored pair while an env token is doing the talking. (Codex's catch.)
func TestEnvTokenIsNeverRenewed(t *testing.T) {
	replay := &silentReplay{}
	srv := replay.start(t)

	t.Setenv("CROSSFIT_AFFILIATE_PROGRAMMING_BEARER_AUTH", "env-owned-token")
	out, _, err := runCapWithSession(t, srv.URL,
		"stale-tok", "", "sess-1", time.Now().Add(-time.Hour))
	// The env token is not "fresh-tok", so the content endpoint answers 401 -
	// and that 401 must surface untouched.
	if err == nil {
		t.Fatalf("expected the env token's own 401 to surface:\n%s", out)
	}
	if replay.authorizeCalls != 0 || replay.grantAttempts != 0 {
		t.Errorf("renewal ran for an env-owned token: authorize=%d grant=%d",
			replay.authorizeCalls, replay.grantAttempts)
	}
}
