// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.

package capjson

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return data
}

func TestParseDayReadsTheWholeShape(t *testing.T) {
	day, err := ParseDay(fixture(t, "day-20260810.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if day.Date != "2026-08-10" {
		t.Errorf("date = %q, want the YYYYMMDD normalised to YYYY-MM-DD", day.Date)
	}
	if day.Load != 2 || day.Volume != 4 || day.Skill != 3 {
		t.Errorf("vectors = %d/%d/%d, want 2/4/3", day.Load, day.Volume, day.Skill)
	}
	if len(day.Workouts) == 0 || !strings.Contains(day.Workouts[0].Description, "box jump-over") {
		t.Errorf("workout[0] = %+v, want the box-jump-over couplet", day.Workouts)
	}
	want := []string{"Box jump-over", "Dumbbell deadlift", "Dumbbell hang power clean", "Dumbbell shoulder to overhead"}
	if len(day.Movements) != len(want) {
		t.Fatalf("movements = %v, want %v", day.Movements, want)
	}
	if len(day.Stimulus) == 0 {
		t.Error("intended stimulus was dropped")
	}
	// The class plan must survive with its timing.
	var haveWarmup, haveWorkout bool
	total := 0
	for _, s := range day.ClassPlan {
		total += s.Minutes
		if strings.Contains(s.Title, "warm_up") {
			haveWarmup = true
		}
		if s.Title == "workout" {
			haveWorkout = true
		}
	}
	if !haveWarmup || !haveWorkout {
		t.Errorf("class plan sections = %+v, want warm-up and workout blocks", day.ClassPlan)
	}
	if total == 0 {
		t.Error("class plan lost all its section minutes")
	}
}

// The 5K row is the degenerate case: one monostructural movement, no barbell,
// no gymnastics. It must parse and classify as pure engine, not empty.
func TestParseDayPureMonostructural(t *testing.T) {
	day, err := ParseDay(fixture(t, "day-20260811.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if day.Name != "5K Row" {
		t.Errorf("name = %q, want 5K Row", day.Name)
	}
	prof := day.Profile()
	if len(prof.Unknown) != 0 {
		t.Errorf("row should be classifiable, unknown = %v", prof.Unknown)
	}
	if !containsPattern(prof.Patterns, Mono) {
		t.Errorf("patterns = %v, want the mono engine pattern", prof.Patterns)
	}
	if !containsModality(prof.Modalities, Monostructural) {
		t.Errorf("modalities = %v, want monostructural", prof.Modalities)
	}
}

// HTML entities and tags in the prescription must be gone; a coach reads this.
func TestCleanTextStripsHTMLAndEntities(t *testing.T) {
	got := cleanText(`<p>15 box jump-overs&nbsp;(20/24 in)<br />12 DB deadlifts &#8211; go light</p>`)
	if strings.Contains(got, "<") || strings.Contains(got, "&") {
		t.Errorf("cleanText left markup: %q", got)
	}
	if !strings.Contains(got, "box jump-overs") {
		t.Errorf("cleanText dropped content: %q", got)
	}
}

func TestParseWeekOverview(t *testing.T) {
	wk, err := ParseWeek(fixture(t, "week-20260810.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wk.Date != "2026-08-10" {
		t.Errorf("date = %q", wk.Date)
	}
	if wk.Overview == "" {
		t.Error("weekly overview text was dropped")
	}
}

// An empty tiles list is how the API says "no plan for that date". It must be a
// clean sentinel, not a decode error, so the command can exit 3.
func TestParseDayNoContent(t *testing.T) {
	_, err := ParseDay([]byte(`{"count":0,"tiles":[]}`))
	if err != ErrNoContent {
		t.Errorf("err = %v, want ErrNoContent", err)
	}
}

// Older CAP cards stored programming_track as one string, whereas current
// cards use a list. Historical reads must support both formats.
func TestParseDayAcceptsLegacyStringProgrammingTrack(t *testing.T) {
	body := []byte(`{"count":1,"tiles":[{"title":"Legacy","acf":{"date":"20240812","name":"Legacy day","load":"1","volume":"1","skill":"1","duration":"","programming_track":"affiliate","workouts":[{"level":"rx","description":"Row, 500 m","score":"time"}]}}]}`)
	day, err := ParseDay(body)
	if err != nil {
		t.Fatalf("ParseDay legacy track: %v", err)
	}
	if got, want := day.Tracks, []string{"affiliate"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("tracks = %v, want %v", got, want)
	}
}
