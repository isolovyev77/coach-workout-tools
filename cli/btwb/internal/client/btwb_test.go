// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"btwb-pp-cli/internal/config"
)

func TestIsWidgetURL(t *testing.T) {
	cases := map[string]bool{
		"https://webwidgets.prod.btwb.com/webwidgets/wods": true,
		"https://WEBWIDGETS.PROD.BTWB.COM/webwidgets/wods": true,
		"https://btwb.com/members/1/tracks":                false,
		"/members/1/tracks":                                false,
		"https://evil.example/webwidgets.prod.btwb.com":    false,
	}
	for target, want := range cases {
		if got := isWidgetURL(target); got != want {
			t.Errorf("isWidgetURL(%q) = %v, want %v", target, got, want)
		}
	}
}

// The Web Widgets API reads `date` as a JavaScript Date.toString() value, so an
// ISO date has to be rewritten before it goes out.
func TestNormalizeWidgetParamsRewritesISODate(t *testing.T) {
	const widgetURL = "https://webwidgets.prod.btwb.com/webwidgets/wods"
	in := map[string]string{"date": "2026-07-30", "days": "7"}
	out := normalizeWidgetParams(widgetURL, in)

	if in["date"] != "2026-07-30" {
		t.Errorf("input map was mutated: date = %q", in["date"])
	}
	if out["days"] != "7" {
		t.Errorf("days = %q, want 7", out["days"])
	}
	got := out["date"]
	// e.g. "Thu Jul 30 2026 23:59:00 GMT+0300 (MSK)"
	shape := regexp.MustCompile(
		`^[A-Z][a-z]{2} [A-Z][a-z]{2} \d{2} 2026 \d{2}:\d{2}:\d{2} GMT[+-]\d{4} \(.+\)$`)
	if !shape.MatchString(got) {
		t.Fatalf("date = %q, want a JavaScript Date.toString() shape", got)
	}
	if !strings.HasPrefix(got, "Thu Jul 30 2026 ") {
		t.Errorf("date = %q, want the same calendar day preserved", got)
	}
}

func TestNormalizeWidgetParamsLeavesOtherInputsAlone(t *testing.T) {
	const widgetURL = "https://webwidgets.prod.btwb.com/webwidgets/wods"
	already := "Sun Sep 14 2025 23:01:22 GMT+0200 (CEST)"
	if got := normalizeWidgetParams(widgetURL, map[string]string{"date": already}); got["date"] != already {
		t.Errorf("an upstream-shaped date was rewritten to %q", got["date"])
	}
	// Session endpoints have their own `d` param and must not be touched.
	session := map[string]string{"d": "2026-07-30"}
	got := normalizeWidgetParams("https://btwb.com/members/1/whiteboard/day", session)
	if got["d"] != "2026-07-30" {
		t.Errorf("session date rewritten to %q", got["d"])
	}
	if normalizeWidgetParams(widgetURL, nil) != nil {
		t.Error("nil params should stay nil")
	}
}

func TestClassifyBtwbPath(t *testing.T) {
	cases := []struct {
		path            string
		kind            string
		member, eventID int
	}{
		{"/members/100001/whiteboard/day", "day", 100001, 0},
		{"/members/100001/whiteboard/month", "month", 100001, 0},
		{"/members/100001/tracks", "tracks", 100001, 0},
		{"/members/100001/workout_sessions", "sessions", 100001, 0},
		{"/tasks/members/100001/track_events/323452610", "event", 100001, 323452610},
		{"/signin", "", 0, 0},
		{"/members/100001/movements", "", 0, 0},
	}
	for _, c := range cases {
		kind, member, event := classifyBtwbPath(c.path)
		if kind != c.kind || member != c.member || event != c.eventID {
			t.Errorf("classifyBtwbPath(%q) = (%q, %d, %d), want (%q, %d, %d)",
				c.path, kind, member, event, c.kind, c.member, c.eventID)
		}
	}
}

// An expired session is served the sign-in page with HTTP 200, so the only way
// to tell it apart from real content is to recognise the page.
func TestTransformBtwbResponseDetectsSignInPage(t *testing.T) {
	page := []byte(`<html><body><form action="/session" method="post">` +
		`<input name="login"><input name="password"></form></body></html>`)
	_, err := transformBtwbResponse("https://btwb.com/members/100001/tracks", nil, page)
	var needsLogin *ErrNeedsLogin
	if !errors.As(err, &needsLogin) {
		t.Fatalf("err = %v, want ErrNeedsLogin", err)
	}
}

