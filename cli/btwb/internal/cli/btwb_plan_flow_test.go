// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"btwb-pp-cli/internal/btwbhtml"
)

// planFormPage is the page btwb returns once it has parsed a description. The
// fixture lives with the parser, since that is what it was captured for.
func planFormPage(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(
		"..", "btwbhtml", "testdata", "plan_form.html"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return data
}

// replayBtwb stands in for btwb, answering the three requests the planning flow
// makes and recording what was posted.
type replayBtwb struct {
	form        []byte
	createdBody url.Values
	createdPath string
	cookies     []string
}

func (r *replayBtwb) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/plan/track_events/workouts/new", func(w http.ResponseWriter, req *http.Request) {
		r.cookies = append(r.cookies, req.Header.Get("Cookie"))
		w.Write(r.form)
	})
	mux.HandleFunc("/plan/workouts/generated_workouts", func(w http.ResponseWriter, req *http.Request) {
		r.cookies = append(r.cookies, req.Header.Get("Cookie"))
		w.Write(r.form)
	})
	mux.HandleFunc("/plan/workouts/multiple/rounds_for_time", func(w http.ResponseWriter, req *http.Request) {
		body, _ := getBody(req)
		r.createdBody = body
		r.createdPath = req.URL.Path
		// The real btwb answers with a redirect whose TARGET carries the new
		// id; the edit page it lands on does not repeat it. The first release
		// of submitPlan read only the body and reported a verified-live false
		// failure, so this mock must redirect the way btwb does.
		http.Redirect(w, req, "/plan/track_events/workouts/303/edit",
			http.StatusFound)
	})
	mux.HandleFunc("/plan/track_events/workouts/303/edit", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<html><body>the edit page, with no id in a matchable shape</body></html>`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func getBody(req *http.Request) (url.Values, error) {
	if err := req.ParseForm(); err != nil {
		return nil, err
	}
	return req.PostForm, nil
}

func replaySession(t *testing.T, base string) *planHTTP {
	t.Helper()
	return &planHTTP{
		client:  &http.Client{Timeout: 5 * time.Second},
		base:    base,
		session: "test-session",
		timeout: 5 * time.Second,
	}
}

// The whole write path, end to end: fetch the page, hand btwb the text, replay
// the form it produced, and read back the id of what was created.
func TestPlanFlowPostsTheFormBtwbProduced(t *testing.T) {
	replay := &replayBtwb{form: planFormPage(t)}
	srv := replay.start(t)
	session := replaySession(t, srv.URL)

	page, err := session.get(planNewPath + "?d=2026-08-06")
	if err != nil {
		t.Fatalf("fetching the planning page: %v", err)
	}
	if !strings.Contains(replay.cookies[0], btwbSessionCookieName+"=test-session") {
		t.Errorf("session cookie was not sent: %q", replay.cookies[0])
	}

	form, err := session.generateWorkout("token", "3 rounds for time:\nRow, 500 m", 5*time.Second)
	if err != nil {
		t.Fatalf("generating the workout: %v", err)
	}
	form.Set("track_event[track_id]", "101")
	form.Set("track_event[event_date]", "2026-09-15")
	form.Set("track_event[title]", "Test WOD")

	eventID, err := session.submitPlan(form)
	if err != nil {
		t.Fatalf("submitting: %v", err)
	}
	if eventID != 303 {
		t.Errorf("event id = %d, want the id from the redirect target", eventID)
	}

	// The post must land on the action btwb chose, not on a path we invented.
	if replay.createdPath != "/plan/workouts/multiple/rounds_for_time" {
		t.Errorf("posted to %q", replay.createdPath)
	}

	// The overrides must be in the body, exactly once each.
	if got := replay.createdBody["track_event[event_date]"]; len(got) != 1 || got[0] != "2026-09-15" {
		t.Errorf("event_date in body = %v", got)
	}
	if got := replay.createdBody["track_event[title]"]; len(got) != 1 || got[0] != "Test WOD" {
		t.Errorf("title in body = %v", got)
	}

	// And btwb's own structure must survive untouched: three movements, in order.
	movements := replay.createdBody["definition[contents][][movementName]"]
	want := []string{"Row", "Box Jump", "Burpee"}
	if len(movements) != len(want) {
		t.Fatalf("movements in body = %v, want %v", movements, want)
	}
	for i := range want {
		if movements[i] != want[i] {
			t.Fatalf("movements in body = %v, want %v", movements, want)
		}
	}
	_ = page
}

// btwb serves the sign-in page with HTTP 200 when a session has gone stale, so
// a plain status check would read that as success and then fail to find a form,
// reporting a parse error instead of "log in again".
func TestPlanFlowDetectsAnExpiredSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<html><body>
		  <form action="/session" method="post">
		    <input name="login"><input name="password">
		  </form>
		</body></html>`))
	}))
	t.Cleanup(srv.Close)

	_, err := replaySession(t, srv.URL).get(planNewPath)
	if err == nil {
		t.Fatal("expected an error for an expired session")
	}
	if !strings.Contains(err.Error(), "auth login") {
		t.Errorf("error = %q, want it to tell the user to sign in again", err)
	}
}

