---
name: pp-btwb
description: "Read and plan CrossFit workouts on btwb.com (Beyond the Whiteboard) - today's WOD, a given date, a training block, one workout in full, or planning a workout onto a track from plain text. Use when asked about the gym's programming, the workout of the day, what's on the whiteboard, a member's logged results, or to schedule a workout."
license: "Apache-2.0"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - btwb-pp-cli
---

# btwb — Printing Press CLI

Reads the workouts programmed on a btwb whiteboard and returns them as JSON.

## Prerequisites

This skill drives the `btwb-pp-cli` binary. Verify it before running anything
from this skill:

```bash
btwb-pp-cli --version
```

If it is missing, build it from the local project (needs Go 1.26 or newer):

```bash
cd ~/Projects/pp-btwb/btwb-cli && make install
```

Make sure `$(go env GOPATH)/bin` is on `$PATH`. This CLI is not published to the
Printing Press public library, so `npx @mvanhorn/printing-press install` will
not find it.

## Auth

btwb has no API token. The CLI signs in with the member's email and password,
the same way a browser does, and keeps only the resulting session cookie in
`~/.config/btwb-pp-cli/config.toml` (mode 0600).

**Never run `auth login` on the user's behalf, and never ask for their password
in chat.** When a command exits 4, tell the user to run this themselves in their
own terminal:

```bash
btwb-pp-cli auth login
```

Check state without touching credentials: `btwb-pp-cli auth status`.

## Reading workouts

Start here. `wod` resolves the member from the stored session, so no id is
needed.

