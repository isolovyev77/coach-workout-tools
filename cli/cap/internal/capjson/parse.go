// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.

package capjson

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// envelope is the content API's list wrapper. CAP queries return one tile.
type envelope struct {
	Count int    `json:"count"`
	Tiles []tile `json:"tiles"`
}

type tile struct {
	Title string          `json:"title"`
	ACF   json.RawMessage `json:"acf"`
}

// firstACF pulls the single tile's acf payload out of the envelope, or reports
// that the document was empty (which is how the API answers "no plan for that
// date": HTTP 200 with an empty tiles list).
func firstACF(body []byte) (json.RawMessage, string, error) {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, "", fmt.Errorf("decoding content envelope: %w", err)
	}
	if len(env.Tiles) == 0 {
		return nil, "", ErrNoContent
	}
	return env.Tiles[0].ACF, env.Tiles[0].Title, nil
}

// ErrNoContent reports that the API returned no tile for a URN, i.e. there is
// no programming for that date. Callers map it to exit code 3 (not found).
var ErrNoContent = fmt.Errorf("no programming published for that date")

// ParseDay turns a daily-class-plan content envelope into a Day.
func ParseDay(body []byte) (*Day, error) {
	raw, title, err := firstACF(body)
	if err != nil {
		return nil, err
	}
	var a dayACF
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("decoding daily class plan: %w", err)
	}

	day := &Day{
		Date:     normaliseDate(a.Date),
		Name:     cleanText(a.Name),
		Title:    cleanText(title),
		Tracks:   a.ProgrammingTrack,
		Load:     atoi(a.Load),
		Volume:   atoi(a.Volume),
		Skill:    atoi(a.Skill),
		Duration: a.Duration,
	}

	for _, w := range a.Workouts {
		day.Workouts = append(day.Workouts, Workout{
			Level:       w.Level,
			Description: cleanText(w.Description),
			Score:       w.Score,
		})
	}
	for _, s := range a.AboutTheWorkout.IntendedStimulus {
		if t := cleanText(s.Stimulus); t != "" {
			day.Stimulus = append(day.Stimulus, t)
		}
	}
	for _, g := range a.AboutTheWorkout.CoachingGoals {
		if t := cleanText(g.Goal); t != "" {
			day.CoachingGoals = append(day.CoachingGoals, t)
		}
	}

	day.Scaling.Overview = cleanText(a.Scaling.Overview)
	for _, ms := range a.Scaling.MovementScalingOptions {
		mv := cleanText(ms.Movement)
		if mv == "" {
			continue
		}
		day.Movements = append(day.Movements, mv)
		day.Scaling.Substitutions = append(day.Scaling.Substitutions, MovementScaling{
			Movement:      mv,
			Substitutions: cleanText(ms.Substitutions),
		})
	}

	for _, cp := range a.ClassPlan {
		day.ClassPlan = append(day.ClassPlan, parseSection(cp))
	}
	return day, nil
}

// ParseWeek turns a weekly-overview content envelope into a Week.
func ParseWeek(body []byte) (*Week, error) {
	raw, _, err := firstACF(body)
	if err != nil {
		return nil, err
	}
	var a weekACF
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("decoding weekly overview: %w", err)
	}
	week := &Week{
		Date:         normaliseDate(a.Date),
		Name:         cleanText(a.Name),
		Overview:     cleanText(a.AtAGlance.Overview),
		WeeklyTip:    cleanText(a.AtAGlance.WeeklyTip),
		MonthlyFocus: cleanText(a.MonthlyFocus.Subheading + " " + a.MonthlyFocus.BodyText),
	}
	for _, w := range a.WorkYourWeakness {
		week.Weaknesses = append(week.Weaknesses, NamedNote{
			Title: cleanText(w.Title),
			Body:  cleanText(w.Body),
		})
	}
	return week, nil
}

func parseSection(cp classPlanACF) Section {
	s := Section{Title: cp.Title, Minutes: atoi(cp.Duration)}
	for _, comp := range cp.Components {
		switch comp.Layout {
		case "wysiwyg":
			if t := cleanText(comp.Body); t != "" {
				s.Blocks = append(s.Blocks, Block{Kind: comp.Layout, Title: cleanText(comp.Title), Text: t})
			}
		case "at_a_glance":
			b := Block{Kind: comp.Layout, Title: cleanText(comp.Title)}
			for _, it := range comp.Items {
				if t := cleanText(it.itemText()); t != "" {
					b.Items = append(b.Items, t)
				}
			}
			if len(b.Items) > 0 || b.Title != "" {
				s.Blocks = append(s.Blocks, b)
			}
		case "table":
			b := Block{Kind: comp.Layout, Title: cleanText(comp.Title)}
			for _, row := range comp.Table.rows() {
				b.Rows = append(b.Rows, row)
			}
			if len(b.Rows) > 0 {
				s.Blocks = append(s.Blocks, b)
			}
		default:
			if t := cleanText(comp.Body); t != "" {
				s.Blocks = append(s.Blocks, Block{Kind: comp.Layout, Text: t})
			}
		}
	}
	return s
}

