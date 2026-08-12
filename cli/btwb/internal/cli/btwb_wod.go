// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. The generated `members whiteboard get-day` command is a
// faithful mirror of the endpoint: it wants a member id and a raw `d` param and
// returns entries whose track is only a number. These commands are the useful
// shape on top of it - they resolve the member from the stored session, name
// the tracks, filter to the track you care about, and can inline the full
// workout text in one call.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"btwb-pp-cli/internal/btwbhtml"
	"btwb-pp-cli/internal/client"
	"btwb-pp-cli/internal/config"
)

// wodDay is a whiteboard day with the workout text optionally inlined.
type wodDay struct {
	Date     string    `json:"date"`
	MemberID int       `json:"member_id"`
	Workouts []wodItem `json:"workouts"`
}

// wodItem is a whiteboard entry. Detail is present when --details was asked for
// and the entry is a planned workout.
type wodItem struct {
	btwbhtml.WodEntry
	Detail *btwbhtml.Wod `json:"detail,omitempty"`
}

func newWodCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wod",
		Short: "Read the workouts programmed on your whiteboard",
		Long: `Read the workouts on your btwb whiteboard.

These commands resolve your member id from the session stored by
'btwb-pp-cli auth login', so you only pass the date and, if you want, the track.

Add --details to inline each workout's full text as written by the coach; that
costs one extra request per workout, so combine it with --track to keep the
result small.`,
	}
	cmd.AddCommand(newWodTodayCmd(flags))
	cmd.AddCommand(newWodDayCmd(flags))
	cmd.AddCommand(newWodWeekCmd(flags))
	cmd.AddCommand(newWodEventCmd(flags))
	cmd.AddCommand(newWodPlanCmd(flags))
	cmd.AddCommand(newWodTracksCmd(flags))
	cmd.AddCommand(newWodUnplanCmd(flags))
	return cmd
}

// wodSelector carries the flags shared by the day-shaped commands.
type wodSelector struct {
	track       string
	trackID     int
	memberID    int
	details     bool
	plannedOnly bool
}

func (s *wodSelector) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&s.track, "track", "",
		"Only entries whose track name contains this text, case-insensitive")
	cmd.Flags().IntVar(&s.trackID, "track-id", 0, "Only entries on this track id")
	cmd.Flags().IntVar(&s.memberID, "member-id", 0,
		"Whiteboard to read (default: the signed-in member)")
	cmd.Flags().BoolVar(&s.details, "details", false,
		"Fetch and inline the full workout text for each planned workout")
	cmd.Flags().BoolVar(&s.plannedOnly, "planned-only", false,
		"Drop your own logged results, keeping only what the coach programmed")
}

func newWodTodayCmd(flags *rootFlags) *cobra.Command {
	sel := &wodSelector{}
	cmd := &cobra.Command{
		Use:         "today",
		Short:       "Today's workouts",
		Example:     "  btwb-pp-cli wod today --track \"Natrium WODs\" --details --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWodDay(cmd, flags, sel, time.Now().Format("2006-01-02"))
		},
	}
	sel.bind(cmd)
	return cmd
}

func newWodDayCmd(flags *rootFlags) *cobra.Command {
	sel := &wodSelector{}
	var date string
	cmd := &cobra.Command{
		Use:         "day",
		Short:       "Workouts on one date",
		Example:     "  btwb-pp-cli wod day --date 2026-07-30 --details --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if date == "" {
				return usageErr(fmt.Errorf("--date is required, as YYYY-MM-DD"))
			}
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return usageErr(fmt.Errorf("--date must be YYYY-MM-DD, got %q", date))
			}
			return runWodDay(cmd, flags, sel, date)
		},
	}
	cmd.Flags().StringVar(&date, "date", "", "Date to read, as YYYY-MM-DD")
	sel.bind(cmd)
	return cmd
}

