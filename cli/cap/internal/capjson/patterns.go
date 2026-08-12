// Copyright 2026 isolovyev. Licensed under Apache-2.0. See LICENSE.

package capjson

import (
	"sort"
	"strings"
)

// Modality is the coarse CrossFit bucket a movement falls in.
type Modality string

const (
	Gymnastics    Modality = "gymnastics"    // bodyweight, control over one's own body
	Weightlifting Modality = "weightlifting" // external load
	Monostructural Modality = "monostructural" // cyclic cardio (row, bike, run, jump rope)
)

// Pattern is the movement pattern that decides whether two days "collide". Two
// sessions that both hammer the same pattern under load are what an athlete
// feels as an overlap, more than a matching load number alone.
type Pattern string

const (
	Squat         Pattern = "squat"          // knee-dominant: squats, wall balls, thrusters, box jumps, lunges
	Hinge         Pattern = "hinge"          // hip-dominant: deadlifts, KB swings, cleans, snatches, GHD
	VerticalPush  Pattern = "vertical_push"  // overhead press, jerk, HSPU
	HorizontalPush Pattern = "horizontal_push" // bench, push-up, burpee
	VerticalPull  Pattern = "vertical_pull"  // pull-up, rope climb, muscle-up
	HorizontalPull Pattern = "horizontal_pull" // row (the strength movement), ring row
	Olympic       Pattern = "olympic"        // explosive triple extension: snatch, clean, jerk
	Core          Pattern = "core"           // midline: toes-to-bar, sit-ups, GHD, planks
	Mono          Pattern = "mono"           // cyclic engine: row/bike/run/jump rope
)

// MovementTags is one movement's classification.
type MovementTags struct {
	Movement string    `json:"movement"`
	Modality Modality  `json:"modality"`
	Patterns []Pattern `json:"patterns"`
}

// classRule maps a substring in a lowercased movement name to a modality and
// the patterns it trains. Rules are tried in order and the FIRST match wins, so
// specific names ("wall ball", "push press") must precede the generic tokens
// they contain ("ball", "press"). A movement can carry more than one pattern:
// a thruster is squat plus vertical push, a snatch is hinge plus olympic.
var classRules = []struct {
	needles  []string
	modality Modality
	patterns []Pattern
}{
	// Monostructural engine. Checked first: these names are unambiguous.
	{[]string{"row"}, Monostructural, []Pattern{Mono, HorizontalPull}}, // erg row: engine, with a pull
	{[]string{"bike", "echo", "assault", "ski"}, Monostructural, []Pattern{Mono}},
	{[]string{"run", "shuttle"}, Monostructural, []Pattern{Mono}},
	{[]string{"double-under", "single-under", "jump rope", "du"}, Monostructural, []Pattern{Mono}},
	{[]string{"swim"}, Monostructural, []Pattern{Mono}},

	// Olympic lifts and their variants: explosive hip extension.
	{[]string{"snatch"}, Weightlifting, []Pattern{Olympic, Hinge}},
	{[]string{"clean and jerk", "clean & jerk"}, Weightlifting, []Pattern{Olympic, Hinge, VerticalPush}},
	{[]string{"clean"}, Weightlifting, []Pattern{Olympic, Hinge}},
	{[]string{"jerk"}, Weightlifting, []Pattern{Olympic, VerticalPush}},

	// Compound barbell/DB combos: name them before their parts.
	{[]string{"thruster"}, Weightlifting, []Pattern{Squat, VerticalPush}},
	{[]string{"wall ball", "wall-ball"}, Weightlifting, []Pattern{Squat, VerticalPush}},
	{[]string{"shoulder to overhead", "shoulder-to-overhead", "s2oh"}, Weightlifting, []Pattern{VerticalPush}},
	{[]string{"push press", "push jerk", "strict press", "shoulder press", "press"}, Weightlifting, []Pattern{VerticalPush}},

	// Hinge.
	{[]string{"deadlift"}, Weightlifting, []Pattern{Hinge}},
	{[]string{"kettlebell swing", "kb swing", "swing"}, Weightlifting, []Pattern{Hinge}},
	{[]string{"good morning", "hip extension", "back extension"}, Weightlifting, []Pattern{Hinge}},
	{[]string{"clean pull", "snatch pull", "high pull"}, Weightlifting, []Pattern{Hinge, Olympic}},

	// Squat / knee-dominant.
	{[]string{"lunge"}, Weightlifting, []Pattern{Squat}},
	{[]string{"pistol"}, Gymnastics, []Pattern{Squat}},
	{[]string{"air squat", "squat"}, Weightlifting, []Pattern{Squat}}, // back/front/overhead/air squat
	{[]string{"box jump", "box step", "step-up", "step up"}, Gymnastics, []Pattern{Squat, Mono}},

	// Vertical push (gymnastics). Before the generic "push-up" rule, since a
	// handstand push-up contains that substring but is a press, not a horizontal
	// push.
	{[]string{"handstand push-up", "handstand push up", "hspu"}, Gymnastics, []Pattern{VerticalPush}},
	{[]string{"handstand walk", "handstand hold", "handstand"}, Gymnastics, []Pattern{VerticalPush, Core}},
	{[]string{"wall walk"}, Gymnastics, []Pattern{VerticalPush, Core}},

	// Jumps and rolls: knee-dominant power plus engine.
	{[]string{"broad jump", "jumping lunge", "tuck jump"}, Gymnastics, []Pattern{Squat, Mono}},
	{[]string{"candlestick", "forward roll"}, Gymnastics, []Pattern{Core}},

	// Horizontal push.
	{[]string{"bench press", "bench"}, Weightlifting, []Pattern{HorizontalPush}},
	{[]string{"push-up", "push up", "pushup"}, Gymnastics, []Pattern{HorizontalPush}},
	{[]string{"burpee"}, Gymnastics, []Pattern{HorizontalPush, Mono}},
	{[]string{"dip"}, Gymnastics, []Pattern{HorizontalPush}},

	// Vertical pull.
	{[]string{"muscle-up", "muscle up"}, Gymnastics, []Pattern{VerticalPull}},
	{[]string{"rope climb"}, Gymnastics, []Pattern{VerticalPull}},
	{[]string{"pull-up", "pull up", "pullup", "chin-up", "chin up"}, Gymnastics, []Pattern{VerticalPull}},

	// Horizontal pull (strength).
	{[]string{"ring row", "bent-over row", "barbell row", "dumbbell row", "seated row"}, Weightlifting, []Pattern{HorizontalPull}},

	// Core / midline.
	{[]string{"toes-to-bar", "toes to bar", "t2b", "knees-to-elbow", "knees to elbow", "k2e"}, Gymnastics, []Pattern{Core, VerticalPull}},
	{[]string{"sit-up", "sit up", "situp", "ghd", "v-up", "hollow", "plank", "l-sit", "leg raise"}, Gymnastics, []Pattern{Core}},

	// Carries: loaded gait.
	{[]string{"farmers carry", "farmer's carry", "carry", "sled"}, Weightlifting, []Pattern{Hinge, Mono}},
}

