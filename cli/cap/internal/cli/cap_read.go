// Copyright 2026 isolovyev. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. The generated `subscriptions` command is a faithful mirror of
// the content endpoint: it wants a raw URN and returns the raw envelope. These
// commands are the useful shape on top of it - they build the URN from a date,
// parse the envelope into a Day/Week, and (for `compare`) reduce each day to the
// load/volume/skill vectors plus movement-pattern tags a coach reorders around.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cap-pp-cli/internal/capjson"
)

// The content URN is uniform across everything CAP publishes:
//
//	content_api:///programming/<track>/daily-class-plan/<id>
//
// For the affiliate track the id is a date; for the skill and hero tracks it is
// a sequence position ("day-1"), because those are programmes you start when
// you like rather than on a calendar.
const (
	dayURNTemplate  = "content_api:///programming/%s/daily-class-plan/%s"
	weekURNTemplate = "content_api:///programming/%s/weekly-overview/%s"
	contentPath     = "/subscriptions/v1/content"

	defaultTrack = "affiliate"
)

// knownTracks are the tracks seen published. The list is for help text and
// suggestions only - any track slug is passed through, so a new one works
// without a release.
var knownTracks = []struct{ Slug, Desc string }{
	{"affiliate", "the daily affiliate programming (dates)"},
	{"murph", "10-day Murph preparation (day-N)"},
	{"chad", "10-day Chad preparation (day-N)"},
	{"pull-up", "pull-up skill progression (day-N)"},
	{"chest-to-bar-pull-up", "chest-to-bar skill progression (day-N)"},
	{"bar-muscle-up", "bar muscle-up skill progression (day-N)"},
	{"ring-muscle-up", "ring muscle-up skill progression (day-N)"},
	{"toes-to-bar", "toes-to-bar skill progression (day-N)"},
	{"handstand-push-up", "handstand push-up skill progression (day-N)"},
	{"double-under", "double-under skill progression (day-N)"},
}

var isoDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
var compactDateRe = regexp.MustCompile(`^\d{8}$`)
var dayNRe = regexp.MustCompile(`^(?:day-)?(\d{1,3})$`)

func newCapCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cap",
		Short: "Read CrossFit Affiliate Programming (CAP)",
		Long: `Read the CrossFit Affiliate Programming daily class plans and weekly overviews.

Dates are given as YYYY-MM-DD or YYYYMMDD. 'cap day' parses one day's workout,
its intended stimulus and the load/volume/skill vectors; 'cap week' reads the
weekly overview; 'cap warmup' pulls the class plan's warm-up blocks; and
'cap compare' scores how much two days overlap - by CAP's own vectors and by the
movement patterns they train - which is the raw material for resequencing a week
so athletes who train on fixed days do not hit the same pattern twice running.`,
	}
	cmd.AddCommand(newCapDayCmd(flags))
	cmd.AddCommand(newCapWeekCmd(flags))
	cmd.AddCommand(newCapWarmupCmd(flags))
	cmd.AddCommand(newCapCompareCmd(flags))
	cmd.AddCommand(newCapTracksCmd(flags))
	cmd.AddCommand(newCapMovementCmd(flags))
	cmd.AddCommand(newCapMovementsCmd(flags))
	cmd.AddCommand(newCapBenchmarksCmd(flags))
	cmd.AddCommand(newCapResourcesCmd(flags))
	return cmd
}

func newCapTracksCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "tracks",
		Short: "The programming tracks that can be read",
		Long: `The tracks 'cap day --track' can read.

The affiliate track is the daily gym programming, addressed by date. The others
are fixed-length programmes - hero workout preparation and skill progressions -
addressed by day number.

Any track slug is accepted, so a newly published one works without waiting for
this list to be updated.`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if wantsJSON(cmd, flags) {
				out := make([]map[string]string, 0, len(knownTracks))
				for _, t := range knownTracks {
					out = append(out, map[string]string{"slug": t.Slug, "description": t.Desc})
				}
				return printCapJSON(cmd, flags, map[string]any{"tracks": out})
			}
			for _, t := range knownTracks {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-24s %s\n", t.Slug, t.Desc)
			}
			return nil
		},
	}
}

// resolveDate accepts YYYY-MM-DD or YYYYMMDD and returns the compact YYYYMMDD
// form the URN needs plus the ISO form for display. An empty date means today.
func resolveDate(in string) (compact, iso string, err error) {
	in = strings.TrimSpace(in)
	if in == "" {
		now := time.Now()
		return now.Format("20060102"), now.Format("2006-01-02"), nil
	}
	switch {
	case isoDateRe.MatchString(in):
		t, e := time.Parse("2006-01-02", in)
		if e != nil {
			return "", "", fmt.Errorf("invalid date %q", in)
		}
		return t.Format("20060102"), in, nil
	case compactDateRe.MatchString(in):
		t, e := time.Parse("20060102", in)
		if e != nil {
			return "", "", fmt.Errorf("invalid date %q", in)
		}
		return in, t.Format("2006-01-02"), nil
	default:
		return "", "", fmt.Errorf("date must be YYYY-MM-DD or YYYYMMDD, got %q", in)
	}
}

