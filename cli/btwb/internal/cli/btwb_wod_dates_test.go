// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// These pin the date regression: `wod day --date 2025-08-10` answered "date not
// present in page" and handed back the current fortnight instead. The cause was
// the route, not the date handling - /members/<id>/whiteboard/day ignores ?d=
// and always renders the current two weeks, while the bare /whiteboard route
// honours it. Every case here fails against the old route.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fortnightPage renders a whiteboard page covering the two weeks ending on
// `last`, in the markup shape the parser expects. The member-scoped routes are
// deliberately made to ignore the date, exactly as btwb does, so a test can
// tell the two routes apart.
func fortnightPage(last time.Time) string {
	var days strings.Builder
	for i := 13; i >= 0; i-- {
		d := last.AddDate(0, 0, -i).Format("2006-01-02")
		fmt.Fprintf(&days, `
      <div class="box box-day current-month">
        <a href="/members/249397/whiteboard/day?d=%s">%s</a>
        <div data-task="track_event" data-track-id="530228">
          <a href="/tasks/members/249397/track_events/9000%02d">Workout %s</a>
        </div>
      </div>`, d, d, i, d)
	}
	return `<!DOCTYPE html><html><body>
    <a href="/members/249397/tracks">tracks</a>
    <div class="box-week">` + days.String() + `</div>
  </body></html>`
}

// btwbDateReplay serves the whiteboard the way btwb really does: the bare
// /whiteboard route honours ?d=, the member-scoped routes do not.
func btwbDateReplay(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var requested []string
	current := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := r.URL.Query().Get("d")
		requested = append(requested, r.URL.Path+"?d="+d)

		switch {
		case r.URL.Path == "/whiteboard":
			last := current
			if d != "" {
				if parsed, err := time.Parse("2006-01-02", d); err == nil {
					last = parsed
				}
			}
			w.Write([]byte(fortnightPage(last)))
		case strings.HasSuffix(r.URL.Path, "/tracks"):
			w.Write([]byte(`<html><body><ul>
			  <li id="following_track_530228"><span class="track-name">CAP</span></li>
			</ul></body></html>`))
		default:
			// The member-scoped routes ignore the date - the behaviour that
			// caused the regression.
			w.Write([]byte(fortnightPage(current)))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &requested
}

func runWod(t *testing.T, base string, args ...string) (string, error) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := "base_url = \"" + base + "\"\nsession_cookie = \"test\"\nmember_id = 249397\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	root := RootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"--config", cfgPath, "--no-cache", "--json"}, args...))
	err := root.Execute()
	return out.String(), err
}

// The reported case: a date from a year ago must come back, not be reported
// missing with the current fortnight in its place.
func TestWodDayReadsAHistoricalDate(t *testing.T) {
	srv, requested := btwbDateReplay(t)

	out, err := runWod(t, srv.URL, "wod", "day", "--date", "2025-08-10")
	if err != nil {
		t.Fatalf("historical date failed: %v\n%s", err, out)
	}

	var env struct {
		Results struct {
			Date string `json:"date"`
		} `json:"results"`
	}
	if jErr := json.Unmarshal([]byte(out), &env); jErr != nil {
		t.Fatalf("output is not the {meta,results} envelope: %v\n%s", jErr, out)
	}
	if env.Results.Date != "2025-08-10" {
		t.Errorf("returned date = %q, want the date that was asked for", env.Results.Date)
	}

	// The date must survive into the request, on the route that honours it.
	var asked bool
	for _, r := range *requested {
		if r == "/whiteboard?d=2025-08-10" {
			asked = true
		}
	}
	if !asked {
		t.Errorf("no request carried the date to the working route; sent: %v", *requested)
	}
}

func TestWodWeekReadsAHistoricalDate(t *testing.T) {
	srv, _ := btwbDateReplay(t)

	out, err := runWod(t, srv.URL, "wod", "week", "--date", "2025-08-01")
	if err != nil {
		t.Fatalf("historical week failed: %v\n%s", err, out)
	}
	var env struct {
		Results struct {
			Days []struct {
				Date string `json:"date"`
			} `json:"days"`
		} `json:"results"`
	}
	if jErr := json.Unmarshal([]byte(out), &env); jErr != nil {
		t.Fatalf("output is not the expected envelope: %v\n%s", jErr, out)
	}
	if len(env.Results.Days) != 14 {
		t.Fatalf("returned %d days, want the full fortnight", len(env.Results.Days))
	}
	var found bool
	for _, d := range env.Results.Days {
		if d.Date == "2025-08-01" {
			found = true
		}
	}
	if !found {
		t.Errorf("window %s..%s does not contain the requested date",
			env.Results.Days[0].Date, env.Results.Days[len(env.Results.Days)-1].Date)
	}
}

// The same defect hid future dates: workouts visible in the mobile app were
// unreachable from the CLI.
func TestWodDayReadsAFutureDate(t *testing.T) {
	srv, _ := btwbDateReplay(t)

	out, err := runWod(t, srv.URL, "wod", "day", "--date", "2026-08-20")
	if err != nil {
		t.Fatalf("future date failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "2026-08-20") {
		t.Errorf("future date missing from the answer:\n%s", out)
	}
}

// Today's date must keep working, and must still go through the route that
// honours dates rather than falling back to the old one.
func TestWodTodayStillWorks(t *testing.T) {
	srv, requested := btwbDateReplay(t)

	if out, err := runWod(t, srv.URL, "wod", "today"); err != nil {
		t.Fatalf("today failed: %v\n%s", err, out)
	}
	for _, r := range *requested {
		if strings.Contains(r, "/whiteboard/day?d=") || strings.Contains(r, "/whiteboard/month?d=") {
			t.Errorf("a date-ignoring route was used: %s", r)
		}
	}
}

// A date btwb genuinely has nothing for must say so plainly, and name the
// window it did return - the message that made the regression diagnosable.
func TestMissingDateNamesTheWindow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tracks") {
			w.Write([]byte(`<html><body></body></html>`))
			return
		}
		// Whatever is asked, answer with a window that cannot contain it.
		w.Write([]byte(fortnightPage(time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC))))
	}))
	t.Cleanup(srv.Close)

	out, err := runWod(t, srv.URL, "wod", "day", "--date", "2001-01-01")
	if err == nil {
		t.Fatalf("expected a not-found error:\n%s", out)
	}
	if code := ExitCode(err); code != 3 {
		t.Errorf("exit code = %d, want 3 (not found)", code)
	}
	if !strings.Contains(err.Error(), "2026-08-16") {
		t.Errorf("error = %q, want it to name the window btwb returned", err)
	}
}
