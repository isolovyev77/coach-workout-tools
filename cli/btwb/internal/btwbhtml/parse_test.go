package btwbhtml

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testMemberID = 100001

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// trackNamesFromFixture builds the id -> name map the way a caller would: by
// parsing the tracks page first.
func trackNamesFromFixture(t *testing.T) map[int]string {
	t.Helper()
	tracks, err := ParseTracks(fixture(t, "tracks.html"))
	if err != nil {
		t.Fatalf("ParseTracks: %v", err)
	}
	names := make(map[int]string, len(tracks))
	for _, tr := range tracks {
		names[tr.ID] = tr.Name
	}
	return names
}

func TestParseWeeksCoversBothWeeks(t *testing.T) {
	want := []string{
		"2026-07-20", "2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24",
		"2026-07-25", "2026-07-26", "2026-07-27", "2026-07-28", "2026-07-29",
		"2026-07-30", "2026-07-31", "2026-08-01", "2026-08-02",
	}

	for _, name := range []string{"day_2026-07-30.html", "month_2026-07.html"} {
		t.Run(name, func(t *testing.T) {
			days, err := ParseWeeks(fixture(t, name), testMemberID, nil)
			if err != nil {
				t.Fatalf("ParseWeeks: %v", err)
			}
			if len(days) != len(want) {
				t.Fatalf("got %d days, want %d", len(days), len(want))
			}
			for i, date := range want {
				if days[i].Date != date {
					t.Errorf("days[%d].Date = %q, want %q", i, days[i].Date, date)
				}
				if days[i].MemberID != testMemberID {
					t.Errorf("days[%d].MemberID = %d, want %d", i, days[i].MemberID, testMemberID)
				}
				if len(days[i].Workouts) == 0 {
					t.Errorf("days[%d] (%s) has no workouts", i, days[i].Date)
				}
			}

			// Every entry in the page carries a data-task attribute, so the
			// parsed total must match the raw count exactly: no entry dropped,
			// none counted twice.
			raw := bytes.Count(fixture(t, name), []byte(`data-task="`))
			total := 0
			for _, d := range days {
				total += len(d.Workouts)
			}
			if total != raw {
				t.Errorf("parsed %d entries, page has %d data-task attributes", total, raw)
			}
		})
	}
}

func TestParseDayEntries(t *testing.T) {
	page := fixture(t, "day_2026-07-30.html")
	names := trackNamesFromFixture(t)

	tests := []struct {
		name  string
		date  string
		want  WodEntry
		total int // expected number of entries on that day
	}{
		{
			name:  "planned strength workout",
			date:  "2026-07-20",
			total: 23,
			want: WodEntry{
				Kind:       KindPlanned,
				Title:      "Rx'd / Intermediate / Masters 55+ Deadlift : 1-1-1-1-1-1-1",
				TrackID:    200001,
				TrackName:  "CAP",
				EventID:    323452610,
				DetailPath: "/tasks/members/100001/track_events/323452610",
			},
		},
		{
			name:  "logged result",
			date:  "2026-07-20",
			total: 23,
			want: WodEntry{
				Kind:       KindLogged,
				Title:      "Every 1 min for 8 mins: Push Press",
				TrackID:    200002,
				TrackName:  "Personal Test Track",
				EventID:    130283272,
				DetailPath: "/tasks/members/100001/workout_sessions/130283272",
			},
		},
		{
			name:  "rest day",
			date:  "2026-07-23",
			total: 12,
			want: WodEntry{
				Kind:       KindPlanned,
				Title:      "День отдыха",
				TrackID:    694246,
				TrackName:  "CAP - Compete",
				EventID:    323462448,
				DetailPath: "/tasks/members/100001/track_events/323462448",
			},
		},
		{
			name:  "coach note",
			date:  "2026-07-23",
			total: 12,
			want: WodEntry{
				Kind:       KindPlanned,
				Title:      "Daily Brief",
				TrackID:    200001,
				TrackName:  "CAP",
				EventID:    323440474,
				DetailPath: "/tasks/members/100001/track_events/323440474",
			},
		},
		{
			name:  "ampersand in title is decoded",
			date:  "2026-07-25",
			total: 14,
			want: WodEntry{
				Kind:       KindPlanned,
				Title:      "Gym #1 - h pull strength & hypertrophy Every 2:30 for 10 mins: Supinated Barbell Bent Over Rows",
				TrackID:    530409,
				TrackName:  "Natrium Gym",
				EventID:    321541719,
				DetailPath: "/tasks/members/100001/track_events/321541719",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			day, err := ParseDay(page, testMemberID, tc.date, names)
			if err != nil {
				t.Fatalf("ParseDay(%s): %v", tc.date, err)
			}
			if day.Date != tc.date {
				t.Fatalf("Date = %q, want %q", day.Date, tc.date)
			}
			if len(day.Workouts) != tc.total {
				t.Errorf("got %d entries for %s, want %d", len(day.Workouts), tc.date, tc.total)
			}
			var got *WodEntry
			for i := range day.Workouts {
				if day.Workouts[i].EventID == tc.want.EventID {
					got = &day.Workouts[i]
					break
				}
			}
			if got == nil {
				t.Fatalf("event %d not found on %s (entries: %+v)", tc.want.EventID, tc.date, day.Workouts)
			}
			if *got != tc.want {
				t.Errorf("entry mismatch\n got: %+v\nwant: %+v", *got, tc.want)
			}
		})
	}
}

