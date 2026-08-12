// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.

package capjson

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Movement is one movement's coaching material: how it should look, what goes
// wrong, and what to say when it does.
//
// This comes from a different API than the programming: the CMS at
// cms-api.crossfit.com, which is public - no token, no subscription. So the
// movement commands keep working when the programming token has expired.
type Movement struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	YouTubeID   string `json:"youtube_id,omitempty"`
	GIF         string `json:"gif,omitempty"`

	// Faults are the errors a coach looks for, each with the cues that fix it.
	Faults []Fault `json:"faults,omitempty"`
	// Progressions are the teaching drills, grouped as CAP groups them.
	Progressions []ProgressionGroup `json:"progressions,omitempty"`
	// Substitutions are movements offered as alternatives.
	Substitutions []string `json:"substitutions,omitempty"`
}

// Fault is one thing that goes wrong, and what to say about it.
type Fault struct {
	Fault string   `json:"fault"`
	Cues  []string `json:"cues,omitempty"`
}

// ProgressionGroup is a titled set of teaching drills.
type ProgressionGroup struct {
	Title string        `json:"title,omitempty"`
	Steps []Progression `json:"steps,omitempty"`
}

// Progression is one drill: what to focus on, and what to do.
type Progression struct {
	Focus string `json:"focus,omitempty"`
	Text  string `json:"text,omitempty"`
}

// CatalogEntry is one item in a CMS listing page: a movement in the movement
// catalogue, a benchmark in the benchmarks page, a resource in the programming
// resources page.
type CatalogEntry struct {
	Name string `json:"name"`
	// Slug is filled when the link points at a page this CLI can fetch
	// directly, e.g. a movement.
	Slug string `json:"slug,omitempty"`
	Body string `json:"body,omitempty"`
	Link string `json:"link,omitempty"`
}

// CatalogSection is one headed group of entries on a listing page.
type CatalogSection struct {
	Title   string         `json:"title"`
	Entries []CatalogEntry `json:"entries"`
}

// Catalog is a whole CMS listing page reduced to its sections.
type Catalog struct {
	Title    string           `json:"title"`
	Sections []CatalogSection `json:"sections"`
}

// Count returns the total number of entries across all sections.
func (c *Catalog) Count() int {
	n := 0
	for _, s := range c.Sections {
		n += len(s.Entries)
	}
	return n
}

// --- CMS wire shapes -------------------------------------------------------

type cmsEnvelope struct {
	Tiles []cmsTile `json:"tiles"`
}

type cmsTile struct {
	Title string          `json:"title"`
	Slug  string          `json:"slug"`
	ACF   json.RawMessage `json:"acf"`
}

func firstCMSTile(body []byte) (*cmsTile, error) {
	var env cmsEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decoding CMS response: %w", err)
	}
	if len(env.Tiles) == 0 {
		return nil, ErrNoContent
	}
	return &env.Tiles[0], nil
}

type movementACF struct {
	Slug    string `json:"slug"`
	Details struct {
		Overview struct {
			Description string `json:"description"`
			GIF         string `json:"gif"`
			YouTubeID   string `json:"youtube_id"`
		} `json:"overview"`
		FaultsAndCorrections []struct {
			Fault struct {
				Description string `json:"description"`
			} `json:"fault"`
			Corrections []struct {
				Cue string `json:"cue"`
			} `json:"corrections"`
		} `json:"faults_and_corrections"`
		LearningProgressions []struct {
			Title        string `json:"title"`
			Progressions []struct {
				Focus string `json:"focus"`
				Text  string `json:"text"`
			} `json:"progressions"`
		} `json:"learning_progressions"`
		Substitutions []struct {
			HeadlineText string `json:"headline_text"`
		} `json:"substitutions"`
	} `json:"details"`
}

// ParseMovement turns a CMS movement page into a Movement.
func ParseMovement(body []byte) (*Movement, error) {
	tile, err := firstCMSTile(body)
	if err != nil {
		return nil, err
	}
	var a movementACF
	if err := json.Unmarshal(tile.ACF, &a); err != nil {
		return nil, fmt.Errorf("decoding movement: %w", err)
	}

	m := &Movement{
		Slug:        firstNonEmpty(a.Slug, tile.Slug),
		Name:        cleanText(tile.Title),
		Description: cleanText(a.Details.Overview.Description),
		YouTubeID:   a.Details.Overview.YouTubeID,
		GIF:         a.Details.Overview.GIF,
	}
	for _, f := range a.Details.FaultsAndCorrections {
		fault := Fault{Fault: cleanText(f.Fault.Description)}
		for _, c := range f.Corrections {
			if cue := cleanText(c.Cue); cue != "" {
				fault.Cues = append(fault.Cues, cue)
			}
		}
		if fault.Fault != "" || len(fault.Cues) > 0 {
			m.Faults = append(m.Faults, fault)
		}
	}
	for _, g := range a.Details.LearningProgressions {
		group := ProgressionGroup{Title: cleanText(g.Title)}
		for _, p := range g.Progressions {
			step := Progression{Focus: cleanText(p.Focus), Text: cleanText(p.Text)}
			if step.Focus != "" || step.Text != "" {
				group.Steps = append(group.Steps, step)
			}
		}
		if len(group.Steps) > 0 || group.Title != "" {
			m.Progressions = append(m.Progressions, group)
		}
	}
	for _, s := range a.Details.Substitutions {
		if n := cleanText(s.HeadlineText); n != "" {
			m.Substitutions = append(m.Substitutions, n)
		}
	}
	return m, nil
}