// When btwb cannot parse the text there is no form to replay. Inventing one
// would plan an empty workout, so the flow has to stop.
func TestPlanFlowStopsWhenBtwbReturnsNoWorkout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<html><body><p>Sorry, I could not read that.</p></body></html>`))
	}))
	t.Cleanup(srv.Close)

	_, err := replaySession(t, srv.URL).generateWorkout("token", "asdfghjkl", time.Second)
	if err == nil {
		t.Fatal("expected an error when btwb returned no workout")
	}
	if !strings.Contains(err.Error(), "did not return a workout") {
		t.Errorf("error = %q", err)
	}
}

// btwb answers the POST with a frame it fills in a moment later. Giving up on
// the first look would make the command fail intermittently on slow parses.
func TestPlanFlowPollsWhileBtwbIsStillParsing(t *testing.T) {
	replay := &replayBtwb{form: planFormPage(t)}
	var calls int
	mux := http.NewServeMux()
	mux.HandleFunc("/plan/workouts/generated_workouts", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<html><body>
		  <turbo-frame id="gw" src="/plan/workouts/generated_workouts/401020"></turbo-frame>
		</body></html>`))
	})
	mux.HandleFunc("/plan/workouts/generated_workouts/401020", func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 2 {
			w.Write([]byte(`<html><body><turbo-frame id="gw">working</turbo-frame></body></html>`))
			return
		}
		w.Write(replay.form)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	form, err := replaySession(t, srv.URL).generateWorkout("token", "text", 10*time.Second)
	if err != nil {
		t.Fatalf("polling: %v", err)
	}
	if form.Action != "/plan/workouts/multiple/rounds_for_time" {
		t.Errorf("action = %q", form.Action)
	}
	if calls < 2 {
		t.Errorf("polled %d times, expected it to wait for the form", calls)
	}
}

// Reporting an id that was never seen would send the user looking for an event
// that may not exist.
func TestPlanFlowRefusesToInventAnEventID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<html><body>saved</body></html>`))
	}))
	t.Cleanup(srv.Close)

	form, err := parsePlanFormForTest(t)
	if err != nil {
		t.Fatal(err)
	}
	_, err = replaySession(t, srv.URL).submitPlan(form)
	if err == nil {
		t.Fatal("expected an error when btwb did not report the new event")
	}
	if !strings.Contains(err.Error(), "did not report its id") {
		t.Errorf("error = %q", err)
	}
}

func parsePlanFormForTest(t *testing.T) (*btwbhtml.PlanForm, error) {
	t.Helper()
	return btwbhtml.ParsePlanForm(planFormPage(t))
}
