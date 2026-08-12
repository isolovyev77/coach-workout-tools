// Copyright 2026 isolovyev. Licensed under Apache-2.0. See LICENSE.

package capjson

import (
	"os"
	"path/filepath"
	"testing"
)

func containsPattern(ps []Pattern, want Pattern) bool {
	for _, p := range ps {
		if p == want {
			return true
		}
	}
	return false
}

func containsModality(ms []Modality, want Modality) bool {
	for _, m := range ms {
		if m == want {
			return true
		}
	}
	return false
}

// The classifier's whole job is to put a movement in the right pattern. These
// cases pin the ones that decide collisions, including the traps: a thruster is
// two patterns, an erg row is engine-plus-pull, equipment prefixes must not
// change the pattern.
func TestClassifyMovement(t *testing.T) {
	cases := []struct {
		name     string
		modality Modality
		patterns []Pattern
	}{
		{"Back squat", Weightlifting, []Pattern{Squat}},
		{"Air squat", Weightlifting, []Pattern{Squat}},
		{"Deadlift", Weightlifting, []Pattern{Hinge}},
		{"Dumbbell deadlift", Weightlifting, []Pattern{Hinge}}, // equipment prefix ignored
		{"Power snatch", Weightlifting, []Pattern{Olympic, Hinge}},
		{"Dumbbell hang power clean", Weightlifting, []Pattern{Olympic, Hinge}},
		{"Shoulder press", Weightlifting, []Pattern{VerticalPush}},
		{"Push press", Weightlifting, []Pattern{VerticalPush}},
		{"Dumbbell shoulder to overhead", Weightlifting, []Pattern{VerticalPush}},
		{"Toes-to-bar", Gymnastics, []Pattern{Core, VerticalPull}},
		{"Rope climb", Gymnastics, []Pattern{VerticalPull}},
		{"Pull-up", Gymnastics, []Pattern{VerticalPull}},
		{"Deficit strict handstand push-up", Gymnastics, []Pattern{VerticalPush}},
		{"Row", Monostructural, []Pattern{Mono, HorizontalPull}},
		{"Echo bike", Monostructural, []Pattern{Mono}},
		{"Run", Monostructural, []Pattern{Mono}},
		{"Weighted run", Monostructural, []Pattern{Mono}}, // "weighted" stripped
		{"Double-under", Monostructural, []Pattern{Mono}},
		{"Box jump-over", Gymnastics, []Pattern{Squat, Mono}},
		{"Single-dumbbell walking lunge", Weightlifting, []Pattern{Squat}},
		{"Burpee", Gymnastics, []Pattern{HorizontalPush, Mono}},
	}
	for _, c := range cases {
		got := ClassifyMovement(c.name)
		if got.Modality != c.modality {
			t.Errorf("%q modality = %q, want %q", c.name, got.Modality, c.modality)
		}
		if !samePatterns(got.Patterns, c.patterns) {
			t.Errorf("%q patterns = %v, want %v", c.name, got.Patterns, c.patterns)
		}
	}
}

func samePatterns(a, b []Pattern) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[Pattern]bool{}
	for _, p := range a {
		set[p] = true
	}
	for _, p := range b {
		if !set[p] {
			return false
		}
	}
	return true
}

// Every movement in every fixture must classify. An unknown movement is not a
// test failure in production (new names appear), but across the captured week
// it flags a gap in the rules worth filling.
func TestNoUnknownMovementsAcrossFixtures(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "day-*.json"))
	if len(files) == 0 {
		t.Fatal("no day fixtures found")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		day, err := ParseDay(data)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if u := day.Profile().Unknown; len(u) != 0 {
			t.Errorf("%s: unclassified movements %v - add a rule", filepath.Base(f), u)
		}
	}
}

// The collision score is what drives the future reorder optimiser and what
// `cap compare` shows. Two representative real days must score sensibly:
//   - two hinge/olympic-heavy days collide (shared patterns push the score up)
//   - a max-load strength day and a pure engine day do not (few shared patterns,
//     and their load/volume vectors sit far apart)
func TestCollisionScoreSeparatesSimilarFromComplementary(t *testing.T) {
	load := func(name string) Profile {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		day, err := ParseDay(data)
		if err != nil {
			t.Fatal(err)
		}
		return day.Profile()
	}

	// 0810 (DB couplet: hinge + olympic + squat + vertical push) vs
	// 0816 (snatches + double-unders: olympic + hinge + mono). These share the
	// olympic/hinge load and should read as a collision.
	similar := load("day-20260810.json").CollisionScore(load("day-20260816.json"))

	// 0814 (1RM back squat / press / deadlift: pure strength, load 5 volume 1)
	// vs 0811 (5K row: pure engine, load 1 volume 5). Opposite vectors, almost
	// no shared pattern. This is the pairing a coach WANTS on consecutive days.
	complementary := load("day-20260814.json").CollisionScore(load("day-20260811.json"))

	if similar.Score <= complementary.Score {
		t.Errorf("collision scoring inverted: similar=%d (%v) complementary=%d (%v)",
			similar.Score, similar.SharedPatterns,
			complementary.Score, complementary.SharedPatterns)
	}
	if complementary.VectorDistance <= similar.VectorDistance {
		t.Errorf("vector distance did not separate the pairs: similar=%d complementary=%d",
			similar.VectorDistance, complementary.VectorDistance)
	}
}

// CAP lists only the movements that need scaling, so a day whose engine work
// (a run, a row) has no scaling entry would otherwise profile as if it had no
// conditioning at all. Verified against a real day: 2026-08-25 prescribes a
// 400-m run but its scaling section names only pull-ups, HSPU and goblet squats.
func TestProfileReadsMovementsMissingFromScaling(t *testing.T) {
	day := &Day{
		Movements: []string{"Chest-to-bar pull-up", "Handstand push-up", "Dumbbell goblet squat"},
		Workouts: []Workout{{
			Level:       "rx",
			Description: "2 rounds for total reps:\nOn a 3:00 clock:\n400-m run\nMax chest-to-bar pull-ups\n1:00 rest",
		}},
	}
	prof := day.Profile()
	if !containsPattern(prof.Patterns, Mono) {
		t.Errorf("patterns = %v, want the run to register as mono", prof.Patterns)
	}
	if !containsPattern(prof.Patterns, VerticalPull) {
		t.Errorf("patterns = %v, want the scaling-listed pull-up kept", prof.Patterns)
	}
}

// Word-boundary matching: a substring inside a longer word must not fire.
func TestClassifyTextMatchesWholeWordsOnly(t *testing.T) {
	_, pats := ClassifyText("athletes should feel impressive control in a narrow stance")
	for _, p := range pats {
		if p == VerticalPush || p == Mono || p == HorizontalPull {
			t.Errorf("substring inside a word produced pattern %q from prose", p)
		}
	}
	_, pats = ClassifyText("500-m row, then 20 push presses")
	if !containsPattern(pats, Mono) || !containsPattern(pats, VerticalPush) {
		t.Errorf("real movements were missed: %v", pats)
	}
}