func newWodWeekCmd(flags *rootFlags) *cobra.Command {
	sel := &wodSelector{}
	var date string
	cmd := &cobra.Command{
		Use:   "week",
		Short: "Workouts for every day btwb returns around a date",
		Long: `Workouts for the fortnight btwb renders around a date.

btwb's whiteboard renders two weeks at a time, so this returns every day in that
window rather than exactly seven days. Each day has the same shape as 'wod day'.`,
		Example:     "  btwb-pp-cli wod week --date 2026-07-30 --track CAP --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if date == "" {
				date = time.Now().Format("2006-01-02")
			}
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return usageErr(fmt.Errorf("--date must be YYYY-MM-DD, got %q", date))
			}
			c, memberID, names, err := wodContext(flags, sel)
			if err != nil {
				return err
			}
			// Only the bare /whiteboard route honours ?d=; the member-scoped
			// day and month routes silently answer with the current fortnight
			// whatever date is asked for, which made every historical and
			// future date read as "not present in page".
			raw, err := c.Get(whiteboardPath, map[string]string{"d": date})
			if err != nil {
				return classifyBtwbError(err, flags)
			}
			var parsed struct {
				Days []btwbhtml.WhiteboardDay `json:"days"`
			}
			if err := json.Unmarshal(raw, &parsed); err != nil {
				return fmt.Errorf("reading whiteboard response: %w", err)
			}
			if len(parsed.Days) == 0 {
				return fmt.Errorf("no days returned for %s", date)
			}
			out := make([]wodDay, 0, len(parsed.Days))
			for _, d := range parsed.Days {
				day, bErr := buildWodDay(c, sel, names, d)
				if bErr != nil {
					return bErr
				}
				out = append(out, *day)
			}
			if wantsJSON(cmd, flags) {
				return printWodJSON(cmd, flags, map[string]any{
					"member_id": memberID, "days": out,
				})
			}
			for _, d := range out {
				printWodDayText(cmd, &d)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&date, "date", "", "Any date in the window (default: today)")
	sel.bind(cmd)
	return cmd
}

func newWodEventCmd(flags *rootFlags) *cobra.Command {
	var memberID int
	cmd := &cobra.Command{
		Use:   "event <event-id>",
		Short: "One workout in full, by its event id",
		Long: `One workout in full.

Event ids come from the entries returned by 'wod today' and 'wod day'.`,
		Args:        cobra.ExactArgs(1),
		Example:     "  btwb-pp-cli wod event 323452610 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := strconv.Atoi(args[0])
			if err != nil {
				return usageErr(fmt.Errorf("event id must be a number, got %q", args[0]))
			}
			c, resolved, _, err := wodContext(flags, &wodSelector{memberID: memberID})
			if err != nil {
				return err
			}
			wod, err := fetchWodDetail(c, resolved, eventID)
			if err != nil {
				return classifyBtwbError(err, flags)
			}
			if wantsJSON(cmd, flags) {
				return printWodJSON(cmd, flags, wod)
			}
			printWodText(cmd, wod, "")
			return nil
		},
	}
	cmd.Flags().IntVar(&memberID, "member-id", 0,
		"Whiteboard to read (default: the signed-in member)")
	return cmd
}

// whiteboardPath is the only route that honours the ?d= parameter. The
// member-scoped /members/<id>/whiteboard/day and .../month routes ignore it and
// always render the current fortnight, so a request for any other date came
// back as "date not present in page".
const whiteboardPath = "/whiteboard"

// dayFromWindow picks one date out of the fortnight btwb renders, and says
// plainly what the page did cover when the date is not in it.
func dayFromWindow(raw []byte, memberID int, date string) (*btwbhtml.WhiteboardDay, error) {
	var window struct {
		Days []btwbhtml.WhiteboardDay `json:"days"`
	}
	if err := json.Unmarshal(raw, &window); err != nil {
		return nil, fmt.Errorf("reading whiteboard response: %w", err)
	}
	for i := range window.Days {
		if window.Days[i].Date == date {
			d := window.Days[i]
			if d.MemberID == 0 {
				d.MemberID = memberID
			}
			return &d, nil
		}
	}
	if len(window.Days) == 0 {
		return nil, notFoundErr(fmt.Errorf("btwb returned no days for %s", date))
	}
	return nil, notFoundErr(fmt.Errorf(
		"no whiteboard for %s; btwb returned %s..%s",
		date, window.Days[0].Date, window.Days[len(window.Days)-1].Date))
}

// wodContext resolves the client, whose whiteboard to read, and the track names.
func wodContext(flags *rootFlags, sel *wodSelector) (*client.Client, int, map[int]string, error) {
	c, err := flags.newClient()
	if err != nil {
		return nil, 0, nil, err
	}
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return nil, 0, nil, configErr(err)
	}
	memberID := sel.memberID
	if memberID == 0 {
		memberID = cfg.MemberIDValue()
	}
	if memberID == 0 {
		return nil, 0, nil, authErr(fmt.Errorf(
			"no member id stored: run 'btwb-pp-cli auth login', or pass --member-id"))
	}
	// Track names are a nicety; a failure here must not fail the read.
	names := map[int]string{}
	if raw, tErr := c.Get(fmt.Sprintf("/members/%d/tracks", memberID), nil); tErr == nil {
		var wrapper struct {
			Tracks []btwbhtml.Track `json:"tracks"`
		}
		if json.Unmarshal(raw, &wrapper) == nil {
			for _, t := range wrapper.Tracks {
				names[t.ID] = t.Name
			}
		}
	}
	return c, memberID, names, nil
}