// The 23rd is the day that mixes a rest day with logged results, so assert the
// whole trio the caller depends on.
func TestParseDayRestDayAndLoggedResults(t *testing.T) {
	day, err := ParseDay(fixture(t, "day_2026-07-30.html"), testMemberID, "2026-07-23", nil)
	if err != nil {
		t.Fatalf("ParseDay: %v", err)
	}

	byID := map[int]WodEntry{}
	for _, w := range day.Workouts {
		byID[w.EventID] = w
	}

	for _, want := range []struct {
		id   int
		kind string
	}{
		{323462448, KindPlanned}, // rest_day
		{130359780, KindLogged},
		{130359803, KindLogged},
	} {
		got, ok := byID[want.id]
		if !ok {
			t.Errorf("event %d missing from 2026-07-23", want.id)
			continue
		}
		if got.Kind != want.kind {
			t.Errorf("event %d Kind = %q, want %q", want.id, got.Kind, want.kind)
		}
		if got.TrackName != "" {
			t.Errorf("event %d TrackName = %q, want empty when trackNames is nil", want.id, got.TrackName)
		}
	}

	if got := byID[130359780].Title; got != "Every 1 min for 10 mins: Hang Snatch" {
		t.Errorf("130359780 Title = %q", got)
	}
	if got := byID[130359803].Title; got != "FT: Row Calories, Burpee Over Rowers, AbMat Sit-ups, and 6 more" {
		t.Errorf("130359803 Title = %q", got)
	}
	if got := byID[130359780].DetailPath; got != "/tasks/members/100001/workout_sessions/130359780" {
		t.Errorf("130359780 DetailPath = %q", got)
	}
}

func TestParseDayMissingDate(t *testing.T) {
	_, err := ParseDay(fixture(t, "day_2026-07-30.html"), testMemberID, "2026-08-03", nil)
	if err == nil {
		t.Fatal("expected an error for a date the page does not cover")
	}
	if !strings.Contains(err.Error(), "2026-08-03") {
		t.Errorf("error should name the missing date, got: %v", err)
	}
}

func TestParseWeeksRejectsNonWhiteboard(t *testing.T) {
	if _, err := ParseWeeks([]byte("<html><body><p>login</p></body></html>"), testMemberID, nil); err == nil {
		t.Fatal("expected an error for a page with no weeks")
	}
}