// ClassifyMovement tags one movement by name. An unrecognised movement gets an
// empty modality and no patterns rather than a wrong guess; callers can surface
// the gap. New movement names appear regularly, so silence beats a bad tag.
func ClassifyMovement(name string) MovementTags {
	n := normaliseMovement(name)
	for _, rule := range classRules {
		for _, needle := range rule.needles {
			if strings.Contains(n, needle) {
				return MovementTags{Movement: name, Modality: rule.modality, Patterns: rule.patterns}
			}
		}
	}
	return MovementTags{Movement: name}
}

func normaliseMovement(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	// Strip equipment qualifiers that do not change the pattern, so
	// "Dumbbell hang power clean" and "Power clean" classify the same.
	for _, q := range []string{
		"dumbbell ", "db ", "single-dumbbell ", "single dumbbell ",
		"barbell ", "kettlebell ", "kb ", "weighted ", "single-arm ",
		"hang ", "power ", "deficit ", "strict ", "kipping ", "alternating ",
		"single-leg ", "single leg ",
	} {
		n = strings.ReplaceAll(n, q, "")
	}
	return strings.TrimSpace(n)
}

// Profile is the reduced form of a day used for comparison and reordering: the
// numeric vectors CAP assigns, plus the movement-pattern and modality tags the
// day trains.
type Profile struct {
	Date      string     `json:"date"`
	Name      string     `json:"name"`
	Load      int        `json:"load"`
	Volume    int        `json:"volume"`
	Skill     int        `json:"skill"`
	Modalities []Modality `json:"modalities"`
	Patterns  []Pattern  `json:"patterns"`
	// Unknown lists movements the classifier could not tag, so a thin profile
	// is visibly thin rather than silently empty.
	Unknown []string `json:"unknown_movements,omitempty"`
}

// ClassifyText scans a prescription for movements the rules recognise and
// returns what it trains.
//
// This exists because Day.Movements comes from CAP's scaling section, which
// only lists movements that NEED scaling. A 400-m run or a row inside the
// workout often has no scaling entry, so a profile built from the scaling list
// alone silently loses the day's engine work - which is exactly the kind of
// overlap the reorder logic must see.
func ClassifyText(text string) (mods []Modality, pats []Pattern) {
	t := strings.ToLower(text)
	modSet := map[Modality]bool{}
	patSet := map[Pattern]bool{}
	for _, rule := range classRules {
		for _, needle := range rule.needles {
			if !containsWord(t, needle) {
				continue
			}
			if rule.modality != "" {
				modSet[rule.modality] = true
			}
			for _, p := range rule.patterns {
				patSet[p] = true
			}
			break // one match per rule is enough
		}
	}
	for m := range modSet {
		mods = append(mods, m)
	}
	for p := range patSet {
		pats = append(pats, p)
	}
	return mods, pats
}

