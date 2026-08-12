// Copyright 2026 isolovyev. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. The coaching library (movements, benchmarks, programming
// resources) lives on a different host from the programming: the public CMS at
// cms-api.crossfit.com, which needs no token and no subscription. That is why
// these commands do not go through the generated client at all - it is wired
// for the authenticated c3po API - and why they keep working when the
// programming token has expired.
//
// A note for whoever debugs a "wrong movement came back" report: every response
// names the movement it carries, and fetchMovement rejects a mismatch. The
// likelier cause is on the caller's side - several lookups backgrounded in one
// shell finish out of order, so their output interleaves and answers line up
// against the wrong questions. Reproduced deliberately: two parallel calls
// printed in a different order on consecutive runs. SKILL.md tells agents to
// run these one at a time and to key results by .results.slug rather than by
// output order.

package cli

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"cap-pp-cli/internal/capjson"
)

// cmsBase is a var, not a const, so tests can point the library commands at a
// replay server; nothing else reassigns it.
var cmsBase = "https://cms-api.crossfit.com/tiles?full_path="

// CMS listing pages worth having as named commands.
const (
	pathMovements = "/atk-page/movements"
	pathBenchmark = "/atk-page/programming-resources/benchmarks"
	pathResources = "/atk-page/programming-resources"
	pathKidsTeens = "/atk-page/kids-teens-training-plans"
)

// fetchCMS reads a public CMS page. No Authorization header is sent: this API
// is open, and sending an expired programming token would only invite a 401.
func fetchCMS(flags *rootFlags, path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, cmsBase+path, nil)
	if err != nil {
		return nil, fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("User-Agent", "cap-pp-cli")
	client := &http.Client{Timeout: flags.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, apiErr(fmt.Errorf("reading %s: %w", path, err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("reading the response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, notFoundErr(fmt.Errorf("no such page: %s", path))
	}
	if resp.StatusCode >= 400 {
		return nil, apiErr(fmt.Errorf("CrossFit's CMS answered HTTP %d for %s",
			resp.StatusCode, path))
	}
	return body, nil
}

func newCapMovementCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "movement <name>",
		Short: "Coaching material for one movement: faults, cues, progressions",
		Long: `Read a movement's coaching page: the overview, the faults a coach looks
for with the cues that fix each one, the teaching progressions, and the
substitutions CrossFit offers.

The movement is named by its slug ("air-squat", "ring-muscle-up"); run
'cap-pp-cli cap movements' for the catalogue. Names are forgiving: "Air Squat"
and "air squat" both work.

This reads CrossFit's public CMS, so it needs no token and works even when the
programming token has expired.`,
		Example: `  cap-pp-cli cap movement air-squat
  cap-pp-cli cap movement "ring muscle-up" --agent`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := slugify(args[0])
			if dryRunOK(flags) {
				return nil
			}
			mv, err := fetchMovement(flags, slug)
			if err != nil {
				return err
			}
			if wantsJSON(cmd, flags) {
				return printCapJSON(cmd, flags, mv)
			}
			printMovementText(cmd, mv)
			return nil
		},
	}
}

// fetchMovement reads one movement and refuses to hand back a different one.
//
// The CMS is a cached, CDN-fronted service and this command exists to answer
// "how do I coach this movement" - cues for the wrong movement are worse than
// no answer, because nothing in the output would look wrong to a coach reading
// it. So the slug that comes back is checked against the slug that was asked
// for, with one retry before giving up.
//
// This is a guard, not a fix for an observed defect: a report of the CMS
// serving a neighbouring movement did not reproduce (22 requests, plus a sweep
// of all 81 catalogue entries, every one self-consistent). The check is kept
// because it costs one string comparison and removes the possibility of a
// silent swap entirely.
func fetchMovement(flags *rootFlags, slug string) (*capjson.Movement, error) {
	var lastGot string
	for attempt := 0; attempt < 2; attempt++ {
		body, err := fetchCMS(flags, "/movement/"+slug)
		if err != nil {
			// A misspelled movement is the common case; point at the catalogue.
			if ExitCode(err) == 3 {
				return nil, notFoundErr(fmt.Errorf(
					"no movement %q; run 'cap-pp-cli cap movements' for the list", slug))
			}
			return nil, err
		}
		mv, err := capjson.ParseMovement(body)
		if err != nil {
			if err == capjson.ErrNoContent {
				return nil, notFoundErr(fmt.Errorf(
					"no movement %q; run 'cap-pp-cli cap movements' for the list", slug))
			}
			return nil, err
		}
		if mv.Slug == "" || mv.Slug == slug {
			return mv, nil
		}
		lastGot = mv.Slug
	}
	return nil, apiErr(fmt.Errorf(
		"CrossFit's CMS returned the movement %q when asked for %q, twice; "+
			"try again shortly", lastGot, slug))
}

func newCapMovementsCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "movements [search]",
		Short:       "The movement catalogue",
		Long:        "List every movement with a coaching page, grouped by modality. An optional search term filters by name.",
		Example:     "  cap-pp-cli cap movements squat",
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCatalog(cmd, flags, pathMovements, args, true)
		},
	}
}

func newCapBenchmarksCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "benchmarks [search]",
		Short: "Benchmark, Hero and Open workouts",
		Long: `The published benchmark workouts, Hero workouts and Open workouts, with
their prescriptions.

An optional search term matches the name or the workout text, so both
'benchmarks murph' and 'benchmarks "wall walk"' work.`,
		Example:     "  cap-pp-cli cap benchmarks murph",
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCatalog(cmd, flags, pathBenchmark, args, false)
		},
	}
}

func newCapResourcesCmd(flags *rootFlags) *cobra.Command {
	var kids bool
	cmd := &cobra.Command{
		Use:   "resources [search]",
		Short: "Programming resources: warm-ups, progressions, coaching tips, scaling",
		Long: `CrossFit's programming resources: general warm-ups, teaching progressions
and specific warm-ups, coaching tips, scaling options, movement demos, benchmark
class plans, partner workouts and reference material.

--kids switches to the kids and teens training plans.`,
		Example:     "  cap-pp-cli cap resources warm-up",
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			path := pathResources
			if kids {
				path = pathKidsTeens
			}
			return runCatalog(cmd, flags, path, args, false)
		},
	}
	cmd.Flags().BoolVar(&kids, "kids", false, "Read the kids and teens training plans instead")
	return cmd
}

// runCatalog is the shared body of the listing commands.
func runCatalog(cmd *cobra.Command, flags *rootFlags, path string, args []string, wantSlugs bool) error {
	query := ""
	if len(args) == 1 {
		query = args[0]
	}
	if dryRunOK(flags) {
		return nil
	}
	body, err := fetchCMS(flags, path)
	if err != nil {
		return err
	}
	cat, err := capjson.ParseCatalog(body)
	if err != nil {
		return err
	}

	if query != "" {
		hits := cat.FindEntries(query)
		if wantsJSON(cmd, flags) {
			return printCapJSON(cmd, flags, map[string]any{
				"query": query, "matches": hits,
			})
		}
		if len(hits) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "nothing matches %q\n", query)
			return nil
		}
		for _, e := range hits {
			printCatalogEntry(cmd, e, wantSlugs)
		}
		return nil
	}

	if wantsJSON(cmd, flags) {
		return printCapJSON(cmd, flags, cat)
	}
	w := cmd.OutOrStdout()
	for _, s := range cat.Sections {
		if s.Title != "" {
			fmt.Fprintf(w, "\n%s (%d)\n", s.Title, len(s.Entries))
		}
		for _, e := range s.Entries {
			printCatalogEntry(cmd, e, wantSlugs)
		}
	}
	return nil
}

func printCatalogEntry(cmd *cobra.Command, e capjson.CatalogEntry, wantSlug bool) {
	w := cmd.OutOrStdout()
	if wantSlug && e.Slug != "" {
		fmt.Fprintf(w, "  %-32s %s\n", e.Slug, e.Name)
		return
	}
	fmt.Fprintf(w, "  %s\n", e.Name)
	if e.Body != "" {
		for _, line := range strings.Split(wrapTo(e.Body, 76), "\n") {
			fmt.Fprintf(w, "      %s\n", line)
		}
	}
}

func printMovementText(cmd *cobra.Command, m *capjson.Movement) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s (%s)\n", m.Name, m.Slug)
	if m.Description != "" {
		fmt.Fprintf(w, "\n%s\n", wrapTo(m.Description, 78))
	}
	if len(m.Faults) > 0 {
		fmt.Fprintf(w, "\nfaults and cues:\n")
		for _, f := range m.Faults {
			fmt.Fprintf(w, "  • %s\n", f.Fault)
			for _, c := range f.Cues {
				fmt.Fprintf(w, "      → %s\n", c)
			}
		}
	}
	for _, g := range m.Progressions {
		fmt.Fprintf(w, "\n%s\n", firstNonBlank(g.Title, "progressions"))
		for _, s := range g.Steps {
			if s.Focus != "" {
				fmt.Fprintf(w, "  • %s\n", s.Focus)
			}
			if s.Text != "" {
				fmt.Fprintf(w, "      %s\n", s.Text)
			}
		}
	}
	if len(m.Substitutions) > 0 {
		fmt.Fprintf(w, "\nsubstitutions: %s\n", strings.Join(m.Substitutions, ", "))
	}
	if m.YouTubeID != "" {
		fmt.Fprintf(w, "\nvideo: https://youtu.be/%s\n", m.YouTubeID)
	}
}

// slugify turns a human movement name into the CMS slug.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	// Collapse the double hyphens that "ring muscle-up" -> "ring-muscle-up"
	// style input can produce.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}

func firstNonBlank(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// wrapTo soft-wraps prose so a terminal read of a workout stays legible.
func wrapTo(s string, width int) string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		line := ""
		for _, word := range words {
			if line == "" {
				line = word
				continue
			}
			if len(line)+1+len(word) > width {
				out = append(out, line)
				line = word
				continue
			}
			line += " " + word
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
