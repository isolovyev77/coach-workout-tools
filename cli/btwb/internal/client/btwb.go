// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. btwb serves HTML, not JSON, on the member-facing endpoints,
// and it authenticates them with a session cookie while the Web Widgets host
// authenticates with a bearer key. Both concerns live here so the generated
// client stays a plain transport.

package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"btwb-pp-cli/internal/btwbhtml"
	"btwb-pp-cli/internal/config"
)

// widgetHost is btwb's Web Widgets API. Requests to it carry the gym's widget
// key, never the member session cookie.
const widgetHost = "webwidgets.prod.btwb.com"

// widgetAccept is the vendor media type the Web Widgets API requires. Without
// it the API answers 406.
const widgetAccept = "application/vnd.btwb.v1.webwidgets+json"

// sessionCookieName is the current name of btwb's Rails session cookie.
// The site changed it from _btwb_session to _btwb_session_id in July 2026.
const sessionCookieName = "_btwb_session_id"

// ErrNeedsLogin reports that no usable session was available, or that btwb
// bounced the request to the sign-in page. Callers map this to exit code 4.
type ErrNeedsLogin struct{ Reason string }

func (e *ErrNeedsLogin) Error() string {
	if e.Reason == "" {
		return "not signed in: run 'btwb-pp-cli auth login'"
	}
	return e.Reason + ": run 'btwb-pp-cli auth login'"
}

// isWidgetURL reports whether a target URL addresses the Web Widgets API.
func isWidgetURL(target string) bool {
	u, err := url.Parse(target)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, widgetHost)
}

// applyBtwbAuth attaches the right credential for the host being addressed.
// The generated client would otherwise send the session value as a header
// literally named after the cookie, which btwb ignores.
func applyBtwbAuth(req *http.Request, cfg *config.Config, targetURL string) error {
	if isWidgetURL(targetURL) {
		key := cfg.WidgetKeyValue()
		if key == "" {
			return fmt.Errorf("no widget key: run 'btwb-pp-cli auth set-widget-key <key>' " +
				"(gym admin: gym menu -> Website Integration), or set BTWB_WIDGET_KEY")
		}
		req.Header.Set("Authorization", "Bearer "+key)
		if req.Header.Get("Accept") == "" {
			req.Header.Set("Accept", widgetAccept)
		}
		return nil
	}

	session := cfg.SessionValue()
	if session == "" {
		return &ErrNeedsLogin{}
	}
	// Preserve any cookie the caller already set.
	if existing := req.Header.Get("Cookie"); existing != "" &&
		!strings.Contains(existing, sessionCookieName+"=") {
		req.Header.Set("Cookie", existing+"; "+sessionCookieName+"="+session)
	} else {
		req.Header.Set("Cookie", sessionCookieName+"="+session)
	}
	return nil
}

// widgetDateLayout is the shape the Web Widgets API expects in `date`: a
// JavaScript Date.toString() value, because the browser widget passes its
// `data-date` attribute through untouched. Callers of this CLI give an ISO date.
const widgetDateLayout = "Mon Jan 02 2006 15:04:05 GMT-0700 (MST)"

// normalizeWidgetParams rewrites an ISO `date` into the JavaScript form the Web
// Widgets API requires. A value that is already in that form is left alone.
func normalizeWidgetParams(targetURL string, params map[string]string) map[string]string {
	if !isWidgetURL(targetURL) || params == nil {
		return params
	}
	raw, ok := params["date"]
	if !ok || raw == "" {
		return params
	}
	day, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		// Not an ISO date: assume the caller knows the upstream format.
		return params
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		out[k] = v
	}
	// btwb treats `date` as the last day of the window, inclusive, and reads it
	// as a wall-clock moment; end of day keeps that day in range.
	out["date"] = day.Add(23*time.Hour + 59*time.Minute).Format(widgetDateLayout)
	return out
}