// containsWord matches a needle only at word boundaries, so "row" does not fire
// on "narrow" and "press" does not fire on "impressive". A plural suffix is
// allowed, because prescriptions are written in the plural: "20 push presses",
// "15 box jump-overs", "5 burpees".
func containsWord(haystack, needle string) bool {
	from := 0
	for {
		i := strings.Index(haystack[from:], needle)
		if i < 0 {
			return false
		}
		i += from
		startOK := i == 0 || !isWordByte(haystack[i-1])
		if startOK && endsAtWordBoundary(haystack, i+len(needle)) {
			return true
		}
		from = i + 1
	}
}

// endsAtWordBoundary reports whether position end terminates a word, allowing
// an "s" or "es" plural first.
func endsAtWordBoundary(s string, end int) bool {
	for _, suffix := range []string{"", "s", "es"} {
		stop := end + len(suffix)
		if stop > len(s) {
			continue
		}
		if s[end:stop] != suffix {
			continue
		}
		if stop >= len(s) || !isWordByte(s[stop]) {
			return true
		}
	}
	return false
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// Profile reduces a day to its comparable shape.
func (d *Day) Profile() Profile {
	p := Profile{Date: d.Date, Name: d.Name, Load: d.Load, Volume: d.Volume, Skill: d.Skill}
	modSet := map[Modality]bool{}
	patSet := map[Pattern]bool{}
	for _, m := range d.Movements {
		tags := ClassifyMovement(m)
		if tags.Modality == "" && len(tags.Patterns) == 0 {
			p.Unknown = append(p.Unknown, m)
			continue
		}
		if tags.Modality != "" {
			modSet[tags.Modality] = true
		}
		for _, pat := range tags.Patterns {
			patSet[pat] = true
		}
	}

	// Fill the gaps the scaling list leaves by reading the prescription itself.
	// The Rx level is the reference; other levels are the same work scaled.
	for _, w := range d.Workouts {
		if w.Level != "" && w.Level != "rx" && len(d.Workouts) > 1 {
			continue
		}
		mods, pats := ClassifyText(w.Description)
		for _, m := range mods {
			modSet[m] = true
		}
		for _, pat := range pats {
			patSet[pat] = true
		}
	}
	for m := range modSet {
		p.Modalities = append(p.Modalities, m)
	}
	for pat := range patSet {
		p.Patterns = append(p.Patterns, pat)
	}
	sort.Slice(p.Modalities, func(i, j int) bool { return p.Modalities[i] < p.Modalities[j] })
	sort.Slice(p.Patterns, func(i, j int) bool { return p.Patterns[i] < p.Patterns[j] })
	return p
}

// Overlap scores how much two days collide, higher meaning more overlap and so
// a worse pair to place on consecutive training days. The score has two parts,
// kept separate so the reorder optimiser (and a human reading `cap compare`)
// can see which is driving it:
//
//   - PatternOverlap: shared movement patterns, the count of patterns both days
//     train. This is the primary signal - back-to-back hinge or vertical-pull
//     days are what fatigue an athlete.
//   - VectorDistance: how far apart the load/volume/skill numbers are. Small
//     distance means similar overall demand; the optimiser wants variety, so a
//     small distance adds to the collision score.
type Overlap struct {
	SharedPatterns  []Pattern `json:"shared_patterns"`
	SharedModalities []Modality `json:"shared_modalities"`
	VectorDistance  int       `json:"vector_distance"`
	Score           int       `json:"score"`
}

// CollisionScore compares two profiles. Higher = collide more.
func (a Profile) CollisionScore(b Profile) Overlap {
	shared := sharedPatterns(a.Patterns, b.Patterns)
	sharedMod := sharedModalities(a.Modalities, b.Modalities)
	dist := abs(a.Load-b.Load) + abs(a.Volume-b.Volume) + abs(a.Skill-b.Skill)

	// Each shared pattern weighs 2; each shared modality weighs 1. Vector
	// closeness contributes inversely: a max L1 distance of 12 (three axes,
	// 0..4 each) means no vector collision, distance 0 means identical demand.
	score := 2*len(shared) + len(sharedMod) + max(0, 6-dist)
	return Overlap{
		SharedPatterns:   shared,
		SharedModalities: sharedMod,
		VectorDistance:   dist,
		Score:            score,
	}
}

func sharedPatterns(a, b []Pattern) []Pattern {
	set := map[Pattern]bool{}
	for _, p := range a {
		set[p] = true
	}
	var out []Pattern
	for _, p := range b {
		if set[p] {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sharedModalities(a, b []Modality) []Modality {
	set := map[Modality]bool{}
	for _, m := range a {
		set[m] = true
	}
	var out []Modality
	for _, m := range b {
		if set[m] {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