// catalogACF is the flexible-content page model the CMS listing pages share.
// Two component layouts carry entries, under different key names, and the rest
// (hero, text_block) are page furniture.
type catalogACF struct {
	Components []struct {
		Layout string `json:"acf_fc_layout"`
		List   struct {
			Headline string        `json:"headline_text"`
			Items    []catalogItem `json:"content_items"`
		} `json:"curated_content_list"`
		Resources struct {
			Headline string        `json:"headline_text"`
			Items    []catalogItem `json:"training_resources"`
			Cards    []catalogItem `json:"content_items"`
		} `json:"curated_training_resource_list"`
	} `json:"components"`
}

type catalogItem struct {
	HeadlineText string          `json:"headline_text"`
	BodyText     string          `json:"body_text"`
	FullPath     string          `json:"full_path"`
	Link         json.RawMessage `json:"link"`
}

// url pulls the destination out of the link field, which the CMS serialises
// either as an object or, on some pages, as a bare string.
func (i catalogItem) url() string {
	if i.FullPath != "" {
		return i.FullPath
	}
	if len(i.Link) == 0 {
		return ""
	}
	var obj struct {
		URL      string `json:"url"`
		FullPath string `json:"full_path"`
	}
	if err := json.Unmarshal(i.Link, &obj); err == nil {
		return firstNonEmpty(obj.FullPath, obj.URL)
	}
	var s string
	if err := json.Unmarshal(i.Link, &s); err == nil {
		return s
	}
	return ""
}

// ParseCatalog turns any CMS listing page into sections of entries.
func ParseCatalog(body []byte) (*Catalog, error) {
	tile, err := firstCMSTile(body)
	if err != nil {
		return nil, err
	}
	var a catalogACF
	if err := json.Unmarshal(tile.ACF, &a); err != nil {
		return nil, fmt.Errorf("decoding catalog page: %w", err)
	}

	cat := &Catalog{Title: cleanText(tile.Title)}
	for _, comp := range a.Components {
		var headline string
		var items []catalogItem
		switch comp.Layout {
		case "curated_content_list":
			headline, items = comp.List.Headline, comp.List.Items
		case "curated_training_resource_list":
			headline = comp.Resources.Headline
			items = append(append([]catalogItem{}, comp.Resources.Items...), comp.Resources.Cards...)
		default:
			continue // hero, text_block: page furniture, not entries
		}
		section := CatalogSection{Title: cleanText(headline)}
		for _, it := range items {
			name := cleanText(it.HeadlineText)
			if name == "" {
				continue
			}
			link := it.url()
			section.Entries = append(section.Entries, CatalogEntry{
				Name: name,
				Slug: movementSlug(link),
				Body: cleanText(it.BodyText),
				Link: link,
			})
		}
		if len(section.Entries) > 0 {
			cat.Sections = append(cat.Sections, section)
		}
	}
	return cat, nil
}

// movementSlug extracts the slug from a /movement/<slug> link so a catalogue
// entry can be fed straight back into ParseMovement's fetch.
func movementSlug(link string) string {
	const marker = "/movement/"
	i := strings.Index(link, marker)
	if i < 0 {
		return ""
	}
	slug := link[i+len(marker):]
	if j := strings.IndexAny(slug, "/?#"); j >= 0 {
		slug = slug[:j]
	}
	return slug
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// FindEntries returns catalogue entries whose name or body matches a query,
// case-insensitively. An empty query returns everything.
func (c *Catalog) FindEntries(query string) []CatalogEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []CatalogEntry
	for _, s := range c.Sections {
		for _, e := range s.Entries {
			if q == "" ||
				strings.Contains(strings.ToLower(e.Name), q) ||
				strings.Contains(strings.ToLower(e.Body), q) {
				out = append(out, e)
			}
		}
	}
	return out
}
