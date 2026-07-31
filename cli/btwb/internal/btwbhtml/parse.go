package btwbhtml

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Entry kinds reported in WodEntry.Kind.
const (
	KindPlanned = "planned"
	KindLogged  = "logged"
)

// taskWorkoutSession is the data-task value btwb uses for a logged result.
const taskWorkoutSession = "workout_session"

func parseDoc(page []byte) (*goquery.Document, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	return doc, nil
}

// ParseWeeks extracts every calendar day present in a btwb whiteboard page.
// The day and month whiteboard URLs share this markup, and a single page holds
// more than one week, so the result is the flattened set of days sorted by date.
//
// trackNames maps track id to display name (as produced by ParseTracks). Track
// ids missing from the map simply leave WodEntry.TrackName empty.
func ParseWeeks(page []byte, memberID int, trackNames map[int]string) ([]WhiteboardDay, error) {
	doc, err := parseDoc(page)
	if err != nil {
		return nil, err
	}

	weeks := doc.Find("div.box-week")
	if weeks.Length() == 0 {
		return nil, fmt.Errorf("no whiteboard weeks found: page has no div.box-week (not a whiteboard page, or the session was not authenticated)")
	}

	var days []WhiteboardDay
	weeks.Each(func(_ int, week *goquery.Selection) {
		week.Find("div.box.box-day").Each(func(_ int, box *goquery.Selection) {
			date := dayDate(box)
			if date == "" {
				return
			}
			days = append(days, WhiteboardDay{
				Date:     date,
				MemberID: memberID,
				Workouts: dayEntries(box, trackNames),
			})
		})
	})

	if len(days) == 0 {
		return nil, fmt.Errorf("no whiteboard days found: page has div.box-week but no dated div.box.box-day inside it")
	}

	sort.SliceStable(days, func(i, j int) bool { return days[i].Date < days[j].Date })
	return days, nil
}

// ParseDay extracts the single whiteboard day matching date (YYYY-MM-DD) from a
// btwb whiteboard page. The page normally carries a whole week (or two), so the
// requested date must be one of them.
func ParseDay(page []byte, memberID int, date string, trackNames map[int]string) (*WhiteboardDay, error) {
	days, err := ParseWeeks(page, memberID, trackNames)
	if err != nil {
		return nil, err
	}
	for i := range days {
		if days[i].Date == date {
			day := days[i]
			return &day, nil
		}
	}
	available := make([]string, 0, len(days))
	for _, d := range days {
		available = append(available, d.Date)
	}
	return nil, fmt.Errorf("date %s not present in page; page covers %s..%s (%d days)",
		date, available[0], available[len(available)-1], len(available))
}

// dayDate reads a day box's ISO date out of its "view day" link.
func dayDate(box *goquery.Selection) string {
	date := ""
	box.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href, _ := a.Attr("href")
		if m := dayHrefRe.FindStringSubmatch(href); m != nil {
			date = m[1]
			return false
		}
		return true
	})
	return date
}

// dayEntries reads the whiteboard entries out of one day box.
func dayEntries(box *goquery.Selection, trackNames map[int]string) []WodEntry {
	var entries []WodEntry
	box.Find("ul.event-list > li").Each(func(_ int, li *goquery.Selection) {
		details := li.Find("div.view-task-details").First()
		if details.Length() == 0 {
			return
		}
		uri, _ := details.Attr("data-uri")
		task, _ := details.Attr("data-task")

		kind := KindPlanned
		if task == taskWorkoutSession {
			kind = KindLogged
		}

		class, _ := li.Attr("class")
		trackID := trackIDFromClass(class)

		entries = append(entries, WodEntry{
			Kind:       kind,
			Title:      textWithoutIcons(details),
			TrackID:    trackID,
			TrackName:  trackNames[trackID],
			EventID:    trailingInt(uri),
			DetailPath: uri,
		})
	})
	return entries
}

