// Copyright 2026 isolovyev. Licensed under Apache-2.0. See LICENSE.

// Package capjson turns the CrossFit content API's JSON into structured Go
// values, and classifies each day's movements into training patterns.
//
// It is pure data work: no network, no auth, no command wiring. Callers fetch
// the content envelope elsewhere and hand the bytes to Parse*. The point of the
// package is the second half - Day.Profile() reduces a day to the load/volume/
// skill numbers CAP already assigns plus a set of movement-pattern tags derived
// from the movements, so two days can be compared and, later, reordered.
package capjson

// Day is one daily class plan.
type Day struct {
	// Date is the YYYY-MM-DD the plan is programmed for.
	Date string `json:"date"`
	// Name is CAP's title for the day (a benchmark name, or the raw date code
	// like "260810" when the workout is unnamed).
	Name string `json:"name"`
	// Title is the document's own heading. On the affiliate track it repeats
	// the date, but on the sequence tracks it carries the context that Name
	// lacks: "Week 1, Day 1 - Pull-Up", "Murph - Day 1 (April 13)".
	Title string `json:"title,omitempty"`
	// Track is the programming track, e.g. "Affiliate".
	Tracks []string `json:"tracks,omitempty"`

	// Load, Volume, Skill are CAP's own 1-5 ratings. They are the coarse
	// vectors the reorder optimiser balances first. Zero means CAP left it blank.
	Load   int `json:"load"`
	Volume int `json:"volume"`
	Skill  int `json:"skill"`

	// Workouts is the prescription at each level (rx, intermediate, ...).
	Workouts []Workout `json:"workouts"`
	// Movements is the canonical movement list CAP publishes in its scaling
	// section - cleaner than parsing the free-text prescription.
	Movements []string `json:"movements"`

	// Stimulus is the coach-facing "intended stimulus" notes.
	Stimulus []string `json:"stimulus,omitempty"`
	// CoachingGoals is the coach-facing goals list.
	CoachingGoals []string `json:"coaching_goals,omitempty"`

	// ClassPlan is the timed lesson structure (whiteboard, warm-ups, workout,
	// cooldown), each block with its minutes and content.
	ClassPlan []Section `json:"class_plan,omitempty"`

	// Scaling holds the overview and per-movement substitution options.
	Scaling Scaling `json:"scaling,omitempty"`

	// Duration is CAP's total class length in minutes, as published.
	Duration string `json:"duration,omitempty"`
}

// Workout is one prescription level of a day.
type Workout struct {
	Level       string `json:"level"`
	Description string `json:"description"`
	Score       string `json:"score,omitempty"`
}

// Section is one timed block of the class plan.
type Section struct {
	Title    string `json:"title"`
	Minutes  int    `json:"minutes"`
	Blocks   []Block `json:"blocks,omitempty"`
}

// Block is one piece of content inside a section. CAP uses a flexible-content
// model with a handful of layouts; Kind names the layout and the fields carry
// whichever shape it produced.
type Block struct {
	Kind  string   `json:"kind"`            // wysiwyg | at_a_glance | table
	Title string   `json:"title,omitempty"`
	Text  string   `json:"text,omitempty"`  // wysiwyg body, tags stripped
	Items []string `json:"items,omitempty"` // at_a_glance bullet items
	Rows  [][]string `json:"rows,omitempty"` // table rows
}

// Scaling is the day's scaling guidance.
type Scaling struct {
	Overview     string              `json:"overview,omitempty"`
	Substitutions []MovementScaling  `json:"substitutions,omitempty"`
}

// MovementScaling is one movement and its published substitutions.
type MovementScaling struct {
	Movement      string `json:"movement"`
	Substitutions string `json:"substitutions,omitempty"`
}

// Week is a weekly overview document.
type Week struct {
	Date         string       `json:"date"`
	Name         string       `json:"name"`
	Overview     string       `json:"overview,omitempty"`
	WeeklyTip    string       `json:"weekly_tip,omitempty"`
	MonthlyFocus string       `json:"monthly_focus,omitempty"`
	Weaknesses   []NamedNote  `json:"work_your_weakness,omitempty"`
}

// NamedNote is a titled block of prose (used by "work your weakness").
type NamedNote struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}