- `btwb-pp-cli wod today` — every entry on today's whiteboard
- `btwb-pp-cli wod day --date 2026-07-30` — one date
- `btwb-pp-cli wod week --date 2026-07-30` — the fortnight btwb renders around
  that date (btwb's grid is two weeks, not seven days)
- `btwb-pp-cli wod event <event-id>` — one workout in full

Flags on the day-shaped commands:

| Flag | Effect |
|---|---|
| `--track "<text>"` | only tracks whose name contains this text, case-insensitive |
| `--track-id <id>` | only this track id |
| `--details` | inline each planned workout's full text as written by the coach |
| `--planned-only` | drop the member's own logged results |
| `--member-id <id>` | read a different member's whiteboard |

**`--details` costs one request per workout.** A whiteboard day commonly carries
20+ entries across all tracks, so pair it with `--track` or `--planned-only`.
Without a filter, prefer two steps: list the day without `--details`, then
`wod event <id>` for the workout that matters.

**Any date reads, past or future.** btwb is not limited to the current
fortnight and neither is this CLI. When `--date` answers
`date <D> not present in page; page covers <A>..<B>`, btwb genuinely has nothing
programmed for that date on the tracks the member follows - the message names
the window it did return so the gap is visible. Do not report this as a CLI
limitation: reaching past and future dates was broken up to v0.1.7 and produced
exactly this message, and the fix shipped in v0.1.8. Two things to check before
believing an empty answer: the binary is v0.1.8 or newer (`btwb-pp-cli
--version`), and the answer is not a cached pre-fix one (`--no-cache`).

Supporting commands:

- `btwb-pp-cli members tracks list <memberId>` — track ids and names
- `btwb-pp-cli members workout-sessions list-sessions <memberId>` — logged results

## What comes back

Every command wraps its payload in a provenance envelope. **Read the data from
`.results`**:

```json
{
  "meta": { "source": "live" },
  "results": {
    "date": "2026-07-30",
    "member_id": 100200,
    "workouts": [
      {
        "kind": "planned",
        "title": "Rx'd AMRAP 12 mins: Double Unders and Toes-to-bars",
        "track_id": 694243,
        "track_name": "CAP",
        "event_id": 325508983,
        "detail_path": "/tasks/members/100200/track_events/325508983"
      }
    ]
  }
}
```

`kind` is `planned` for what the coach programmed and `logged` for a result the
member recorded. `event_id` is what `wod event` takes. With `--details`, each
planned entry also carries a `detail` object holding `name`, `description` (the
workout as written, newlines preserved), `variant`, `results_count` and
`movements`.

A track name ending in `*` is one btwb marks as unavailable to this member.

Note that a whiteboard day mixes several tracks - a gym's own programming, a
licensed program like CAP, coaches' notes, the member's personal track. Entries
titled "Daily Brief", "Warm-up", "Cool Down" or "Overview Video" are coach notes,
not workouts. Say which track a workout came from rather than presenting one
day's entries as a single session.

## Agent mode

Add `--agent` to any command: JSON on stdout, no prompts, no colour. Trim the
payload with `--select` — a full day with details is large:

```bash
btwb-pp-cli wod today --agent --select 'workouts.title,workouts.track_name'
btwb-pp-cli wod today --track CAP --details --agent \
  --select 'workouts.title,workouts.detail.description'
```

Paths in `--select` are relative to the payload, not to `.results`.

Other useful flags: `--dry-run` shows the request without sending it,
`--no-cache` bypasses the 5-minute response cache, `--deliver file:<path>`
writes the output to a file.

## Gym-wide data (needs a gym key)

`webwidgets widget-wods`, `widget-activities` and `widget-leaderboard` read
btwb's Web Widgets API, which is authenticated per gym rather than per member. A
gym admin finds the key under the gym menu -> Website Integration:

```bash
btwb-pp-cli auth set-widget-key <key>
btwb-pp-cli webwidgets widget-wods --track-ids 530228 --days 7 --agent
```

Without the key these exit 5 saying so. Everything under `wod` works without it,
so a missing widget key is not a blocker — use `wod`.

These three commands have been built against the API's documented shape but not
exercised against it live. If one behaves oddly, report that plainly instead of
explaining the output away.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 2 | Usage error |
| 3 | Not found |
| 4 | Not signed in, or the session expired — the user must run `auth login` |
| 5 | Upstream error, or a missing credential |
| 7 | Rate limited |
| 10 | Config error |

## Argument parsing

Parse `$ARGUMENTS`:

1. Empty, `help`, or `--help` → show `btwb-pp-cli --help`
2. A date, or wording about today, a day, a week, or the workout of the day →
   the matching `wod` command with `--agent`
3. Anything else → match it to a command above and run it with `--agent`; drill
   into `btwb-pp-cli <command> --help` when the arguments are unclear

`btwb-pp-cli which "<capability in your own words>"` resolves a plain-language
capability to a command. Exit 0 means it matched; exit 2 means it did not.

## Planning workouts (writes)

`wod plan` schedules a workout onto a track. The workout is given as plain text,
the way a coach writes it; btwb parses it into movements server-side:

```bash
btwb-pp-cli wod plan --date 2026-08-06 --workout "3 rounds for time:
Row, 500 m
21 Box Jumps, 24/20 in
12 Burpees" --dry-run
```

Rules for agents, in order:

1. **Always run `--dry-run` first** and show the user the movements btwb parsed.
   btwb's parser is good but not perfect; the preview is the only place a
   mistranslation is caught before the whole track sees it.
2. **Only pass `--yes` after the user has approved that exact preview.** Without
   `--yes` the command refuses to write in agent mode; that refusal is the
   design, not a bug to work around.
3. Which tracks are writable is btwb's decision: `btwb-pp-cli wod tracks` lists
   them. Without gym admin rights that is the personal track only. When a gym
   track is asked for and not offered, exit 4 explains it - relay that message,
   do not retry.
4. `wod unplan <event-id>` removes a planned workout. It is deliberately not
   exposed over MCP; when a planned workout must be removed, give the user the
   command to run.

`--track "<name>"` / `--track-id <id>` choose the track (with one track,
optional), `--title` sets a custom title, `--workout-file wod.txt` (or `-` for
stdin) reads the text from a file.

Logging RESULTS is a different thing and stays read-only: this CLI does not
post scores. When asked to record a result, give the user the `log_path` from
the workout's detail — it is the page on btwb that records it.

### CAP as a planning source

When the requested workout comes from CrossFit Affiliate Programming, use the
`pp-cap` skill and `cap-pp-cli` to read and verify the exact CAP date, track day,
and requested level before building BTWB plain text. Keep one conditioning
complex intact. Then follow the normal `wod plan` guardrail: inspect writable
tracks, run `--dry-run`, show the parsed preview, and pass `--yes` only after the
user approves that exact preview. Do not add `CAP`, a CAP URL, or other source
attribution to the BTWB title or workout unless the user explicitly requests it.

## Local data commands

`sync`, `search`, `analytics`, `stale`, `orphans` and `load` come from the
generator and cache into SQLite. They are not part of the tested path here;
prefer `wod` for anything a user asks about workouts.

## MCP server

Every command is also available as an MCP tool:

```bash
cd ~/Projects/pp-btwb/btwb-cli && make install-mcp
claude mcp add btwb-pp-mcp -- btwb-pp-mcp
```
