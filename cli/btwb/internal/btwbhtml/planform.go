// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.

package btwbhtml

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// PlanForm is btwb's "plan this workout" form, captured well enough to be
// submitted again.
//
// btwb builds the workout server-side: you post a text description, it answers
// with a page whose form already holds the parsed movements. This CLI never
// tries to compose that structure itself, it replays the form btwb produced and
// only overrides the few fields a caller actually chooses (track, date, title).
// That is why the fields are kept as an ordered list rather than a map:
// definition[contents][] repeats with no index, so btwb groups the movements by
// position alone and any reordering silently rewrites the workout.
type PlanForm struct {
	// Action is the path the form posts to, e.g.
	// /plan/workouts/multiple/rounds_for_time. It varies with the shape of the
	// workout btwb recognised.
	Action string
	Fields []FormField
}

// FormField is one name/value pair, in document order.
type FormField struct {
	Name  string
	Value string
}

// planFormActionPrefix marks the forms that create a workout. btwb picks the
// exact suffix (single, multiple/<type>, lifting_complex) from what it parsed.
const planFormActionPrefix = "/plan/workouts/"

// ParsePlanForm extracts the planning form from a page btwb returned after it
// parsed a workout description.
//
// The fields are collected in document order across the whole page, not just
// from inside <form>: btwb attaches most of them with the HTML form="<id>"
// attribute, so a serialiser that only walks the form's descendants would drop
// the track, the date and every movement.
func ParsePlanForm(page []byte) (*PlanForm, error) {
	doc, err := parseDoc(page)
	if err != nil {
		return nil, err
	}

	var formNode *html.Node
	var action string
	doc.Find("form").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		a, _ := s.Attr("action")
		if strings.HasPrefix(a, planFormActionPrefix) {
			formNode = s.Nodes[0]
			action = a
			return false
		}
		return true
	})
	if formNode == nil {
		return nil, fmt.Errorf("no planning form on the page: btwb did not return a workout to plan")
	}
	formID := nodeAttr(formNode, "id")

	form := &PlanForm{Action: action}
	doc.Find("input, select, textarea").Each(func(_ int, s *goquery.Selection) {
		if !belongsToForm(s, formNode, formID) {
			return
		}
		name, ok := s.Attr("name")
		if !ok || name == "" {
			return
		}
		if _, disabled := s.Attr("disabled"); disabled {
			return
		}
		value, include := fieldValue(s)
		if !include {
			return
		}
		form.Fields = append(form.Fields, FormField{Name: name, Value: value})
	})
	if len(form.Fields) == 0 {
		return nil, fmt.Errorf("the planning form at %s has no fields", action)
	}
	return form, nil
}

// belongsToForm implements the HTML form-owner rule: an explicit form="<id>"
// attribute wins, otherwise the nearest ancestor <form> owns the control.
func belongsToForm(s *goquery.Selection, formNode *html.Node, formID string) bool {
	if owner, ok := s.Attr("form"); ok {
		return formID != "" && owner == formID
	}
	closest := s.Closest("form")
	return len(closest.Nodes) > 0 && closest.Nodes[0] == formNode
}

// fieldValue returns the value a browser would submit, and whether the control
// is submitted at all. Buttons are excluded: they only contribute when clicked,
// and btwb's planning form does not name its submit button.
func fieldValue(s *goquery.Selection) (string, bool) {
	switch goquery.NodeName(s) {
	case "textarea":
		return s.Text(), true
	case "select":
		var value string
		found := false
		s.Find("option").EachWithBreak(func(_ int, opt *goquery.Selection) bool {
			if _, selected := opt.Attr("selected"); selected {
				value = optionValue(opt)
				found = true
				return false
			}
			return true
		})
		if !found {
			// A browser submits the first option when none is marked selected.
			if first := s.Find("option").First(); len(first.Nodes) > 0 {
				value = optionValue(first)
			}
		}
		return value, true
	default:
		typ := strings.ToLower(s.AttrOr("type", "text"))
		switch typ {
		case "submit", "button", "image", "reset", "file":
			return "", false
		case "checkbox", "radio":
			if _, checked := s.Attr("checked"); !checked {
				return "", false
			}
		}
		return s.AttrOr("value", ""), true
	}
}