func TestParseEventWorkout(t *testing.T) {
	wod, err := ParseEvent(fixture(t, "event_workout.html"), 323452610)
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}

	checks := []struct {
		field, got, want string
	}{
		{"TrackName", wod.TrackName, "CAP"},
		{"Kind", wod.Kind, "Workout"},
		{"Variant", wod.Variant, "Rx'd / Intermediate / Masters 55+"},
		{"Category", wod.Category, "FL Силовые подъемы"},
		{"Name", wod.Name, "Deadlift : 1-1-1-1-1-1-1"},
		{"ResultsPath", wod.ResultsPath, "/track_events/323452610"},
		{"LogPath", wod.LogPath, "/workouts/69-deadlift-1-1-1-1-1-1-1/workout_sessions/new?d=2026-07-20"},
		{"PreviousResult", wod.PreviousResult, ""},
		{"Instructions", wod.Instructions, ""},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
		}
	}

	if wod.EventID != 323452610 {
		t.Errorf("EventID = %d", wod.EventID)
	}
	if wod.ResultsCount != 14 {
		t.Errorf("ResultsCount = %d, want 14", wod.ResultsCount)
	}
	if len(wod.Movements) != 1 || wod.Movements[0] != "Deadlift" {
		t.Errorf("Movements = %q, want [Deadlift]", wod.Movements)
	}

	if !strings.HasPrefix(wod.Description, "Deadlift 1-1-1-1-1-1-1") {
		t.Errorf("Description should start with the workout line, got %q", wod.Description)
	}
	lines := strings.Split(wod.Description, "\n")
	found := false
	for _, l := range lines {
		if l == "Use the heaviest weight you can for each set." {
			found = true
		}
	}
	if !found {
		t.Errorf("Description should keep %q on its own line, got lines %q",
			"Use the heaviest weight you can for each set.", lines)
	}
	if strings.Contains(wod.Description, "\r") {
		t.Errorf("Description should have CRLF normalised, got %q", wod.Description)
	}
	if strings.HasPrefix(wod.Description, " ") || strings.HasSuffix(wod.Description, "\n") {
		t.Errorf("Description should be trimmed, got %q", wod.Description)
	}
}

func TestParseEventMetcon(t *testing.T) {
	wod, err := ParseEvent(fixture(t, "event_metcon.html"), 323458859)
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if wod.TrackName != "CAP" || wod.Kind != "Workout" {
		t.Errorf("TrackName/Kind = %q/%q, want CAP/Workout", wod.TrackName, wod.Kind)
	}
	if wod.Variant != "Rx'd" {
		t.Errorf("Variant = %q, want Rx'd", wod.Variant)
	}
	if wod.Category != "" {
		t.Errorf("Category = %q, want empty (no span.label in this fragment)", wod.Category)
	}
	if wod.Name != "5 RFT: Row Calories and AbMat Sit-ups" {
		t.Errorf("Name = %q", wod.Name)
	}
	if wod.Description != "5 rounds for time of:\n30/25 Row Calories\n30 AbMat Sit-ups" {
		t.Errorf("Description = %q", wod.Description)
	}
	if wod.ResultsCount != 11 || wod.ResultsPath != "/track_events/323458859" {
		t.Errorf("results = %d %q, want 11 /track_events/323458859", wod.ResultsCount, wod.ResultsPath)
	}
	if wod.LogPath == "" {
		t.Error("LogPath is empty")
	}
	want := []string{"Row Calorie", "AbMat Sit-up"}
	if len(wod.Movements) != len(want) {
		t.Fatalf("Movements = %q, want %q", wod.Movements, want)
	}
	for i := range want {
		if wod.Movements[i] != want[i] {
			t.Errorf("Movements[%d] = %q, want %q", i, wod.Movements[i], want[i])
		}
	}
}

func TestParseEventSparseFragments(t *testing.T) {
	tests := []struct {
		fixtureName  string
		eventID      int
		wantTrack    string
		wantKind     string
		wantName     string
		wantInstrPfx string
	}{
		{
			fixtureName:  "event_note.html",
			eventID:      323440474,
			wantTrack:    "CAP",
			wantKind:     "Заметка",
			wantName:     "Daily Brief",
			wantInstrPfx: "Предполагаемая нагрузка",
		},
		{
			fixtureName: "event_restday.html",
			eventID:     323462448,
			wantTrack:   "CAP - Compete",
			wantKind:    "День отдыха",
			wantName:    "День отдыха",
		},
	}

	for _, tc := range tests {
		t.Run(tc.fixtureName, func(t *testing.T) {
			wod, err := ParseEvent(fixture(t, tc.fixtureName), tc.eventID)
			if err != nil {
				t.Fatalf("ParseEvent must not fail on a sparse fragment: %v", err)
			}
			if wod.EventID != tc.eventID {
				t.Errorf("EventID = %d, want %d", wod.EventID, tc.eventID)
			}
			if wod.TrackName != tc.wantTrack {
				t.Errorf("TrackName = %q, want %q", wod.TrackName, tc.wantTrack)
			}
			if wod.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", wod.Kind, tc.wantKind)
			}
			if wod.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", wod.Name, tc.wantName)
			}
			if tc.wantInstrPfx == "" {
				if wod.Instructions != "" {
					t.Errorf("Instructions = %q, want empty", wod.Instructions)
				}
			} else if !strings.HasPrefix(wod.Instructions, tc.wantInstrPfx) {
				t.Errorf("Instructions = %q, want prefix %q", wod.Instructions, tc.wantInstrPfx)
			}
			// Sparse fragments carry none of the workout machinery.
			if wod.Description != "" || wod.ResultsCount != 0 || wod.ResultsPath != "" ||
				wod.LogPath != "" || wod.PreviousResult != "" || len(wod.Movements) != 0 {
				t.Errorf("expected zero values for the workout fields, got %+v", *wod)
			}
		})
	}
}

