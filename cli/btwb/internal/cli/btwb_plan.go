// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Planning a workout is a multi-step HTML flow with a CSRF
// token, not an API call, so it does not go through the generated transport
// (which posts JSON). The shape of the flow is btwb's, not ours:
//
//	GET  /plan/track_events/workouts/new?d=<date>   -> CSRF token, plannable tracks
//	POST /plan/workouts/generated_workouts          -> btwb parses the text
//	POST /plan/workouts/<shape>                     -> the workout is created
//
// The middle step is the important one: btwb turns a plain description into a
// structured workout server-side. This CLI never composes that structure, it
// replays the form btwb produced and overrides only track, date and title.

package cli

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"btwb-pp-cli/internal/btwbhtml"
	"btwb-pp-cli/internal/config"
)

const (
	btwbBase        = "https://btwb.com"
	planNewPath     = "/plan/track_events/workouts/new"
	planGeneratePat = "/plan/workouts/generated_workouts"
)

var (
	// The page btwb redirects to once a workout has been created.
	createdEventRe = regexp.MustCompile(`/plan/track_events/workouts/(\d+)/edit`)
	// The frame btwb fills in asynchronously while it parses a description.
	pendingGenerationRe = regexp.MustCompile(`/plan/workouts/generated_workouts/(\d+)`)
)