func TestTransformBtwbResponsePassesWidgetJSONThrough(t *testing.T) {
	body := []byte(`{"wodsets":[]}`)
	out, err := transformBtwbResponse(
		"https://webwidgets.prod.btwb.com/webwidgets/wods", nil, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(body) {
		t.Errorf("widget JSON was rewritten to %q", out)
	}
}

// Paths this client does not model must pass through untouched, so probes and
// ad-hoc requests keep working.
func TestTransformBtwbResponseLeavesUnmodelledPathsAlone(t *testing.T) {
	body := []byte("<html>whatever</html>")
	out, err := transformBtwbResponse("https://btwb.com/members/1/movements", nil, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(body) {
		t.Errorf("body rewritten to %q", out)
	}
}

func TestTransformBtwbResponseParsesTracks(t *testing.T) {
	page := []byte(`<html><body><ul>
	  <li id="following_track_530228"><span class="track-name">Natrium WODs</span>
	    <a class="follow-track following" data-method="delete" href="#">Unfollow</a></li>
	</ul></body></html>`)
	out, err := transformBtwbResponse("https://btwb.com/members/100001/tracks", nil, page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var wrapper struct {
		Tracks []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal(out, &wrapper); err != nil {
		t.Fatalf("output is not the documented shape: %v", err)
	}
	if len(wrapper.Tracks) != 1 || wrapper.Tracks[0].ID != 530228 {
		t.Fatalf("tracks = %+v, want one track 530228", wrapper.Tracks)
	}
}

// btwb ignores a header named after its cookie; the session has to travel as a
// real Cookie, and the widget host has to get a bearer key instead.
func TestApplyBtwbAuthPicksCredentialByHost(t *testing.T) {
	clearBtwbEnv(t)
	cfg := &config.Config{BtwbSessionCookie: "sess-value", WidgetKey: "widget-value"}

	req, _ := http.NewRequest(http.MethodGet, "https://btwb.com/members/1/tracks", nil)
	if err := applyBtwbAuth(req, cfg, req.URL.String()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := req.Header.Get("Cookie"); got != "_btwb_session_id=sess-value" {
		t.Errorf("Cookie = %q", got)
	}
	if req.Header.Get("Authorization") != "" {
		t.Error("session request must not carry an Authorization header")
	}

	wreq, _ := http.NewRequest(http.MethodGet,
		"https://webwidgets.prod.btwb.com/webwidgets/wods", nil)
	if err := applyBtwbAuth(wreq, cfg, wreq.URL.String()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := wreq.Header.Get("Authorization"); got != "Bearer widget-value" {
		t.Errorf("Authorization = %q", got)
	}
	if got := wreq.Header.Get("Accept"); got != widgetAccept {
		t.Errorf("Accept = %q, want the vendor media type", got)
	}
	if wreq.Header.Get("Cookie") != "" {
		t.Error("widget request must not carry the member session cookie")
	}
}

// clearBtwbEnv makes the config file the only credential source, so an
// exported BTWB_* in the developer's shell cannot mask a broken lookup.
func clearBtwbEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"BTWB_SESSION_COOKIE", "BTWB_WIDGET_KEY", "BTWB_MEMBER_ID"} {
		t.Setenv(k, "")
	}
}

func TestApplyBtwbAuthReportsMissingCredentials(t *testing.T) {
	clearBtwbEnv(t)
	empty := &config.Config{}

	req, _ := http.NewRequest(http.MethodGet, "https://btwb.com/members/1/tracks", nil)
	var needsLogin *ErrNeedsLogin
	if err := applyBtwbAuth(req, empty, req.URL.String()); !errors.As(err, &needsLogin) {
		t.Errorf("err = %v, want ErrNeedsLogin", err)
	}

	wreq, _ := http.NewRequest(http.MethodGet,
		"https://webwidgets.prod.btwb.com/webwidgets/wods", nil)
	err := applyBtwbAuth(wreq, empty, wreq.URL.String())
	if err == nil || !strings.Contains(err.Error(), "widget key") {
		t.Errorf("err = %v, want a message naming the widget key", err)
	}
}

func TestApplyBtwbAuthSendsAllCookiesSavedByLogin(t *testing.T) {
	clearBtwbEnv(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`session_cookie = "short-session"

[session_cookies]
_btwb_session_id = "short-session"
remember_user_token = "long-session"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, "https://btwb.com/members/1/tracks", nil)
	if err := applyBtwbAuth(req, cfg, req.URL.String()); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Cookie"); !strings.Contains(got, "_btwb_session_id=short-session") ||
		!strings.Contains(got, "remember_user_token=long-session") {
		t.Errorf("Cookie = %q, want the whole saved browser session", got)
	}
}

func TestClientPersistsRotatedBtwbCookies(t *testing.T) {
	clearBtwbEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Cookie"); got != "_btwb_session_id=stale-session" {
			t.Errorf("Cookie = %q, want initial stored session", got)
		}
		http.SetCookie(w, &http.Cookie{Name: "_btwb_session_id", Value: "fresh-session", Path: "/", HttpOnly: true})
		http.SetCookie(w, &http.Cookie{Name: "remember_user_token", Value: "long-session", Path: "/", HttpOnly: true})
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := &config.Config{BaseURL: server.URL, Path: path}
	if err := cfg.SaveSession("stale-session", 100001); err != nil {
		t.Fatal(err)
	}
	c := New(cfg, time.Second, 0)
	c.NoCache = true
	if _, err := c.Get("/unmodelled", nil); err != nil {
		t.Fatalf("request carrying a rotating session failed: %v", err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "fresh-session") || !strings.Contains(string(saved), "remember_user_token") {
		t.Errorf("rotated browser cookies were not persisted for the next CLI process")
	}
}
