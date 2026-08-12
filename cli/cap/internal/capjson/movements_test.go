// Copyright 2026 isolovyev. Licensed under Apache-2.0. See LICENSE.

package capjson

import (
	"strings"
	"testing"
)

// The point of the movement pages is coaching material: what goes wrong and
// what to say. A parse that returns the video and drops the faults is useless.
func TestParseMovementKeepsFaultsAndCues(t *testing.T) {
	m, err := ParseMovement(fixture(t, "movement-air-squat.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "Air Squat" || m.Slug != "air-squat" {
		t.Errorf("name/slug = %q/%q", m.Name, m.Slug)
	}
	if m.Description == "" {
		t.Error("overview description was dropped")
	}
	if len(m.Faults) != 5 {
		t.Fatalf("faults = %d, want the 5 published for the air squat", len(m.Faults))
	}

	// Every fault must carry at least one cue, and no HTML may survive.
	for _, f := range m.Faults {
		if f.Fault == "" {
			t.Error("a fault came through with no description")
		}
		if len(f.Cues) == 0 {
			t.Errorf("fault %q has no cues, which is the useful half", f.Fault)
		}
		for _, c := range f.Cues {
			if strings.ContainsAny(c, "<>") {
				t.Errorf("cue kept markup: %q", c)
			}
		}
	}

	// The multi-cue fault is the one worth pinning: "weight shifting into the
	// toes" ships two different cues, and a parser that takes only the first
	// would look correct on every other fault.
	var multi bool
	for _, f := range m.Faults {
		if len(f.Cues) > 1 {
			multi = true
		}
	}
	if !multi {
		t.Error("no fault kept more than one cue; the corrections list was truncated")
	}

	if len(m.Progressions) == 0 || len(m.Progressions[0].Steps) == 0 {
		t.Error("learning progressions were dropped")
	}
}

// The movement catalogue is how a coach finds the slug to look up. Its entries
// must carry a usable slug, not just a display name.
func TestParseCatalogMovementsHaveSlugs(t *testing.T) {
	cat, err := ParseCatalog(fixture(t, "movements-catalog.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat.Count() < 70 {
		t.Errorf("catalog has %d movements, want the full published list", cat.Count())
	}
	var withSlug int
	for _, s := range cat.Sections {
		for _, e := range s.Entries {
			if e.Slug != "" {
				withSlug++
			}
		}
	}
	if withSlug != cat.Count() {
		t.Errorf("%d of %d movements have no slug, so they cannot be looked up",
			cat.Count()-withSlug, cat.Count())
	}

	// The sections are the CrossFit modalities; losing them loses the grouping.
	if len(cat.Sections) < 3 {
		t.Errorf("sections = %d, want gymnastics/weightlifting/monostructural", len(cat.Sections))
	}
}

// The benchmarks page is a sectioned list whose entries carry both a name and
// the workout text, and search has to reach both: a coach asks "what is Murph"
// but also "which benchmark has thrusters".
//
// The fixture is deliberately small and its workout texts are synthetic - the
// real page is CrossFit's subscription content and does not belong in a public
// repository. What is asserted here is the parser's mechanics, not the size of
// CrossFit's catalogue.
func TestParseCatalogBenchmarksCarryWorkoutText(t *testing.T) {
	cat, err := ParseCatalog(fixture(t, "benchmarks.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cat.Sections) < 3 {
		t.Errorf("sections = %d, want the benchmark/hero/open grouping kept",
			len(cat.Sections))
	}
	if cat.Count() == 0 {
		t.Fatal("benchmarks page parsed to zero entries")
	}

	byName := cat.FindEntries("murph")
	if len(byName) == 0 {
		t.Fatal("search by name found nothing")
	}
	if byName[0].Body == "" {
		t.Error("benchmark entry has no workout text")
	}

	// Search must look inside the body too, not just at names.
	if len(cat.FindEntries("thruster")) == 0 {
		t.Error("search does not reach the workout text")
	}
}

func TestParseCatalogHandlesResourcePages(t *testing.T) {
	for _, name := range []string{"programming-resources.json", "kids-teens.json"} {
		cat, err := ParseCatalog(fixture(t, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if cat.Count() == 0 {
			t.Errorf("%s: parsed to zero entries", name)
		}
	}
}

func TestParseMovementNoContent(t *testing.T) {
	if _, err := ParseMovement([]byte(`{"tiles":[]}`)); err != ErrNoContent {
		t.Errorf("err = %v, want ErrNoContent", err)
	}
}