// resolveSelector turns a track plus a user-supplied position into the URN id.
// The affiliate track is addressed by date; every other track is a sequence,
// so "3", "day-3" and an empty value (meaning day 1) all resolve to "day-3"/1.
func resolveSelector(track, in string) (id, display string, err error) {
	if track == defaultTrack {
		return resolveDate(in)
	}
	in = strings.TrimSpace(in)
	if in == "" {
		return "day-1", "day 1", nil
	}
	if m := dayNRe.FindStringSubmatch(in); m != nil {
		return "day-" + m[1], "day " + m[1], nil
	}
	return "", "", fmt.Errorf(
		"track %q is a sequence, not a calendar: pass a day number like 3 or day-3, got %q",
		track, in)
}

// fetchContent gets and unwraps a content document for a URN.
//
// A date with no programming comes back two different ways depending on how far
// off it is: an empty tiles list (handled by the parser) or a bare HTTP 404.
// Both mean the same thing to a coach, so the 404 is turned into the same
// not-found answer instead of a raw HTTP error body.
func fetchContent(flags *rootFlags, urn, iso string) ([]byte, error) {
	c, err := flags.newClient()
	if err != nil {
		return nil, err
	}
	raw, err := c.Get(contentPath, map[string]string{"urn": urn})
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, notFoundErr(fmt.Errorf("no CAP programming published for %s", iso))
		}
		return nil, classifyAPIError(err, flags)
	}
	return raw, nil
}

// mapContentErr maps the "no tile" sentinel to exit code 3.
func mapContentErr(err error, iso string) error {
	if errors.Is(err, capjson.ErrNoContent) {
		return notFoundErr(fmt.Errorf("no CAP programming published for %s", iso))
	}
	return err
}

func newCapDayCmd(flags *rootFlags) *cobra.Command {
	var full bool
	var track string
	cmd := &cobra.Command{
		Use:   "day [date|day-N]",
		Short: "One day's class plan",
		Long: `Read one CAP daily class plan.

By default it prints the workout, its intended stimulus, the load/volume/skill
vectors and the movement patterns the day trains. Add --full to include the
whole timed class plan, scaling and coaching goals.

--track selects which programme to read. The affiliate track is addressed by
date; the skill and hero tracks (murph, chad, pull-up, ring-muscle-up and the
rest) are sequences, addressed by day number. Run 'cap-pp-cli cap tracks' for
the list.`,
		Example: `  cap-pp-cli cap day 2026-08-10
  cap-pp-cli cap day 20260810 --full --agent
  cap-pp-cli cap day 3 --track murph`,
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			sel := ""
			if len(args) == 1 {
				sel = args[0]
			}
			id, display, err := resolveSelector(track, sel)
			if err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				return nil
			}
			raw, err := fetchContent(flags, fmt.Sprintf(dayURNTemplate, track, id), display)
			if err != nil {
				return err
			}
			day, err := capjson.ParseDay(raw)
			if err != nil {
				return mapContentErr(err, display)
			}
			if wantsJSON(cmd, flags) {
				out := dayOutput(day, full)
				return printCapJSON(cmd, flags, out)
			}
			printDayText(cmd, day, full)
			return nil
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Include the class plan, scaling and coaching goals")
	cmd.Flags().StringVar(&track, "track", defaultTrack,
		"Programme to read: affiliate (dates), or a skill/hero track (day numbers)")
	return cmd
}

func newCapWeekCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "week [date]",
		Short:       "The weekly overview",
		Long:        "Read the CAP weekly overview for the week containing a date.",
		Example:     "  cap-pp-cli week 2026-08-10 --agent",
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			date := ""
			if len(args) == 1 {
				date = args[0]
			}
			compact, iso, err := resolveDate(date)
			if err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				return nil
			}
			raw, err := fetchContent(flags, fmt.Sprintf(weekURNTemplate, defaultTrack, compact), iso)
			if err != nil {
				return err
			}
			week, err := capjson.ParseWeek(raw)
			if err != nil {
				return mapContentErr(err, iso)
			}
			if wantsJSON(cmd, flags) {
				return printCapJSON(cmd, flags, week)
			}
			printWeekText(cmd, week)
			return nil
		},
	}
	return cmd
}

func newCapWarmupCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "warmup [date]",
		Short: "The warm-up blocks of a day's class plan",
		Long: `Pull the warm-up sections of a day's class plan.

CAP splits the lesson into timed sections; this returns the general and specific
warm-up blocks with their content, ready to adapt into your own warm-up.`,
		Example:     "  cap-pp-cli warmup 2026-08-10",
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			date := ""
			if len(args) == 1 {
				date = args[0]
			}
			compact, iso, err := resolveDate(date)
			if err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				return nil
			}
			raw, err := fetchContent(flags, fmt.Sprintf(dayURNTemplate, defaultTrack, compact), iso)
			if err != nil {
				return err
			}
			day, err := capjson.ParseDay(raw)
			if err != nil {
				return mapContentErr(err, iso)
			}
			warmups := warmupSections(day)
			if wantsJSON(cmd, flags) {
				return printCapJSON(cmd, flags, map[string]any{
					"date": day.Date, "name": day.Name, "warmup": warmups,
				})
			}
			printWarmupText(cmd, day, warmups)
			return nil
		},
	}
	return cmd
}

func newCapCompareCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compare <date> <date>",
		Short: "Score how much two days overlap",
		Long: `Compare two CAP days by training demand.

It reports each day's load/volume/skill vectors and movement patterns, then a
collision score: how much placing these two on consecutive training days would
double up on the same patterns and demand. A high score means "do not run these
back to back"; a low score means they complement each other. This is the signal
for resequencing a week around athletes who train on fixed days.`,
		Example:     "  cap-pp-cli compare 2026-08-10 2026-08-16 --agent",
		Args:        cobra.ExactArgs(2),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadProfile(flags, args[0])
			if err != nil {
				return err
			}
			b, err := loadProfile(flags, args[1])
			if err != nil {
				return err
			}
			overlap := a.CollisionScore(b)
			result := map[string]any{
				"a": a, "b": b, "overlap": overlap,
			}
			if wantsJSON(cmd, flags) {
				return printCapJSON(cmd, flags, result)
			}
			printCompareText(cmd, a, b, overlap)
			return nil
		},
	}
	return cmd
}

func loadProfile(flags *rootFlags, date string) (capjson.Profile, error) {
	compact, iso, err := resolveDate(date)
	if err != nil {
		return capjson.Profile{}, usageErr(err)
	}
	raw, err := fetchContent(flags, fmt.Sprintf(dayURNTemplate, defaultTrack, compact), iso)
	if err != nil {
		return capjson.Profile{}, err
	}
	day, err := capjson.ParseDay(raw)
	if err != nil {
		return capjson.Profile{}, mapContentErr(err, iso)
	}
	return day.Profile(), nil
}

// --- output shaping --------------------------------------------------------

func dayOutput(day *capjson.Day, full bool) any {
	base := map[string]any{
		"date":      day.Date,
		"name":      day.Name,
		"title":     day.Title,
		"tracks":    day.Tracks,
		"load":      day.Load,
		"volume":    day.Volume,
		"skill":     day.Skill,
		"workouts":  day.Workouts,
		"stimulus":  day.Stimulus,
		"profile":   day.Profile(),
	}
	if full {
		base["coaching_goals"] = day.CoachingGoals
		base["class_plan"] = day.ClassPlan
		base["scaling"] = day.Scaling
		base["duration"] = day.Duration
	}
	return base
}

func warmupSections(day *capjson.Day) []capjson.Section {
	var out []capjson.Section
	for _, s := range day.ClassPlan {
		if strings.Contains(strings.ToLower(s.Title), "warm_up") ||
			strings.Contains(strings.ToLower(s.Title), "warm-up") ||
			strings.Contains(strings.ToLower(s.Title), "warmup") {
			out = append(out, s)
		}
	}
	return out
}

// wantsJSON mirrors the generated commands: explicit --json, or a piped stdout
// with no other format asked for.
func wantsJSON(cmd *cobra.Command, flags *rootFlags) bool {
	if flags.asJSON {
		return true
	}
	return !isTerminal(cmd.OutOrStdout()) && !flags.csv && !flags.quiet && !flags.plain
}

// printCapJSON wraps output in the same {meta, results} envelope the generated
// commands use, so this CLI never has two response shapes.
func printCapJSON(cmd *cobra.Command, flags *rootFlags, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding response: %w", err)
	}
	if flags.selectFields != "" {
		data = filterFields(data, flags.selectFields)
	} else if flags.compact {
		data = compactFields(data)
	}
	wrapped, err := wrapWithProvenance(data, DataProvenance{Source: "live"})
	if err != nil {
		return err
	}
	return printOutput(cmd.OutOrStdout(), wrapped, true)
}

// --- human text ------------------------------------------------------------