// --- ACF wire shapes -------------------------------------------------------
// These mirror the acf payload as WordPress ACF serialises it. They are kept
// unexported: the package's public surface is Day/Week, not this.

type dayACF struct {
	Date             string          `json:"date"`
	Name             string          `json:"name"`
	Load             string          `json:"load"`
	Volume           string          `json:"volume"`
	Skill            string          `json:"skill"`
	Duration         string          `json:"duration"`
	ProgrammingTrack []string        `json:"programming_track"`
	Workouts         []workoutACF    `json:"workouts"`
	AboutTheWorkout  aboutACF        `json:"about_the_workout"`
	Scaling          scalingACF      `json:"scaling"`
	ClassPlan        []classPlanACF  `json:"class_plan"`
}

type workoutACF struct {
	Level       string `json:"level"`
	Description string `json:"description"`
	Score       string `json:"score"`
}

type aboutACF struct {
	CoachingGoals    []struct{ Goal string `json:"goal"` }        `json:"coaching_goals"`
	IntendedStimulus []struct{ Stimulus string `json:"stimulus"` } `json:"intended_stimulus"`
}

type scalingACF struct {
	Overview               string `json:"overview"`
	MovementScalingOptions []struct {
		Movement      string `json:"movement"`
		Substitutions string `json:"substitutions"`
	} `json:"movement_scaling_options"`
}

type classPlanACF struct {
	Title      string          `json:"title"`
	Duration   string          `json:"duration"`
	Components []componentACF  `json:"components"`
}

type componentACF struct {
	Layout string          `json:"acf_fc_layout"`
	Title  string          `json:"title"`
	Body   string          `json:"body"`
	Items  []atAGlanceItem `json:"items"`
	Table  tableACF        `json:"table"`
}

type atAGlanceItem struct {
	Item  string `json:"item"`
	Text  string `json:"text"`
	Label string `json:"label"`
}

func (i atAGlanceItem) itemText() string {
	for _, v := range []string{i.Item, i.Text, i.Label} {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// tableACF is ACF's table shape: a header row plus body rows, each cell an
// object with a "c" (content) field. It is decoded leniently because ACF has
// shipped more than one table serialisation over the years.
type tableACF struct {
	Header json.RawMessage `json:"header"`
	Body   json.RawMessage `json:"body"`
}

func (t tableACF) rows() [][]string {
	var out [][]string
	out = append(out, decodeTableRows(t.Header)...)
	out = append(out, decodeTableRows(t.Body)...)
	return out
}

func decodeTableRows(raw json.RawMessage) [][]string {
	if len(raw) == 0 {
		return nil
	}
	// Body is [][]cell, header is []cell. Try the 2-D shape first.
	var grid [][]tableCell
	if err := json.Unmarshal(raw, &grid); err == nil {
		var rows [][]string
		for _, r := range grid {
			rows = append(rows, cellRow(r))
		}
		return rows
	}
	var row []tableCell
	if err := json.Unmarshal(raw, &row); err == nil {
		return [][]string{cellRow(row)}
	}
	return nil
}

type tableCell struct {
	C string `json:"c"`
}

func cellRow(cells []tableCell) []string {
	out := make([]string, 0, len(cells))
	for _, c := range cells {
		out = append(out, cleanText(c.C))
	}
	return out
}

type weekACF struct {
	Date      string `json:"date"`
	Name      string `json:"name"`
	AtAGlance struct {
		Overview  string `json:"overview"`
		WeeklyTip string `json:"weekly_tip"`
	} `json:"at_a_glance"`
	MonthlyFocus struct {
		Subheading string `json:"subheading"`
		BodyText   string `json:"body_text"`
	} `json:"monthly_focus"`
	WorkYourWeakness []struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	} `json:"work_your_weakness"`
}

// --- text helpers ----------------------------------------------------------

var tagRe = regexp.MustCompile(`<[^>]+>`)
var wsRe = regexp.MustCompile(`[ \t\r\f\v]+`)
var blankLinesRe = regexp.MustCompile(`\n{3,}`)

// cleanText strips HTML tags, decodes entities, and collapses whitespace while
// keeping paragraph breaks. CAP's prose fields are WordPress HTML.
func cleanText(s string) string {
	if s == "" {
		return ""
	}
	// Turn block-level breaks into newlines before stripping tags.
	s = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(s, "\n")
	s = regexp.MustCompile(`(?i)</(p|div|li|tr)>`).ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, " ", " ") // non-breaking space
	// Collapse horizontal whitespace per line, then squeeze blank runs.
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimSpace(wsRe.ReplaceAllString(ln, " "))
	}
	s = strings.Join(lines, "\n")
	s = blankLinesRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// normaliseDate turns CAP's YYYYMMDD into YYYY-MM-DD. Anything else is returned
// unchanged.
func normaliseDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 8 && isAllDigits(s) {
		return s[0:4] + "-" + s[4:6] + "-" + s[6:8]
	}
	return s
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
