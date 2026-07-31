// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.

package btwbhtml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func planFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "plan_form.html"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return data
}

func TestParsePlanFormFindsTheWorkoutForm(t *testing.T) {
	form, err := ParsePlanForm(planFixture(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if form.Action != "/plan/workouts/multiple/rounds_for_time" {
		t.Errorf("Action = %q", form.Action)
	}
	// The page also carries a /plan/previews/track_events form holding a
	// track_event[track_id] of its own; picking it would post to the wrong place
	// with the wrong track.
	if got := form.Get("track_event[track_id]"); got != "101" {
		t.Errorf("track_id = %q, want the planning form's track, not the preview form's", got)
	}
}

// Every field btwb attaches to the form must be submitted, whether it sits
// inside <form> or is bound to it by the form="" attribute. Counting them is
// the cheapest guard against a serialiser that silently drops a whole class of
// field: the real page has 31, and a form-descendants-only walk finds 6.
func TestParsePlanFormCollectsFieldsInsideAndOutsideTheForm(t *testing.T) {
	form, err := ParsePlanForm(planFixture(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(form.Fields) != 31 {
		var names []string
		for _, f := range form.Fields {
			names = append(names, f.Name)
		}
		t.Fatalf("collected %d fields, want 31:\n%s", len(form.Fields), strings.Join(names, "\n"))
	}
	for _, f := range form.Fields {
		if f.Name == "definition[ignored]" {
			t.Error("a disabled field was submitted")
		}
	}
}

// definition[contents][] repeats with no index, so btwb reconstructs the
// movements from field order alone. If the serialiser reorders anything, the
// workout that gets planned is not the one that was reviewed.
func TestParsePlanFormPreservesMovementOrder(t *testing.T) {
	form, err := ParsePlanForm(planFixture(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var movements []string
	for _, f := range form.Fields {
		if f.Name == "definition[contents][][movementName]" {
			movements = append(movements, f.Value)
		}
	}
	want := []string{"Row", "Box Jump", "Burpee"}
	if len(movements) != len(want) {
		t.Fatalf("movements = %v, want %v", movements, want)
	}
	for i := range want {
		if movements[i] != want[i] {
			t.Fatalf("movements = %v, want %v", movements, want)
		}
	}

	// The encoded body must keep that order too. net/url sorts by key, which
	// would interleave the three movements into nonsense.
	body := form.Encode()
	iRow := strings.Index(body, "Row")
	iBox := strings.Index(body, "Box+Jump")
	iBurpee := strings.Index(body, "Burpee")
	if !(iRow < iBox && iBox < iBurpee) {
		t.Errorf("encoded body reordered the movements: Row@%d Box@%d Burpee@%d",
			iRow, iBox, iBurpee)
	}
}

func TestPlanFormSetReplacesWithoutDuplicating(t *testing.T) {
	form, err := ParsePlanForm(planFixture(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	before := len(form.Fields)

	form.Set("track_event[event_date]", "2026-09-01")
	form.Set("track_event[title]", "Test WOD")
	if got := form.Get("track_event[event_date]"); got != "2026-09-01" {
		t.Errorf("event_date = %q", got)
	}
	if got := form.Get("track_event[title]"); got != "Test WOD" {
		t.Errorf("title = %q", got)
	}
	if len(form.Fields) != before {
		t.Errorf("field count changed from %d to %d; Set must replace, not append",
			before, len(form.Fields))
	}

	count := 0
	for _, f := range form.Fields {
		if f.Name == "track_event[event_date]" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("event_date appears %d times, want 1", count)
	}

	// A name that is not in the form yet must be appended, not dropped.
	form.Set("track_event[brand_new]", "x")
	if got := form.Get("track_event[brand_new]"); got != "x" {
		t.Errorf("a new field was not appended: %q", got)
	}
}

// Without gym admin rights btwb offers exactly one track here, while the
// whiteboard's own track list shows every track the member can read. Confusing
// the two is what makes "why can't I plan into the gym track" hard to diagnose.
func TestParsePlannableTracksIgnoresThePlaceholder(t *testing.T) {
	tracks, err := ParsePlannableTracks(planFixture(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("tracks = %+v, want exactly one", tracks)
	}
	if tracks[0].ID != 101 || tracks[0].Name != "Coach Personal Track" {
		t.Errorf("track = %+v", tracks[0])
	}
}

func TestCSRFTokenPrefersTheMetaTag(t *testing.T) {
	token, err := CSRFToken(planFixture(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "SCRUBBED" {
		t.Errorf("token = %q", token)
	}

	if _, err := CSRFToken([]byte("<html><body>no token here</body></html>")); err == nil {
		t.Error("a page without a token must report an error")
	}
}

func TestParsePlanFormRejectsAPageWithoutAForm(t *testing.T) {
	_, err := ParsePlanForm([]byte(`<html><body>
	  <form action="/plan/previews/track_events" method="get"></form>
	</body></html>`))
	if err == nil {
		t.Fatal("expected an error when btwb returned no planning form")
	}
	if !strings.Contains(err.Error(), "no planning form") {
		t.Errorf("error = %v, want it to name the missing form", err)
	}
}

func TestPlanFormSummaryReadsBackTheMovements(t *testing.T) {
	form, err := ParsePlanForm(planFixture(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := form.Summary()
	if len(lines) != 3 {
		t.Fatalf("summary = %v, want one line per movement", lines)
	}
	if !strings.Contains(lines[0], "Row") || !strings.Contains(lines[0], "500") {
		t.Errorf("first line = %q, want the row distance", lines[0])
	}
	if !strings.HasPrefix(lines[2], "12 Burpee") {
		t.Errorf("last line = %q, want the rep count in front", lines[2])
	}
}