func runWodDay(cmd *cobra.Command, flags *rootFlags, sel *wodSelector, date string) error {
	c, memberID, names, err := wodContext(flags, sel)
	if err != nil {
		return err
	}
	// btwb answers with a fortnight around the date, and only on the bare
	// /whiteboard route does the date have any effect; the requested day is
	// then picked out of that window.
	raw, err := c.Get(whiteboardPath, map[string]string{"d": date})
	if err != nil {
		return classifyBtwbError(err, flags)
	}
	day, err := dayFromWindow(raw, memberID, date)
	if err != nil {
		return err
	}
	built, err := buildWodDay(c, sel, names, *day)
	if err != nil {
		return err
	}
	if wantsJSON(cmd, flags) {
		return printWodJSON(cmd, flags, built)
	}
	printWodDayText(cmd, built)
	return nil
}

// buildWodDay names tracks, applies the filters, and optionally inlines detail.
func buildWodDay(c *client.Client, sel *wodSelector, names map[int]string,
	day btwbhtml.WhiteboardDay) (*wodDay, error) {

	out := &wodDay{Date: day.Date, MemberID: day.MemberID}
	wantTrack := strings.ToLower(strings.TrimSpace(sel.track))

	for _, e := range day.Workouts {
		if e.TrackName == "" {
			if n, ok := names[e.TrackID]; ok {
				e.TrackName = n
			}
		}
		if sel.trackID != 0 && e.TrackID != sel.trackID {
			continue
		}
		if wantTrack != "" && !strings.Contains(strings.ToLower(e.TrackName), wantTrack) {
			continue
		}
		if sel.plannedOnly && e.Kind != "planned" {
			continue
		}
		item := wodItem{WodEntry: e}
		if sel.details && e.Kind == "planned" && e.EventID != 0 {
			detail, err := fetchWodDetail(c, day.MemberID, e.EventID)
			if err != nil {
				return nil, fmt.Errorf("reading workout %d: %w", e.EventID, err)
			}
			item.Detail = detail
		}
		out.Workouts = append(out.Workouts, item)
	}
	return out, nil
}

func fetchWodDetail(c *client.Client, memberID, eventID int) (*btwbhtml.Wod, error) {
	raw, err := c.Get(fmt.Sprintf("/tasks/members/%d/track_events/%d", memberID, eventID), nil)
	if err != nil {
		return nil, err
	}
	var wod btwbhtml.Wod
	if err := json.Unmarshal(raw, &wod); err != nil {
		return nil, fmt.Errorf("reading workout response: %w", err)
	}
	return &wod, nil
}

// wantsJSON mirrors the generated commands: explicit --json, or a piped stdout
// with no other format asked for.
func wantsJSON(cmd *cobra.Command, flags *rootFlags) bool {
	if flags.asJSON {
		return true
	}
	return !isTerminal(cmd.OutOrStdout()) && !flags.csv && !flags.quiet && !flags.plain
}

// printWodJSON emits the same {meta, results} envelope the generated commands
// use. Two response shapes in one CLI is a trap for whoever parses it.
func printWodJSON(cmd *cobra.Command, flags *rootFlags, v any) error {
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

func printWodDayText(cmd *cobra.Command, day *wodDay) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s\n", day.Date)
	if len(day.Workouts) == 0 {
		fmt.Fprintln(w, "  nothing programmed")
		return
	}
	for _, item := range day.Workouts {
		marker := "*"
		if item.Kind == "logged" {
			marker = "v"
		}
		track := item.TrackName
		if track == "" {
			track = fmt.Sprintf("track %d", item.TrackID)
		}
		fmt.Fprintf(w, "  %s [%s] %s\n", marker, track, item.Title)
		if item.Detail != nil {
			printWodText(cmd, item.Detail, "      ")
		}
	}
}

func printWodText(cmd *cobra.Command, wod *btwbhtml.Wod, indent string) {
	w := cmd.OutOrStdout()
	if wod.Name != "" {
		fmt.Fprintf(w, "%s%s\n", indent, wod.Name)
	}
	for _, line := range strings.Split(wod.Description, "\n") {
		fmt.Fprintf(w, "%s%s\n", indent, line)
	}
	if wod.ResultsCount > 0 {
		fmt.Fprintf(w, "%s(%d results logged)\n", indent, wod.ResultsCount)
	}
}

// classifyBtwbError maps a missing or expired session to exit code 4 before
// falling back to the generated HTTP error classification.
func classifyBtwbError(err error, flags *rootFlags) error {
	var needsLogin *client.ErrNeedsLogin
	if errors.As(err, &needsLogin) {
		return authErr(needsLogin)
	}
	return classifyAPIError(err, flags)
}