// signinFormRe matches the sign-in page btwb redirects to when a session has
// expired. The transport follows redirects, so an expired session otherwise
// looks like a successful 200 carrying an unparseable page.
var signinFormRe = regexp.MustCompile(`<form[^>]+action="/session"`)

var (
	dayPathRe      = regexp.MustCompile(`^/members/(\d+)/whiteboard/day$`)
	monthPathRe    = regexp.MustCompile(`^/members/(\d+)/whiteboard/month$`)
	tracksPathRe   = regexp.MustCompile(`^/members/(\d+)/tracks$`)
	sessionsPathRe = regexp.MustCompile(`^/members/(\d+)/workout_sessions$`)
	eventPathRe    = regexp.MustCompile(`^/tasks/members/(\d+)/track_events/(\d+)$`)
)

// transformBtwbResponse converts an HTML response from a member-facing endpoint
// into the JSON documented in openapi.yaml. Responses that are already JSON
// (the Web Widgets API) pass through untouched.
func transformBtwbResponse(targetURL string, params map[string]string, body []byte) ([]byte, error) {
	if isWidgetURL(targetURL) {
		return body, nil
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		return body, nil
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		path = "/"
	}

	// Only rewrite paths this client actually models; everything else (probe
	// requests, /signin, ad-hoc paths) is left alone.
	kind, memberID, eventID := classifyBtwbPath(path)
	if kind == "" {
		return body, nil
	}

	if signinFormRe.Match(body) {
		return nil, &ErrNeedsLogin{Reason: "btwb redirected to the sign-in page (session expired)"}
	}

	switch kind {
	case "day":
		date := params["d"]
		if date == "" {
			date = u.Query().Get("d")
		}
		day, err := btwbhtml.ParseDay(body, memberID, date, nil)
		if err != nil {
			return nil, fmt.Errorf("reading whiteboard for %s: %w", date, err)
		}
		return json.Marshal(day)

	case "month":
		days, err := btwbhtml.ParseWeeks(body, memberID, nil)
		if err != nil {
			return nil, fmt.Errorf("reading whiteboard: %w", err)
		}
		return json.Marshal(map[string]any{"member_id": memberID, "days": days})

	case "event":
		wod, err := btwbhtml.ParseEvent(body, eventID)
		if err != nil {
			return nil, fmt.Errorf("reading workout %d: %w", eventID, err)
		}
		return json.Marshal(wod)

	case "tracks":
		tracks, err := btwbhtml.ParseTracks(body)
		if err != nil {
			return nil, fmt.Errorf("reading tracks: %w", err)
		}
		return json.Marshal(map[string]any{"tracks": tracks})

	case "sessions":
		sessions, err := btwbhtml.ParseSessions(body)
		if err != nil {
			return nil, fmt.Errorf("reading logged results: %w", err)
		}
		return json.Marshal(map[string]any{"sessions": sessions})
	}
	return body, nil
}

// classifyBtwbPath names the endpoint a path addresses, plus the ids embedded
// in it. An empty kind means "not an endpoint this client rewrites".
func classifyBtwbPath(path string) (kind string, memberID, eventID int) {
	atoi := func(s string) int {
		n, _ := strconv.Atoi(s)
		return n
	}
	if m := dayPathRe.FindStringSubmatch(path); m != nil {
		return "day", atoi(m[1]), 0
	}
	if m := monthPathRe.FindStringSubmatch(path); m != nil {
		return "month", atoi(m[1]), 0
	}
	if m := eventPathRe.FindStringSubmatch(path); m != nil {
		return "event", atoi(m[1]), atoi(m[2])
	}
	if m := tracksPathRe.FindStringSubmatch(path); m != nil {
		return "tracks", atoi(m[1]), 0
	}
	if m := sessionsPathRe.FindStringSubmatch(path); m != nil {
		return "sessions", atoi(m[1]), 0
	}
	return "", 0, 0
}
