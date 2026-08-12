---
name: pp-cap
description: Read CrossFit Affiliate Programming (CAP) and CrossFit's coaching library - daily class plans, weekly overviews, warm-ups, movement faults and cues, benchmark/Hero/Open workouts, programming resources, and hero/skill preparation tracks. Also compares two days by training demand. Use when asked about a CAP workout for a date, a benchmark or Hero workout (Murph, Fran, Chad), how to coach or scale a movement, what cues fix a fault, warm-ups, or whether two days overlap in load and movement patterns.
---

# CAP and CrossFit coaching library CLI

`cap-pp-cli` reads CrossFit's programming and coaching material. Reading only.

Two sources sit behind it, which matters for auth:

| What | Source | Token |
|---|---|---|
| programming (days, weeks, tracks) | c3po content API | **required** |
| movements, benchmarks, resources | public CMS | **not needed** |

So the movement and benchmark commands keep working when the programming token
has expired.

## Programming

```bash
cap-pp-cli cap day 2026-08-10          # workout, stimulus, vectors, patterns
cap-pp-cli cap day 2026-08-10 --full   # + class plan timing, scaling, coaching goals
cap-pp-cli cap week 2026-08-10         # weekly overview
cap-pp-cli cap warmup 2026-08-10       # warm-up blocks with timing
cap-pp-cli cap compare 2026-08-10 2026-08-16
```

Dates: `YYYY-MM-DD` or `YYYYMMDD`; omitted means today. Any past date works, so
a benchmark published on a given day is reachable by that date.

### Other tracks

Beyond the daily affiliate programming, CAP publishes fixed-length preparation
programmes. They are sequences, addressed by day number:

```bash
cap-pp-cli cap tracks                   # what is available
cap-pp-cli cap day 3 --track murph      # Murph prep, day 3
cap-pp-cli cap day 1 --track chad
cap-pp-cli cap day 5 --track ring-muscle-up
```

Tracks: `affiliate` (dates), plus `murph`, `chad`, `pull-up`,
`chest-to-bar-pull-up`, `bar-muscle-up`, `ring-muscle-up`, `toes-to-bar`,
`handstand-push-up`, `double-under` (day numbers). Any other slug is passed
through, so a newly published track works without an update.

## Coaching library (no token)

```bash
cap-pp-cli cap movement air-squat       # faults, cues, progressions, substitutions
cap-pp-cli cap movements                # the catalogue, grouped by modality
cap-pp-cli cap movements squat          # filtered
cap-pp-cli cap benchmarks murph         # benchmark / Hero / Open workouts
cap-pp-cli cap resources warm-up        # warm-ups, progressions, coaching tips, scaling
cap-pp-cli cap resources --kids         # kids and teens plans
```

`cap movement` is the one to reach for when asked how to coach or fix a
movement: it returns each fault with the exact cues CrossFit publishes for it.

`cap benchmarks` searches names *and* workout text, so both `benchmarks murph`
and `benchmarks "wall walk"` work.

## Output

Every command emits the `{meta, results}` envelope; data is under `.results`.
`--agent` for agent-friendly output, `--select` to trim fields.

## Calling this from an agent

**Run one command per shell invocation, and never in the background.** Several
`cap movement` calls backgrounded in one shell finish in a nondeterministic
order, so their output interleaves and the answers line up against the wrong
questions. That failure looks exactly like the API serving the wrong movement:

```bash
# WRONG - output order is not the order you asked in
cap-pp-cli cap movement push-jerk & cap-pp-cli cap movement deadlift & wait

# RIGHT - one at a time, or label each result yourself
cap-pp-cli cap movement push-jerk --json
cap-pp-cli cap movement deadlift --json
```

**Read the identity back from the payload, not from the order of output.** Every
response names what it is: `.results.slug` for a movement, `.results.date` for a
day. When looking several things up, key the results by that field rather than
by position.

**Check the exit code.** 0 is a real answer; 3 means nothing is published for
that date or movement; 4 means the programming token expired (the library
commands still work). A nonzero code with output on stdout is not a partial
answer - treat it as no answer.

`cap movement` additionally verifies that the movement returned is the one
requested and fails loudly if it ever is not, so a mismatch cannot reach you
silently.

## What `compare` is for

Each day carries CAP's own `load`, `volume` and `skill` (1-5) plus the movement
patterns it trains (squat, hinge, vertical_push, horizontal_push, vertical_pull,
horizontal_pull, olympic, core, mono). `compare` scores how badly two days
double up if run on consecutive training days:

- >= 8: strong overlap, avoid back to back
- 4-7: some overlap, workable
- < 4: complementary

This is the input for resequencing a week around athletes who train Mon/Wed/Fri
or Tue/Thu: consecutive sessions *as each group experiences them* should not
repeat a pattern.

Patterns are read from CAP's scaling list **and** the prescription text, because
CAP only lists movements that need scaling - a 400-m run inside a workout can
appear nowhere in the scaling section.

## Auth

Programming needs an OAuth2 access token from the affiliate toolkit (short-lived
public-client token, scope `user:full:read`):

```bash
cap-pp-cli auth set-token <token>
```

It expires; when programming calls return exit 4, get a fresh one. **Never ask
the user for their CrossFit password** - this is a token flow, and the token
comes from an already-signed-in browser session.

## Exit codes

0 ok · 2 usage · 3 nothing published for that date/movement · 4 token missing or
expired · 5 API error · 7 rate limit · 10 config
