package btwbhtml

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var (
	wsRe            = regexp.MustCompile(`\s+`)
	trailingDigitRe = regexp.MustCompile(`(\d+)\s*$`)
	parenRe         = regexp.MustCompile(`\(([^()]*)\)`)
	digitsRe        = regexp.MustCompile(`\d+`)
	trackClassRe    = regexp.MustCompile(`(?:^|\s)track_(\d+)(?:\s|$)`)
	dayHrefRe       = regexp.MustCompile(`whiteboard/day\?d=(\d{4}-\d{2}-\d{2})`)
	elementIDRe     = regexp.MustCompile(`_(\d+)$`)
)

// collapse trims the string and squeezes every run of whitespace (including the
// newlines and non-breaking spaces btwb's templates leave behind) into a single
// space.
func collapse(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	return strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
}

// collapseText is collapse applied to a selection's text.
func collapseText(sel *goquery.Selection) string {
	if sel.Length() == 0 {
		return ""
	}
	return collapse(sel.Text())
}

// normalizeLines converts CRLF/CR line endings to LF and trims surrounding
// whitespace while preserving the internal line structure.
func normalizeLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// textWithout returns the collapsed text of sel with every descendant matching
// drop removed. sel is cloned first, so the source document is untouched.
func textWithout(sel *goquery.Selection, drop string) string {
	if sel.Length() == 0 {
		return ""
	}
	clone := sel.First().Clone()
	clone.Find(drop).Remove()
	return collapse(clone.Text())
}

// textWithoutIcons returns the collapsed text of sel with icon <span>s removed.
// btwb prefixes every whiteboard entry with one or more <span class="icon-*">
// markers that carry no text of their own but would otherwise glue stray
// whitespace into the title.
func textWithoutIcons(sel *goquery.Selection) string {
	if sel.Length() == 0 {
		return ""
	}
	clone := sel.First().Clone()
	clone.Find("span").FilterFunction(func(_ int, s *goquery.Selection) bool {
		class, _ := s.Attr("class")
		return strings.Contains(class, "icon")
	}).Remove()
	return collapse(clone.Text())
}

// trailingInt returns the integer at the end of s ("…/track_events/323452610"
// -> 323452610).
func trailingInt(s string) int {
	m := trailingDigitRe.FindStringSubmatch(strings.TrimRight(s, "/"))
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// parenCount extracts the integer from the last parenthesised group of s.
// btwb renders result links as "<localised label> (14)", so the count is read
// out of the parentheses without depending on the label's language.
func parenCount(s string) (int, bool) {
	groups := parenRe.FindAllStringSubmatch(s, -1)
	if len(groups) == 0 {
		return 0, false
	}
	last := groups[len(groups)-1][1]
	digits := strings.Join(digitsRe.FindAllString(last, -1), "")
	if digits == "" {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return n, true
}

// trackIDFromClass reads the track id out of a whiteboard <li>'s class list,
// e.g. "blue track_200001" -> 200001.
func trackIDFromClass(class string) int {
	m := trackClassRe.FindStringSubmatch(class)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// idFromElementID reads the trailing numeric id out of a DOM id such as
// "following_track_200001" or "workout_session_130464021".
func idFromElementID(id string) int {
	m := elementIDRe.FindStringSubmatch(id)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}