func TestParseEventLoggedResultFragment(t *testing.T) {
	// The logged-result fragment shares the .task-event-details wrapper, so it
	// must parse rather than error even though its shape differs.
	wod, err := ParseEvent(fixture(t, "session_detail.html"), 130283272)
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	if wod.Kind != "Result" {
		t.Errorf("Kind = %q, want Result", wod.Kind)
	}
	if wod.Variant != "Every 1 min for 8 mins: Push Press" {
		t.Errorf("Variant = %q", wod.Variant)
	}
	if !strings.HasPrefix(wod.Description, "Every 1 min for 8 mins:") {
		t.Errorf("Description = %q", wod.Description)
	}
	if len(wod.Movements) != 1 || wod.Movements[0] != "Push Press" {
		t.Errorf("Movements = %q, want [Push Press]", wod.Movements)
	}
}

func TestParseEventRejectsNonFragment(t *testing.T) {
	if _, err := ParseEvent([]byte(`<div class="something-else">nope</div>`), 1); err == nil {
		t.Fatal("expected an error for html that is not a task detail fragment")
	}
}

func TestParseTracks(t *testing.T) {
	tracks, err := ParseTracks(fixture(t, "tracks.html"))
	if err != nil {
		t.Fatalf("ParseTracks: %v", err)
	}
	if len(tracks) != 17 {
		t.Errorf("got %d tracks, want 17", len(tracks))
	}

	byID := map[int]Track{}
	for _, tr := range tracks {
		if _, dup := byID[tr.ID]; dup {
			t.Errorf("duplicate track id %d", tr.ID)
		}
		byID[tr.ID] = tr
	}

	wantNames := map[int]string{
		530228:  "Natrium WODs",
		200001:  "CAP",
		1021085: "Natrium Weightlifting",
		694246:  "CAP - Compete",
		530409:  "Natrium Gym",
		200002:  "Personal Test Track",
		559736:  "Natrium & Linchpin WODs *",
	}
	for id, want := range wantNames {
		got, ok := byID[id]
		if !ok {
			t.Errorf("track %d missing", id)
			continue
		}
		if got.Name != want {
			t.Errorf("track %d Name = %q, want %q", id, got.Name, want)
		}
	}

	wantFollowing := map[int]bool{
		530228:  true, // toggle carries class "following"
		200001:  true,
		1021085: true,
		2:       false, // toggle without "following"
		134619:  false,
		951856:  false,
		1118491: false, // no follow control at all
	}
	for id, want := range wantFollowing {
		got, ok := byID[id]
		if !ok {
			t.Errorf("track %d missing", id)
			continue
		}
		if got.Following != want {
			t.Errorf("track %d Following = %v, want %v", id, got.Following, want)
		}
	}
}

func TestParseTracksRejectsNonTracksPage(t *testing.T) {
	if _, err := ParseTracks([]byte("<html><body><p>login</p></body></html>")); err == nil {
		t.Fatal("expected an error for a page with no tracks")
	}
}