func newWodPlanCmd(flags *rootFlags) *cobra.Command {
	var (
		date        string
		trackID     int
		track       string
		workout     string
		workoutFile string
		title       string
		assumeOK    bool
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Plan a workout onto a track",
		Long: `Plan a workout onto one of your btwb tracks.

You give the workout as plain text, the way a coach writes it on a whiteboard,
and btwb parses it into movements itself:

    3 rounds for time:
    Row, 500 m
    21 Box Jumps, 24/20 in
    12 Burpees

Which tracks you may plan into is decided by btwb, not by this CLI. Without gym
admin rights that is only your personal track; run 'btwb-pp-cli wod tracks' to
see the current list. A gym's subscription holder grants admin rights under
Admin Console -> Members -> Manage.

Nothing is written until you confirm. Use --dry-run to see exactly what would be
planned and stop there. --yes skips the prompt, so only pass it when you already
know what will be written.`,
		Example: `  btwb-pp-cli wod plan --date 2026-08-06 --workout "3 rounds for time:
  Row, 500 m
  12 Burpees" --dry-run

  btwb-pp-cli wod plan --date 2026-08-06 --track "Personal" --workout-file wod.txt --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if date == "" {
				date = time.Now().Format("2006-01-02")
			}
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return usageErr(fmt.Errorf("--date must be YYYY-MM-DD, got %q", date))
			}
			if workoutFile != "" {
				if workout != "" {
					return usageErr(fmt.Errorf("pass --workout or --workout-file, not both"))
				}
				data, rErr := readWorkoutFile(cmd, workoutFile)
				if rErr != nil {
					return rErr
				}
				workout = data
			}
			if strings.TrimSpace(workout) == "" {
				return usageErr(fmt.Errorf(
					"--workout is required: the workout text to plan (or --workout-file)"))
			}

			session, err := planSession(flags)
			if err != nil {
				return err
			}

			// Step 1: open the planning page for the CSRF token and the tracks
			// btwb is willing to accept.
			page, err := session.get(planNewPath + "?d=" + url.QueryEscape(date))
			if err != nil {
				return err
			}
			token, err := btwbhtml.CSRFToken(page)
			if err != nil {
				return fmt.Errorf("reading the planning page: %w", err)
			}
			tracks, err := btwbhtml.ParsePlannableTracks(page)
			if err != nil {
				return fmt.Errorf("reading the planning page: %w", err)
			}
			targetID, err := resolvePlanTrack(tracks, trackID, track)
			if err != nil {
				return err
			}

			// Step 2: hand btwb the text and let it build the workout.
			form, err := session.generateWorkout(token, workout, flags.timeout)
			if err != nil {
				return err
			}

			// btwb's own wizard sends these from the outer page; the form we
			// replayed came from the builder frame, which does not carry them.
			form.Set("track_event[track_id]", strconv.Itoa(targetID))
			form.Set("track_event[event_date]", date)
			form.Set("track_event[task_type]", "Workout")
			if title != "" {
				form.Set("track_event[title]", title)
			}

			plan := planPreview{
				Date:      date,
				TrackID:   targetID,
				TrackName: trackName(tracks, targetID),
				Title:     title,
				Action:    form.Action,
				Movements: form.Summary(),
			}

			if dryRun {
				plan.DryRun = true
				return emitPlan(cmd, flags, plan)
			}
			if !assumeOK {
				ok, cErr := confirmPlan(cmd, flags, plan)
				if cErr != nil {
					return cErr
				}
				if !ok {
					plan.Cancelled = true
					return emitPlan(cmd, flags, plan)
				}
			}

			// Step 3: create it.
			eventID, err := session.submitPlan(form)
			if err != nil {
				return err
			}
			plan.EventID = eventID
			plan.Planned = true
			return emitPlan(cmd, flags, plan)
		},
	}

	cmd.Flags().StringVar(&date, "date", "", "Date to plan onto, as YYYY-MM-DD (default: today)")
	cmd.Flags().IntVar(&trackID, "track-id", 0, "Track id to plan into")
	cmd.Flags().StringVar(&track, "track", "",
		"Track to plan into, matched by name, case-insensitive")
	cmd.Flags().StringVar(&workout, "workout", "", "The workout text btwb should parse")
	cmd.Flags().StringVar(&workoutFile, "workout-file", "",
		"Read the workout text from a file, or from stdin with -")
	cmd.Flags().StringVar(&title, "title", "", "Custom title (default: the one btwb derives)")
	cmd.Flags().BoolVar(&assumeOK, "yes", false, "Skip the confirmation prompt and write")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be planned, write nothing")
	return cmd
}

func newWodTracksCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "tracks",
		Short: "Tracks you are allowed to plan into",
		Long: `The tracks btwb will let you plan a workout into.

This is a shorter list than the tracks you can read: reading a gym's programming
comes with membership, writing to it requires admin rights on that gym.`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := planSession(flags)
			if err != nil {
				return err
			}
			page, err := session.get(planNewPath + "?d=" + time.Now().Format("2006-01-02"))
			if err != nil {
				return err
			}
			tracks, err := btwbhtml.ParsePlannableTracks(page)
			if err != nil {
				return fmt.Errorf("reading the planning page: %w", err)
			}
			if wantsJSON(cmd, flags) {
				return printWodJSON(cmd, flags, map[string]any{"tracks": tracks})
			}
			if len(tracks) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(),
					"no tracks available for planning (btwb offered none)")
				return nil
			}
			for _, t := range tracks {
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\n", t.ID, t.Name)
			}
			return nil
		},
	}
}

func newWodUnplanCmd(flags *rootFlags) *cobra.Command {
	var assumeOK bool
	cmd := &cobra.Command{
		Use:   "unplan <event-id>",
		Short: "Remove a planned workout",
		Long: `Remove a workout you planned, by the event id 'wod plan' reported.

This deletes the entry for everyone who follows the track. It is not offered to
agents over MCP; run it yourself.`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := strconv.Atoi(args[0])
			if err != nil {
				return usageErr(fmt.Errorf("event id must be a number, got %q", args[0]))
			}
			session, err := planSession(flags)
			if err != nil {
				return err
			}
			page, err := session.get(fmt.Sprintf("/plan/track_events/workouts/%d/edit", eventID))
			if err != nil {
				return err
			}
			token, err := btwbhtml.CSRFToken(page)
			if err != nil {
				return fmt.Errorf("reading the workout page: %w", err)
			}
			if !assumeOK && !flags.noInput {
				fmt.Fprintf(cmd.ErrOrStderr(), "Delete planned workout %d? [y/N] ", eventID)
				line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y") {
					fmt.Fprintln(cmd.OutOrStdout(), "cancelled")
					return nil
				}
			} else if !assumeOK {
				return usageErr(fmt.Errorf("--yes is required with --no-input"))
			}
			form := url.Values{"_method": {"delete"}, "authenticity_token": {token}}
			if _, err := session.post(fmt.Sprintf("/plan/track_events/%d", eventID),
				form.Encode()); err != nil {
				return err
			}
			if wantsJSON(cmd, flags) {
				return printWodJSON(cmd, flags,
					map[string]any{"event_id": eventID, "deleted": true})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted planned workout %d\n", eventID)
			return nil
		},
	}
	cmd.Flags().BoolVar(&assumeOK, "yes", false, "Skip the confirmation prompt")
	return cmd
}

// readWorkoutFile reads the workout text from a file, or from stdin for "-".
func readWorkoutFile(cmd *cobra.Command, path string) (string, error) {
	if path == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", usageErr(fmt.Errorf("reading the workout from stdin: %w", err))
		}
		return string(data), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", usageErr(fmt.Errorf("reading %s: %w", path, err))
	}
	return string(data), nil
}

// planPreview is what the user confirms and what the JSON output reports.
type planPreview struct {
	Date      string   `json:"date"`
	TrackID   int      `json:"track_id"`
	TrackName string   `json:"track_name,omitempty"`
	Title     string   `json:"title,omitempty"`
	Action    string   `json:"btwb_form_action"`
	Movements []string `json:"movements"`
	EventID   int      `json:"event_id,omitempty"`
	Planned   bool     `json:"planned"`
	DryRun    bool     `json:"dry_run,omitempty"`
	Cancelled bool     `json:"cancelled,omitempty"`
}

func emitPlan(cmd *cobra.Command, flags *rootFlags, plan planPreview) error {
	if wantsJSON(cmd, flags) {
		return printWodJSON(cmd, flags, plan)
	}
	w := cmd.OutOrStdout()
	switch {
	case plan.Cancelled:
		fmt.Fprintln(w, "cancelled, nothing was planned")
	case plan.DryRun:
		fmt.Fprintln(w, "dry run, nothing was planned:")
		printPlanBody(cmd, plan)
	default:
		fmt.Fprintf(w, "Planned workout %d on %s\n", plan.EventID, plan.Date)
		printPlanBody(cmd, plan)
	}
	return nil
}

func printPlanBody(cmd *cobra.Command, plan planPreview) {
	w := cmd.OutOrStdout()
	name := plan.TrackName
	if name == "" {
		name = fmt.Sprintf("track %d", plan.TrackID)
	}
	fmt.Fprintf(w, "  %s -> %s\n", plan.Date, name)
	if plan.Title != "" {
		fmt.Fprintf(w, "  title: %s\n", plan.Title)
	}
	for _, m := range plan.Movements {
		fmt.Fprintf(w, "    %s\n", m)
	}
}

func confirmPlan(cmd *cobra.Command, flags *rootFlags, plan planPreview) (bool, error) {
	if flags.noInput {
		return false, usageErr(fmt.Errorf(
			"refusing to write without confirmation: pass --yes, or --dry-run to preview"))
	}
	if !isTerminal(cmd.OutOrStdout()) && !flags.asJSON {
		// A piped invocation cannot answer a prompt.
		return false, usageErr(fmt.Errorf(
			"stdin is not interactive: pass --yes to write, or --dry-run to preview"))
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "About to plan:")
	printPlanBody(cmd, plan)
	fmt.Fprint(cmd.ErrOrStderr(), "Write this to btwb? [y/N] ")
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y"), nil
}

// resolvePlanTrack picks the track to write to, and explains the difference
// between "can read" and "can plan" when the requested one is not on offer.
func resolvePlanTrack(tracks []btwbhtml.Track, wantID int, wantName string) (int, error) {
	if len(tracks) == 0 {
		return 0, authErr(fmt.Errorf(
			"btwb offered no tracks to plan into; the session may have expired"))
	}
	if wantID != 0 {
		for _, t := range tracks {
			if t.ID == wantID {
				return wantID, nil
			}
		}
		return 0, authErr(fmt.Errorf(
			"track %d is not one you may plan into (%s); "+
				"planning into a gym track needs admin rights on that gym",
			wantID, describeTracks(tracks)))
	}
	if wantName != "" {
		needle := strings.ToLower(wantName)
		var hits []btwbhtml.Track
		for _, t := range tracks {
			if strings.Contains(strings.ToLower(t.Name), needle) {
				hits = append(hits, t)
			}
		}
		switch len(hits) {
		case 1:
			return hits[0].ID, nil
		case 0:
			return 0, usageErr(fmt.Errorf("no track matches %q (%s)",
				wantName, describeTracks(tracks)))
		default:
			return 0, usageErr(fmt.Errorf("%q matches more than one track (%s); use --track-id",
				wantName, describeTracks(hits)))
		}
	}
	if len(tracks) == 1 {
		return tracks[0].ID, nil
	}
	return 0, usageErr(fmt.Errorf("more than one track available, pass --track or --track-id (%s)",
		describeTracks(tracks)))
}

func describeTracks(tracks []btwbhtml.Track) string {
	parts := make([]string, 0, len(tracks))
	for _, t := range tracks {
		parts = append(parts, fmt.Sprintf("%d %s", t.ID, t.Name))
	}
	return "available: " + strings.Join(parts, "; ")
}

func trackName(tracks []btwbhtml.Track, id int) string {
	for _, t := range tracks {
		if t.ID == id {
			return t.Name
		}
	}
	return ""
}

// planHTTP is a small HTML-speaking client: the generated transport posts JSON
// and parses the response as data, neither of which fits this flow.
type planHTTP struct {
	client  *http.Client
	base    string
	session string
	timeout time.Duration
}

func planSession(flags *rootFlags) (*planHTTP, error) {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return nil, configErr(err)
	}
	value := cfg.SessionValue()
	if value == "" {
		return nil, authErr(fmt.Errorf("not signed in: run 'btwb-pp-cli auth login'"))
	}
	// Honour a configured base_url so this flow can be pointed at a replay of
	// captured pages; the generated transport already works that way.
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = btwbBase
	}
	return &planHTTP{
		client:  &http.Client{Timeout: flags.timeout},
		base:    base,
		session: value,
		timeout: flags.timeout,
	}, nil
}

// request performs one call and reports the final URL alongside the body: the
// transport follows redirects, and btwb puts the interesting fact (the id of
// what was just created) in the redirect target, not in the page it lands on.
func (p *planHTTP) request(method, path, body string) ([]byte, string, error) {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, p.base+path, reader)
	if err != nil {
		return nil, "", fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("Cookie", btwbSessionCookieName+"="+p.session)
	req.Header.Set("User-Agent", "btwb-pp-cli")
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", p.base)
		req.Header.Set("Referer", p.base+planNewPath)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, finalURL, fmt.Errorf("reading the response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, finalURL, apiErr(fmt.Errorf("btwb answered HTTP %d to %s %s",
			resp.StatusCode, method, path))
	}
	// An expired session is served the sign-in page with HTTP 200.
	if signinPageRe.Match(data) {
		return nil, finalURL, authErr(fmt.Errorf(
			"btwb redirected to the sign-in page: run 'btwb-pp-cli auth login'"))
	}
	return data, finalURL, nil
}

func (p *planHTTP) get(path string) ([]byte, error) {
	data, _, err := p.request(http.MethodGet, path, "")
	return data, err
}

func (p *planHTTP) post(path, body string) ([]byte, error) {
	data, _, err := p.request(http.MethodPost, path, body)
	return data, err
}

// generateWorkout hands the description to btwb and returns the form it built.
//
// btwb sometimes answers the POST with a frame it fills in a moment later
// rather than the finished form, so a pending response is polled until the form
// appears or the request timeout is spent.
func (p *planHTTP) generateWorkout(token, text string, timeout time.Duration) (*btwbhtml.PlanForm, error) {
	body := url.Values{
		"authenticity_token": {token},
		"planning_generated_workout[external_description]": {text},
	}.Encode()
	page, err := p.post(planGeneratePat, body)
	if err != nil {
		return nil, err
	}
	form, err := btwbhtml.ParsePlanForm(page)
	if err == nil {
		return form, nil
	}

	m := pendingGenerationRe.FindSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("btwb did not return a workout for that text: %w", err)
	}
	pendingPath := fmt.Sprintf("%s/%s", planGeneratePat, m[1])
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		polled, pErr := p.get(pendingPath)
		if pErr != nil {
			return nil, pErr
		}
		if form, pErr = btwbhtml.ParsePlanForm(polled); pErr == nil {
			return form, nil
		}
	}
	return nil, fmt.Errorf("btwb was still parsing the workout after %s", timeout)
}

// submitPlan posts the completed form and reports the event id btwb created.
//
// btwb answers with a redirect to /plan/track_events/workouts/<id>/edit, and
// the id appears ONLY in that URL: the edit page it lands on does not repeat
// it in a matchable form. (Verified live: the first release looked in the body
// and concluded the write had failed while btwb had in fact created the event.)
func (p *planHTTP) submitPlan(form *btwbhtml.PlanForm) (int, error) {
	page, finalURL, err := p.request(http.MethodPost, form.Action, form.Encode())
	if err != nil {
		return 0, err
	}
	if m := createdEventRe.FindStringSubmatch(finalURL); m != nil {
		id, _ := strconv.Atoi(m[1])
		return id, nil
	}
	if m := createdEventRe.FindSubmatch(page); m != nil {
		id, _ := strconv.Atoi(string(m[1]))
		return id, nil
	}
	// btwb accepted the post but did not show the event; report that honestly
	// rather than claiming an id we did not see. The write may still have
	// happened, so tell the caller where to look.
	return 0, fmt.Errorf(
		"btwb accepted the workout but did not report its id; "+
			"check the calendar for %s before retrying, the write may have succeeded",
		form.Get("track_event[event_date]"))
}