// ParseEvent extracts the detail of a single whiteboard entry from the fragment
// btwb serves at /tasks/members/<member>/track_events/<id> (and the
// workout_sessions sibling). Planned workouts, coach notes and rest days all
// share the same wrapper but expose very different subsets of the markup, so
// every field is optional: a fragment that is a task detail but carries nothing
// beyond a title yields a Wod with zero values rather than an error.
func ParseEvent(fragment []byte, eventID int) (*Wod, error) {
	doc, err := parseDoc(fragment)
	if err != nil {
		return nil, err
	}

	root := doc.Find("div.task-event-details").First()
	if root.Length() == 0 {
		return nil, fmt.Errorf("not a task detail fragment: no div.task-event-details in %d bytes of html", len(fragment))
	}

	wod := &Wod{EventID: eventID}
	wod.TrackName, wod.Kind = splitDataTitle(dataTitle(root))

	// Variant sits in the page header; the category is a <span class="label">
	// nested inside it and has to come back out of the variant text.
	if h2 := root.Find("h2.pull-left").First(); h2.Length() > 0 {
		wod.Category = collapseText(h2.Find("span.label").First())
		wod.Variant = textWithout(h2, "span.label")
	}

	// The workout description box is the first .modality-stats.box; later ones
	// are the milestone and previous-result panels.
	wod.Name = collapseText(root.Find(".modality-stats.box .stats-header .col").First())
	if wod.Name == "" {
		// Notes and rest days have no stats box, only a plain title.
		wod.Name = collapseText(root.Find("p.event-title").First())
	}

	if desc := root.Find("p.workout-description").First(); desc.Length() > 0 {
		wod.Description = normalizeLines(desc.Text())
	}
	if instr := root.Find("p.event-instructions").First(); instr.Length() > 0 {
		wod.Instructions = normalizeLines(instr.Text())
	}

	wod.ResultsCount, wod.ResultsPath = resultsLink(root)
	if log := root.Find(`a[href*="/workout_sessions/new"]`).First(); log.Length() > 0 {
		wod.LogPath, _ = log.Attr("href")
	}
	wod.PreviousResult = previousResult(root)
	wod.Movements = movements(root)

	return wod, nil
}

// dataTitle returns the data-title of the fragment root, falling back to the
// first descendant that carries one (logged-result fragments put it on an inner
// wrapper).
func dataTitle(root *goquery.Selection) string {
	if t, ok := root.Attr("data-title"); ok {
		return t
	}
	t, _ := root.Find("[data-title]").First().Attr("data-title")
	return t
}

// splitDataTitle splits "<track name> : <kind>" on the LAST " : ", because
// track names may themselves contain a colon-free separator such as
// "CAP - Compete" while kinds never contain " : ". A title without the
// separator is treated as a bare kind.
func splitDataTitle(title string) (trackName, kind string) {
	title = collapse(title)
	if title == "" {
		return "", ""
	}
	if i := strings.LastIndex(title, " : "); i >= 0 {
		return collapse(title[:i]), collapse(title[i+len(" : "):])
	}
	return "", title
}

// resultsLink finds the "view results" link. Several links point at
// /track_events/<id>; the one that carries a parenthesised count is the results
// list, the other is a plain button. Prefer the counted one.
func resultsLink(root *goquery.Selection) (count int, path string) {
	fallback := ""
	root.Find(`a[href^="/track_events/"]`).EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href, _ := a.Attr("href")
		if n, ok := parenCount(collapseText(a)); ok {
			count, path = n, href
			return false
		}
		if fallback == "" {
			fallback = href
		}
		return true
	})
	if path == "" {
		path = fallback
	}
	return count, path
}