func printDayText(cmd *cobra.Command, day *capjson.Day, full bool) {
	w := cmd.OutOrStdout()
	heading := day.Name
	if day.Title != "" && day.Title != day.Name {
		heading = day.Title
	}
	fmt.Fprintf(w, "%s  %s\n", day.Date, heading)
	fmt.Fprintf(w, "  load %d / volume %d / skill %d\n", day.Load, day.Volume, day.Skill)
	prof := day.Profile()
	fmt.Fprintf(w, "  patterns: %s\n", joinPatterns(prof.Patterns))
	if len(prof.Unknown) > 0 {
		fmt.Fprintf(w, "  (unclassified: %s)\n", strings.Join(prof.Unknown, ", "))
	}
	for _, wo := range day.Workouts {
		// Sequence tracks publish a single unlabelled prescription; only the
		// affiliate track splits it into rx/intermediate/scaled levels.
		if wo.Level != "" {
			fmt.Fprintf(w, "\n  [%s]\n", wo.Level)
		} else {
			fmt.Fprintln(w)
		}
		for _, line := range strings.Split(wo.Description, "\n") {
			fmt.Fprintf(w, "    %s\n", line)
		}
	}
	if len(day.Stimulus) > 0 {
		fmt.Fprintln(w, "\n  intended stimulus:")
		for _, s := range day.Stimulus {
			fmt.Fprintf(w, "    - %s\n", s)
		}
	}
	if full {
		if day.Scaling.Overview != "" {
			fmt.Fprintf(w, "\n  scaling: %s\n", day.Scaling.Overview)
		}
		if len(day.ClassPlan) > 0 {
			fmt.Fprintln(w, "\n  class plan:")
			for _, s := range day.ClassPlan {
				fmt.Fprintf(w, "    %2d min  %s\n", s.Minutes, s.Title)
			}
		}
	}
}

func printWeekText(cmd *cobra.Command, wk *capjson.Week) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s  %s\n", wk.Date, wk.Name)
	if wk.Overview != "" {
		fmt.Fprintf(w, "\n%s\n", wk.Overview)
	}
	if wk.WeeklyTip != "" {
		fmt.Fprintf(w, "\nweekly tip: %s\n", wk.WeeklyTip)
	}
	if wk.MonthlyFocus != "" {
		fmt.Fprintf(w, "\nmonthly focus: %s\n", wk.MonthlyFocus)
	}
	for _, ww := range wk.Weaknesses {
		fmt.Fprintf(w, "\n%s\n  %s\n", ww.Title, ww.Body)
	}
}

func printWarmupText(cmd *cobra.Command, day *capjson.Day, warmups []capjson.Section) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s  %s - warm-up\n", day.Date, day.Name)
	if len(warmups) == 0 {
		fmt.Fprintln(w, "  no warm-up blocks in this class plan")
		return
	}
	for _, s := range warmups {
		fmt.Fprintf(w, "\n  %s (%d min)\n", s.Title, s.Minutes)
		for _, b := range s.Blocks {
			if b.Title != "" {
				fmt.Fprintf(w, "    %s\n", b.Title)
			}
			if b.Text != "" {
				for _, line := range strings.Split(b.Text, "\n") {
					fmt.Fprintf(w, "      %s\n", line)
				}
			}
			for _, it := range b.Items {
				fmt.Fprintf(w, "      - %s\n", it)
			}
			for _, row := range b.Rows {
				fmt.Fprintf(w, "      %s\n", strings.Join(row, " | "))
			}
		}
	}
}

func printCompareText(cmd *cobra.Command, a, b capjson.Profile, o capjson.Overlap) {
	w := cmd.OutOrStdout()
	line := func(p capjson.Profile) {
		fmt.Fprintf(w, "  %s %-24s L%d V%d S%d  %s\n",
			p.Date, truncateName(p.Name, 24), p.Load, p.Volume, p.Skill, joinPatterns(p.Patterns))
	}
	line(a)
	line(b)
	fmt.Fprintf(w, "\n  collision score: %d\n", o.Score)
	if len(o.SharedPatterns) > 0 {
		fmt.Fprintf(w, "  shared patterns: %s\n", joinPatterns(o.SharedPatterns))
	} else {
		fmt.Fprintln(w, "  shared patterns: none")
	}
	fmt.Fprintf(w, "  vector distance: %d (higher = more different demand)\n", o.VectorDistance)
	fmt.Fprintf(w, "\n  %s\n", collisionVerdict(o))
}

func collisionVerdict(o capjson.Overlap) string {
	switch {
	case o.Score >= 8:
		return "strong overlap - avoid on consecutive training days"
	case o.Score >= 4:
		return "some overlap - acceptable but not ideal back to back"
	default:
		return "complementary - good to run on consecutive days"
	}
}

func joinPatterns(ps []capjson.Pattern) string {
	if len(ps) == 0 {
		return "(none)"
	}
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = string(p)
	}
	return strings.Join(parts, ", ")
}

func truncateName(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
