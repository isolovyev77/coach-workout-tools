// Package btwbhtml converts btwb.com HTML pages into structured Go values.
//
// It is pure parsing: no network access, no authentication, no command wiring.
// Callers fetch the bytes elsewhere and hand them to the Parse* functions.
//
// btwb renders its pages in the viewer's locale, so every selector in this
// package is structural (class names, attributes, URL shapes). No parsing
// decision depends on English or Russian wording; where a number is embedded in
// a localised label it is pulled out with a digit regexp instead.
package btwbhtml

// WhiteboardDay is a single calendar day of the member's whiteboard.
type WhiteboardDay struct {
	Date     string     `json:"date"`
	MemberID int        `json:"member_id"`
	Workouts []WodEntry `json:"workouts"`
}

// WodEntry is one entry in a whiteboard day: either a workout planned by a
// track ("planned") or a result the member logged ("logged").
type WodEntry struct {
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	TrackID    int    `json:"track_id"`
	TrackName  string `json:"track_name,omitempty"`
	EventID    int    `json:"event_id"`
	DetailPath string `json:"detail_path"`
}

// Wod is the detail view of a single whiteboard entry.
type Wod struct {
	EventID        int      `json:"event_id"`
	TrackName      string   `json:"track_name,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Variant        string   `json:"variant,omitempty"`
	Category       string   `json:"category,omitempty"`
	Name           string   `json:"name,omitempty"`
	Description    string   `json:"description,omitempty"`
	Instructions   string   `json:"instructions,omitempty"`
	ResultsCount   int      `json:"results_count"`
	ResultsPath    string   `json:"results_path,omitempty"`
	LogPath        string   `json:"log_path,omitempty"`
	PreviousResult string   `json:"previous_result,omitempty"`
	Movements      []string `json:"movements,omitempty"`
}

// Track is a programming track the member can follow.
type Track struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Following bool   `json:"following"`
}

// LoggedResult is one entry from the member's list of logged results.
type LoggedResult struct {
	SessionID    int    `json:"session_id"`
	Date         string `json:"date,omitempty"`
	WorkoutName  string `json:"workout_name"`
	Result       string `json:"result,omitempty"`
	IsPrescribed bool   `json:"is_prescribed"`
	Notes        string `json:"notes,omitempty"`
	DetailPath   string `json:"detail_path"`
}