// previousResult reads the previous-results panel. When the member has no
// previous result for the workout btwb renders a single bare <p> sentence; a
// real result is always rendered inside markup (links, <b>, slider divs). So
// treat a box whose only element children are childless <p>s as empty rather
// than returning the localised "you have no previous result" sentence.
func previousResult(root *goquery.Selection) string {
	box := root.Find(".previous-results-box .box-info").First()
	if box.Length() == 0 {
		return ""
	}
	structured := false
	box.Children().Each(func(_ int, child *goquery.Selection) {
		if goquery.NodeName(child) != "p" || child.Children().Length() > 0 {
			structured = true
		}
	})
	if !structured {
		return ""
	}
	return collapse(box.Text())
}

// movements lists the movement names linked from the movement tab strip.
func movements(root *goquery.Selection) []string {
	var out []string
	root.Find(".movement-tabs a").Each(func(_ int, a *goquery.Selection) {
		if name := collapseText(a); name != "" {
			out = append(out, name)
		}
	})
	return out
}

// ParseTracks extracts the member's programming tracks from the tracks page.
//
// Track names are returned verbatim, including btwb's trailing "*" marker,
// which the page's own legend explains as "this track is not available to you".
func ParseTracks(page []byte) ([]Track, error) {
	doc, err := parseDoc(page)
	if err != nil {
		return nil, err
	}

	var tracks []Track
	doc.Find(`li[id^="following_track_"]`).Each(func(_ int, li *goquery.Selection) {
		id, _ := li.Attr("id")
		trackID := idFromElementID(id)
		if trackID == 0 {
			return
		}
		// The follow control is a toggle: the "following" class marks the
		// current state (and its data-method is then "delete", i.e. clicking
		// unfollows). Tracks with no control at all are not followed.
		following := false
		if toggle := li.Find("a.follow-track").First(); toggle.Length() > 0 {
			following = toggle.HasClass("following")
		}
		tracks = append(tracks, Track{
			ID:        trackID,
			Name:      collapseText(li.Find(".track-name").First()),
			Following: following,
		})
	})

	if len(tracks) == 0 {
		if doc.Find("#tracks").Length() == 0 {
			return nil, fmt.Errorf("not a tracks page: no li[id^=following_track_] and no #tracks container")
		}
		return []Track{}, nil
	}
	return tracks, nil
}

var (
	workoutSessionHref = regexp.MustCompile(`^/workout_sessions/(\d+)`)
	memberSessionsHref = regexp.MustCompile(`^/members/(\d+)/workout_sessions`)
	memberHref         = regexp.MustCompile(`^/members/(\d+)(?:/|$)`)
	rxdRe              = regexp.MustCompile(`(?i)^(not\s+)?rx'?d$`)
)

// ParseSessions extracts the member's logged results from the workout sessions
// page.
func ParseSessions(page []byte) ([]LoggedResult, error) {
	doc, err := parseDoc(page)
	if err != nil {
		return nil, err
	}
	memberID := pageMemberID(doc)

	var results []LoggedResult
	doc.Find(`li.workout_session[id^="workout_session_"]`).Each(func(_ int, li *goquery.Selection) {
		id, _ := li.Attr("id")
		sessionID := idFromElementID(id)
		if sessionID == 0 {
			return
		}

		title := li.Find(".item_title").First()
		name := collapseText(title.Find(`a[href^="/workouts/"]`).First())
		result := collapseText(title.Find(`b a[href^="/workout_sessions/"]`).First())
		if result == "" {
			result = collapseText(title.Find("b").First())
		}

		results = append(results, LoggedResult{
			SessionID:    sessionID,
			Date:         parseFeedDate(collapseText(li.Find(".post-privacy-text").First())),
			WorkoutName:  name,
			Result:       result,
			IsPrescribed: isPrescribed(result),
			Notes:        collapseText(li.Find(".feed_item_info > i").First()),
			DetailPath:   sessionDetailPath(memberID, sessionID),
		})
	})

	if len(results) == 0 {
		if doc.Find(".workout-sessions, #workout-session-list").Length() == 0 {
			return nil, fmt.Errorf("not a workout sessions page: no li.workout_session and no .workout-sessions container")
		}
		return []LoggedResult{}, nil
	}
	return results, nil
}