func optionValue(opt *goquery.Selection) string {
	if v, ok := opt.Attr("value"); ok {
		return v
	}
	return strings.TrimSpace(opt.Text())
}

func nodeAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// Set replaces every occurrence of a field, or appends it when absent. Fields
// a caller sets (track, date, title) are single-valued, unlike the repeated
// definition[...] ones, so collapsing duplicates is the right behaviour.
func (f *PlanForm) Set(name, value string) {
	replaced := false
	out := f.Fields[:0]
	for _, field := range f.Fields {
		if field.Name != name {
			out = append(out, field)
			continue
		}
		if replaced {
			continue
		}
		field.Value = value
		out = append(out, field)
		replaced = true
	}
	f.Fields = out
	if !replaced {
		f.Fields = append(f.Fields, FormField{Name: name, Value: value})
	}
}

// Get returns the first value stored under a name.
func (f *PlanForm) Get(name string) string {
	for _, field := range f.Fields {
		if field.Name == name {
			return field.Value
		}
	}
	return ""
}

// Encode renders the fields as application/x-www-form-urlencoded, preserving
// order. net/url's Values.Encode sorts by key, which would scramble the
// movement grouping, so the encoding is done by hand.
func (f *PlanForm) Encode() string {
	var b strings.Builder
	for i, field := range f.Fields {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(field.Name))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(field.Value))
	}
	return b.String()
}

// Summary describes the workout the form would create, for a confirmation
// prompt. It reads the movements back out of the repeated definition fields.
func (f *PlanForm) Summary() []string {
	var lines []string
	var current string
	flush := func() {
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
	}
	for _, field := range f.Fields {
		switch field.Name {
		case "definition[contents][][movementName]":
			flush()
			current = field.Value
		case "definition[contents][][reps][value]":
			if current != "" {
				current = field.Value + " " + current
			}
		case "definition[contents][][distance][value]",
			"definition[contents][][weight][value]",
			"definition[contents][][height][value]":
			if current != "" {
				current += ", " + field.Value
			}
		case "definition[contents][][distance][unit]",
			"definition[contents][][weight][unit]",
			"definition[contents][][height][unit]":
			if current != "" && !strings.HasSuffix(current, " ") {
				current += " " + field.Value
			}
		}
	}
	flush()
	return lines
}

// ParsePlannableTracks lists the tracks the signed-in member may plan into.
//
// This is deliberately a different list from ParseTracks: that one reports the
// tracks whose programming the member can read, which for a gym member is many.
// Planning is restricted to the tracks btwb offers in the form's own select,
// which without gym admin rights is just the member's personal track.
func ParsePlannableTracks(page []byte) ([]Track, error) {
	doc, err := parseDoc(page)
	if err != nil {
		return nil, err
	}
	var tracks []Track
	doc.Find(`select[name="track_event[track_id]"] option`).Each(
		func(_ int, opt *goquery.Selection) {
			raw, ok := opt.Attr("value")
			if !ok || raw == "" {
				return // the "Choose a track" placeholder
			}
			id, convErr := strconv.Atoi(raw)
			if convErr != nil {
				return
			}
			tracks = append(tracks, Track{
				ID:        id,
				Name:      strings.TrimSpace(opt.Text()),
				Following: true,
			})
		})
	return tracks, nil
}

// CSRFToken returns the Rails CSRF token a page carries in its head, which
// every non-GET request to btwb must echo back.
func CSRFToken(page []byte) (string, error) {
	doc, err := parseDoc(page)
	if err != nil {
		return "", err
	}
	if token, ok := doc.Find(`meta[name="csrf-token"]`).First().Attr("content"); ok && token != "" {
		return token, nil
	}
	if token, ok := doc.Find(`input[name="authenticity_token"]`).First().Attr("value"); ok && token != "" {
		return token, nil
	}
	return "", fmt.Errorf("no CSRF token on the page")
}
