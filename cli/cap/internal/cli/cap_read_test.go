// Copyright 2026 isolovyev. Licensed under Apache-2.0. See LICENSE.

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
)

// capReplay serves the captured content fixtures at the real content path,
// keyed by the date embedded in the requested URN, so the whole command path
// runs against real API shapes without a token.
func capReplay(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urn := r.URL.Query().Get("urn")
		var name string
		switch {
		case strings.Contains(urn, "weekly-overview"):
			name = "week-" + dateFromURN(urn) + ".json"
		case strings.Contains(urn, "/murph/"):
			name = "track-murph-day1.json"
		case strings.Contains(urn, "/pull-up/"):
			name = "track-pullup-day1.json"
		case strings.Contains(urn, "daily-class-plan"):
			name = "day-" + dateFromURN(urn) + ".json"
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		data, err := os.ReadFile(filepath.Join("..", "capjson", "testdata", name))
		if err != nil {
			// A date with no fixture behaves like the real "no plan" answer.
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"count":0,"tiles":[]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dateFromURN(urn string) string {
	i := strings.LastIndex(urn, "/")
	if i < 0 {
		return ""
	}
	return urn[i+1:]
}

// runCap points a fresh root command at the replay server via a temp config and
// captures stdout.
func runCap(t *testing.T, base string, args ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(
		"base_url = \""+base+"\"\naccess_token = \"test-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	full := append([]string{"--config", cfgPath, "--json"}, args...)
	root.SetArgs(full)
	err := root.Execute()
	return out.String(), err
}

func TestCapDayEndToEnd(t *testing.T) {
	srv := capReplay(t)
	out, err := runCap(t, srv.URL, "cap", "day", "2026-08-10")
	if err != nil {
		t.Fatalf("cap day: %v\n%s", err, out)
	}
	var env struct {
		Results struct {
			Name    string `json:"name"`
			Load    int    `json:"load"`
			Profile struct {
				Patterns []string `json:"patterns"`
			} `json:"profile"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output is not the {meta,results} envelope: %v\n%s", err, out)
	}
	if env.Results.Load != 2 {
		t.Errorf("load = %d, want 2", env.Results.Load)
	}
	if len(env.Results.Profile.Patterns) == 0 {
		t.Error("day output carried no movement patterns")
	}
}

// The signal that makes reordering possible: two olympic/hinge days collide
// harder than a strength day paired with an engine day. This must hold through
// the whole command, not just the pure classifier.
func TestCapCompareRanksCollisions(t *testing.T) {
	srv := capReplay(t)

	similar, err := runCap(t, srv.URL, "cap", "compare", "2026-08-10", "2026-08-16")
	if err != nil {
		t.Fatalf("compare similar: %v\n%s", err, similar)
	}
	complementary, err := runCap(t, srv.URL, "cap", "compare", "2026-08-14", "2026-08-11")
	if err != nil {
		t.Fatalf("compare complementary: %v\n%s", err, complementary)
	}

	scoreOf := func(s string) int {
		var env struct {
			Results struct {
				Overlap struct {
					Score int `json:"score"`
				} `json:"overlap"`
			} `json:"results"`
		}
		if err := json.Unmarshal([]byte(s), &env); err != nil {
			t.Fatalf("bad compare output: %v\n%s", err, s)
		}
		return env.Results.Overlap.Score
	}
	if scoreOf(similar) <= scoreOf(complementary) {
		t.Errorf("collision ranking wrong: similar=%d complementary=%d",
			scoreOf(similar), scoreOf(complementary))
	}
}

// A date with no published plan must exit 3 (not found), not crash or exit 5.
func TestCapDayNotFound(t *testing.T) {
	srv := capReplay(t)
	out, err := runCap(t, srv.URL, "cap", "day", "2019-01-01")
	if err == nil {
		t.Fatalf("expected a not-found error, got success:\n%s", out)
	}
	if code := ExitCode(err); code != 3 {
		t.Errorf("exit code = %d, want 3 (not found)", code)
	}
}

func TestCapWarmupEndToEnd(t *testing.T) {
	srv := capReplay(t)
	out, err := runCap(t, srv.URL, "cap", "warmup", "2026-08-10")
	if err != nil {
		t.Fatalf("cap warmup: %v\n%s", err, out)
	}
	if !strings.Contains(out, "warm_up") {
		t.Errorf("warmup output has no warm-up section:\n%s", out)
	}
}

// Every published programme sits behind the same URN, differing only in the
// track segment and whether the id is a date or a sequence position. A coach
// asking for "day 1 of Murph" must not need a different command.
func TestCapDayReadsHeroAndSkillTracks(t *testing.T) {
	srv := capReplay(t)

	for _, c := range []struct{ track, sel, want string }{
		{"murph", "1", "Murph"},
		{"murph", "day-1", "Murph"},
		{"pull-up", "1", "Pull-Up"},
	} {
		out, err := runCap(t, srv.URL, "cap", "day", c.sel, "--track", c.track)
		if err != nil {
			t.Fatalf("track %s %s: %v\n%s", c.track, c.sel, err, out)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("track %s: output does not mention %q:\n%s", c.track, c.want, out)
		}
	}
}

// A sequence track has no calendar. Passing a date must say so plainly instead
// of building a URN that quietly 404s.
func TestCapDayRejectsADateOnASequenceTrack(t *testing.T) {
	srv := capReplay(t)
	out, err := runCap(t, srv.URL, "cap", "day", "2026-08-10", "--track", "murph")
	if err == nil {
		t.Fatalf("expected a usage error, got:\n%s", out)
	}
	if code := ExitCode(err); code != 2 {
		t.Errorf("exit code = %d, want 2 (usage)", code)
	}
	if !strings.Contains(err.Error(), "day number") {
		t.Errorf("error = %q, want it to explain the day-number form", err)
	}
}