// sessionDetailPath builds the path of the detail fragment for a logged result,
// matching the data-uri the whiteboard uses for the same entry.
func sessionDetailPath(memberID, sessionID int) string {
	if memberID > 0 {
		return fmt.Sprintf("/tasks/members/%d/workout_sessions/%d", memberID, sessionID)
	}
	return fmt.Sprintf("/workout_sessions/%d", sessionID)
}

// pageMemberID recovers the member the page belongs to from its own links, so
// that detail paths can be built without the caller passing the id in.
func pageMemberID(doc *goquery.Document) int {
	id := 0
	doc.Find(`a[href^="/members/"], form[action^="/members/"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		ref, ok := s.Attr("href")
		if !ok {
			ref, _ = s.Attr("action")
		}
		if m := memberSessionsHref.FindStringSubmatch(ref); m != nil {
			id, _ = strconv.Atoi(m[1])
			return false
		}
		if id == 0 {
			if m := memberHref.FindStringSubmatch(ref); m != nil {
				id, _ = strconv.Atoi(m[1])
			}
		}
		return true
	})
	return id
}

// isPrescribed reports whether a result string is flagged as Rx'd. btwb appends
// the flag as the last pipe-separated segment and leaves it in English in every
// locale (the template carries it as a literal data-prescribed-suffix of
// " | Rx'd"), so the segment is compared against that literal.
func isPrescribed(result string) bool {
	parts := strings.Split(result, "|")
	if len(parts) < 2 {
		return false
	}
	last := collapse(parts[len(parts)-1])
	last = strings.ReplaceAll(last, "’", "'")
	m := rxdRe.FindStringSubmatch(last)
	return m != nil && m[1] == ""
}

// monthNames maps the month spellings btwb renders in the locales this package
// has seen to month numbers: Russian (genitive, as used in dates, plus
// nominative) and English (full plus three-letter abbreviations).
var monthNames = map[string]int{
	"января": 1, "февраля": 2, "марта": 3, "апреля": 4, "мая": 5, "июня": 6,
	"июля": 7, "августа": 8, "сентября": 9, "октября": 10, "ноября": 11, "декабря": 12,

	"январь": 1, "февраль": 2, "март": 3, "апрель": 4, "май": 5, "июнь": 6,
	"июль": 7, "август": 8, "сентябрь": 9, "октябрь": 10, "ноябрь": 11, "декабрь": 12,

	"january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6,
	"july": 7, "august": 8, "september": 9, "october": 10, "november": 11, "december": 12,

	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "sept": 9, "oct": 10, "nov": 11, "dec": 12,
}

var (
	dateTokenRe = regexp.MustCompile(`[\p{L}]+|\d+`)
	isoDateRe   = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
)

// parseFeedDate normalises a rendered feed date to YYYY-MM-DD. The day and year
// are read as digits, and the month from a name table, because btwb renders the
// month as a localised word with no machine-readable attribute anywhere near it.
// A date whose month cannot be resolved is returned verbatim rather than dropped.
func parseFeedDate(s string) string {
	if s == "" {
		return ""
	}
	// Already ISO.
	if m := isoDateRe.FindString(s); m != "" {
		return m
	}

	day, year, month := 0, 0, 0
	for _, tok := range dateTokenRe.FindAllString(s, -1) {
		if n, err := strconv.Atoi(tok); err == nil {
			switch {
			case n >= 1000:
				if year == 0 {
					year = n
				}
			case n >= 1 && n <= 31:
				if day == 0 {
					day = n
				}
			}
			continue
		}
		if month == 0 {
			if n, ok := monthNames[strings.ToLower(tok)]; ok {
				month = n
			}
		}
	}
	if day == 0 || month == 0 || year == 0 {
		return s
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}