func TestParseSessions(t *testing.T) {
	results, err := ParseSessions(fixture(t, "sessions.html"))
	if err != nil {
		t.Fatalf("ParseSessions: %v", err)
	}
	if len(results) != 15 {
		t.Fatalf("got %d results, want 15", len(results))
	}

	first := results[0]
	want := LoggedResult{
		SessionID:    130464021,
		Date:         "2026-07-28",
		WorkoutName:  "2x RFT: Rows",
		Result:       "14 mins 31 secs | (7 mins 13 secs) and (7 mins 18 secs) | Rx'd",
		IsPrescribed: true,
		Notes:        "",
		DetailPath:   "/tasks/members/100001/workout_sessions/130464021",
	}
	if first != want {
		t.Errorf("results[0] mismatch\n got: %+v\nwant: %+v", first, want)
	}

	byID := map[int]LoggedResult{}
	for _, r := range results {
		if r.SessionID == 0 {
			t.Errorf("result with zero SessionID: %+v", r)
		}
		if r.WorkoutName == "" {
			t.Errorf("session %d has no workout name", r.SessionID)
		}
		if r.DetailPath == "" {
			t.Errorf("session %d has no detail path", r.SessionID)
		}
		byID[r.SessionID] = r
	}

	// Scaled result: the flag segment reads "Not Rx'd".
	scaled, ok := byID[130283284]
	if !ok {
		t.Fatal("session 130283284 missing")
	}
	if scaled.IsPrescribed {
		t.Errorf("session 130283284 IsPrescribed = true, want false (result %q)", scaled.Result)
	}
	if scaled.Date != "2026-07-20" {
		t.Errorf("session 130283284 Date = %q, want 2026-07-20", scaled.Date)
	}

	// Only two entries in this fixture carry a member note.
	wantNotes := map[int]string{
		130218139: "HSW + pirouette practice",
		130198359: "205reps (9kg, 24kg)",
	}
	for id, note := range wantNotes {
		got, ok := byID[id]
		if !ok {
			t.Errorf("session %d missing", id)
			continue
		}
		if got.Notes != note {
			t.Errorf("session %d Notes = %q, want %q", id, got.Notes, note)
		}
	}
	notes := 0
	for _, r := range results {
		if r.Notes != "" {
			notes++
		}
	}
	if notes != len(wantNotes) {
		t.Errorf("got %d results with notes, want %d", notes, len(wantNotes))
	}

	// Dates must come back in ISO form and descending order (newest first, as
	// the feed renders them).
	for i := 1; i < len(results); i++ {
		if results[i-1].Date < results[i].Date {
			t.Errorf("feed order broken at %d: %q before %q", i, results[i-1].Date, results[i].Date)
		}
	}
}

func TestParseSessionsRejectsNonSessionsPage(t *testing.T) {
	if _, err := ParseSessions([]byte("<html><body><p>login</p></body></html>")); err == nil {
		t.Fatal("expected an error for a page with no sessions")
	}
}

func TestParseFeedDate(t *testing.T) {
	tests := []struct{ in, want string }{
		{"28 июля 2026", "2026-07-28"},
		{"1 января 2025", "2025-01-01"},
		{"July 28, 2026", "2026-07-28"},
		{"Результаты - 20 июля 2026", "2026-07-20"},
		{"2026-07-28", "2026-07-28"},
		{"", ""},
		{"вчера", "вчера"}, // unresolvable: returned verbatim, never dropped
	}
	for _, tc := range tests {
		if got := parseFeedDate(tc.in); got != tc.want {
			t.Errorf("parseFeedDate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsPrescribed(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"14 mins 31 secs | Rx'd", true},
		{"12 mins 22 secs | Not Rx'd", false},
		{"1635 kg | 60 kg, 65 kg | Rx'd", true},
		{"Completed", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isPrescribed(tc.in); got != tc.want {
			t.Errorf("isPrescribed(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParenCount(t *testing.T) {
	tests := []struct {
		in    string
		want  int
		wantK bool
	}{
		{"Посмотреть результаты (14)", 14, true},
		{"View results (1 772)", 1772, true},
		{"CrossFit Affiliate Programming Результаты", 0, false},
		{"AMReps 10 mins (1,2,3,...): Power Snatches", 123, true},
	}
	for _, tc := range tests {
		got, ok := parenCount(tc.in)
		if got != tc.want || ok != tc.wantK {
			t.Errorf("parenCount(%q) = %d,%v want %d,%v", tc.in, got, ok, tc.want, tc.wantK)
		}
	}
}
