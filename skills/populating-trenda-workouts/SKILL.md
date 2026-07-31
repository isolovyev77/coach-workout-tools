---
name: populating-trenda-workouts
description: "Create or update client workouts in Trenda from coaching notes or BTWB. Trigger on requests to add, assign, plan, transfer, or edit a workout in Trenda. Resolve clients only from the current trainer's account, adapt a warm-up from that client's history, and verify every write."
---

# Trenda workout population

Use this skill to turn a coach's written or voice programming notes into a
well-structured Trenda workout, or to transfer one selected BTWB workout.

The package root is `COACH_TOOLS_HOME`. If it is unset, use the directory that
contains this skill's installed package. Commands below assume these paths:

- Trenda wrapper: `$COACH_TOOLS_HOME/apps/trenda/trenda`
- Trenda CLI: `trenda-pp-cli`
- BTWB CLI: `btwb-pp-cli`
- helper scripts: this skill's `scripts/` directory

## Safety rules

1. Each coach authenticates only into their own BTWB and Trenda accounts.
   Never request, display, copy, or store passwords, session cookies, API keys,
   client names, phone numbers, or exported client history in a prompt or file.
2. Resolve a client from the current Trenda account every time. Confirm the
   returned name and ID with the coach before the first write in a conversation.
   Never rely on an ID or name from a prior trainer's account.
3. Read the target calendar day before creating or editing a workout. If more
   than one workout exists and the coach did not identify one, stop and ask.
4. Before every write, show the date, client, and intended change. After every
   write, re-read the day and confirm that the requested content appears once.
5. Do not delete workouts, client records, exercises, or programs unless the
   coach explicitly asks for that exact deletion.

## Select the source

- Coach describes the session in words - use **Free-text to Trenda**.
- Coach asks to add one named exercise - use **Direct exercise addition**.
- Coach asks for a BTWB workout by date, track, or level - use **BTWB to Trenda**.

Do not guess a BTWB track, a date, a client, or a level. Ask one concise
clarifying question when one is missing.

## Standard workflow

1. Check authentication.

   ```bash
   "$COACH_TOOLS_HOME/apps/trenda/trenda-auth.mjs" status
   trenda-pp-cli doctor
   btwb-pp-cli auth status
   ```

2. Resolve the client in the current coach account and read the target date.

   ```bash
   trenda-pp-cli client list --agent
   "$COACH_TOOLS_HOME/apps/trenda/trenda" coach get-calendar \
     --client-id <client_id> --date-from YYYY-MM-DD --date-to YYYY-MM-DD --agent
   ```

3. Normalize the session into three sections: `warmup`, `workout`, and
   `cooldown`. Keep the top-level workout title empty unless the coach asks for
   a title. Preserve existing non-empty sections during an edit.

4. For named strength exercises, search the current Trenda library. Reuse an
   `exercise_id` only when movement, implement, and key modifier all match.
   A high fuzzy score by itself is not enough. For a missing exact match, create
   a manual exercise without `exercise_id`; never invent an exercise ID.

5. Keep an AMRAP, EMOM, interval, or for-time conditioning piece as one
   `MixedModal` item. Its composition goes in `complex_details`, and athlete
   instructions go in `description`. Do not split one conditioning WOD into
   unrelated library exercises or a superset.

6. Build a snake_case payload with `scripts/build_trenda_payload.py`, use the
   correct create or update command, then read the target date again.

## Warm-up policy

For a new or empty day, add a warm-up unless the coach opts out.

1. Read recent workouts for the selected client, expanding the search only as
   needed.
2. Prefer previous warm-ups from days with the same movement pattern and
   implement. If there is no exact match, use the closest safe analogue.
3. Keep the client's established terminology and scale volume to prepare rather
   than fatigue them: general temperature work, specific drill, short rehearsal.
4. Do not write source dates, source URLs, client history, or BTWB provenance
   into client-visible Trenda fields unless the coach explicitly requests it.

## Free-text to Trenda

1. Convert coaching notes into a plan JSON with the client ID, absolute date,
   type `Workout`, empty title, and all three base sections.
2. Search the exercise library and review manual fallbacks.
3. Build a create payload:

   ```bash
   python3 scripts/build_trenda_payload.py --input /tmp/plan.json --mode create \
     > /tmp/trenda-body.json
   "$COACH_TOOLS_HOME/apps/trenda/trenda" coach create-workout --stdin < /tmp/trenda-body.json
   ```

4. For an existing workout, preserve all unrelated blocks and use:

   ```bash
   python3 scripts/build_trenda_payload.py --input /tmp/plan.json --mode update-body \
     > /tmp/trenda-body.json
   "$COACH_TOOLS_HOME/apps/trenda/trenda" coach update-workout-body --stdin < /tmp/trenda-body.json
   ```

## Direct exercise addition

Use this for a request such as "add a split squat on Friday".

1. Resolve the client and absolute date. If the exercise itself is ambiguous,
   ask which movement to add.
2. Read the day. Append to the requested section, defaulting to `workout`; do
   not replace a warm-up, cooldown, or unrelated exercise.
3. Preserve only dosage the coach actually named: sets, reps, load, tempo,
   rest, and side. Do not invent missing dosage.
4. Search the Trenda library first. If no exact match exists, use a manual
   exercise and, only after checking it, add a short exact YouTube demo URL.
5. Write, re-read, and confirm the exercise appears once.

## BTWB to Trenda

1. Read only the relevant BTWB day or planned workout. Ask the coach to choose
   the track and variant when there are several candidates.
2. Keep the selected variant's movement, loading, format, rest, and athlete
   instructions faithful to the source. Do not place BTWB branding, a source URL,
   or the chosen level in the Trenda title or notes unless explicitly requested.
3. Put one conditioning session into one `MixedModal` block. Map ordinary skill
   work to exercises or a superset only when the source genuinely contains
   separately prescribed movements.
4. Build a movement-specific warm-up from the selected client's history, then
   present the exact planned write to the coach before saving.
5. Write and verify by re-reading the client day.

## Payload facts

- API request fields are `snake_case`.
- Creating a workout uses flat `warmup`, `workout`, and `cooldown` arrays.
- `update-workout` changes metadata only. Use `update-workout-body` for sections.
- A block has `is_superset`, `sort_order`, and `exercises`.
- Use an empty title for standard warm-up and cooldown text blocks so Trenda
  keeps its native labels.
- Do not send read-only `can_edit` or `metric` fields back to Trenda.

See `references/trenda-payloads.md` for the plan schema and
`references/matching-rules.md` for matching safeguards.
