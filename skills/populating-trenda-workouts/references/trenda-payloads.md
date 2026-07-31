# Trenda Payloads

This skill uses a normalized "plan" JSON as input to `scripts/build_trenda_payload.py`.

## Plan Schema

Top-level fields:

```json
{
  "mode": "create",
  "client_id": 12345,
  "date": "2026-08-05",
  "type": "Workout",
  "title": "",
  "description": "",
  "warmup": [],
  "workout": [],
  "cooldown": []
}
```

Do not include source provenance in client-visible fields. `BTWB`, BTWB workout
URLs, track names, and level names stay out of titles, descriptions, comments,
and coach notes unless the user explicitly requests attribution. Set
`allow_btwb_attribution: true` in a working plan only after such an instruction;
the flag is validation-only and is never sent to Trenda.

For `mode = "update-body"`:

- omit `client_id`
- omit `date`
- include `workout_id`

> Идентификаторы упражнений в примерах ниже (`882` = «Фронтальные приседания /
> Front Squats») приведены как образец. Брать их нужно только из свежей выгрузки
> библиотеки: сервер принимает любой целый `exercise_id` без ошибки, поэтому
> скопированный из шаблона идентификатор молча подставит клиенту чужое
> упражнение.

## Supported Section Items

### 1. Text block

```json
{
  "kind": "text",
  "title": "",
  "lines": ["Bike 3 min", "10 scap pull-ups", "10 air squats"]
}
```

Result:

- one block
- one exercise
- `type = Warmup` or `Cooldown` depending on section
- empty `title`, allowing Trenda to keep the native `Разминка` or `Заминка` heading
- lines joined into HTML in `description`

### 2. Library-backed exercise

```json
{
  "kind": "exercise",
  "title": "Front Squat",
  "exercise_id": 882,
  "description_lines": ["Controlled eccentric"],
  "sets": [
    { "reps": 5, "kg": 60 },
    { "reps": 5, "kg": 70 },
    { "reps": 5, "kg": 75 }
  ]
}
```

### 3. Manual exercise

```json
{
  "kind": "exercise",
  "title": "Cyclist Squat",
  "description_lines": ["Heels elevated", "Close stance"],
  "video_urls": ["https://www.youtube.com/watch?v=..."],
  "sets": []
}
```

### 4. Mixed-modal piece

```json
{
  "kind": "mixed_modal",
  "title": "AMRAP 12 mins: Double Unders and Toes-to-bars",
  "lines": [
    "Complete as many rounds as possible in 12 mins of:",
    "18 Double Unders",
    "9 Toes-to-bars",
    "*every 2mins (starting at 0:00) complete 12 box jumps 60cm"
  ],
  "instructions_lines": [
    "Каждые 2 минуты (начиная с 0:00) выполните:",
    "12 прыжков на коробку, 60 см",
    "Продолжайте с того места, где остановились перед каждым набором прыжков на коробку."
  ]
}
```

Result:

- `type = MixedModal`
- the exact BTWB title stays in `title`
- `complex_details` stores the composition HTML
- `description` stores Athlete Instructions HTML
- no `exercise_id`
- no `sets` unless the coach explicitly requested a load/result
- no video unless one movement is unusually difficult or unfamiliar

Never split this item into separate Double Under, Toes-to-Bar, and Box Jump
exercises. Complex integrity takes priority over reusing library movement ids.

### 5. Superset

```json
{
  "kind": "superset",
  "exercises": [
    {
      "kind": "exercise",
      "title": "DB Bench Press",
      "exercise_id": 882,
      "sets": [{ "reps": 10, "kg": 20 }]
    },
    {
      "kind": "exercise",
      "title": "Chest Supported Row",
      "sets": [{ "reps": 10 }]
    }
  ]
}
```

Result:

- one block
- `is_superset = true`

### 6. Single Skill Work interval

For a single timed drill with one overall cadence, use one standalone workout
block. This matches the accepted Trenda presentation for today's crossover work:

```json
{
  "kind": "mixed_modal",
  "title": "EMOM 8 mins: Crossover Practices, 30sec",
  "lines": [
    "в начале каждой минуты 30 секунд практики кроссоверов со скакалкой"
  ],
  "sets": [
    { "circles": 0 }
  ]
}
```

The zero-round set is allowed here only to preserve the accepted Trenda UI
shape for this standalone Skill Work interval. Do not generalize it to
conditioning WODs, and do not add a duplicate copy to `warmup`.

### 7. Alternating Skill Work

Represent alternating movements as one linked superset. Repeat the set template
for every prescribed round and keep rest instructions on the relevant exercise:

```json
{
  "kind": "superset",
  "exercises": [
    {
      "kind": "exercise",
      "title": "Scapular Pull-Up",
      "exercise_id": 882,
      "sets": [
        { "reps": 8 },
        { "reps": 8 },
        { "reps": 8 }
      ],
      "description_lines": ["отдых до конца 2:00"]
    },
    {
      "kind": "exercise",
      "title": "Kipping Swing",
      "exercise_id": 882,
      "sets": [
        { "reps": 10 },
        { "reps": 10 },
        { "reps": 10 }
      ],
      "description_lines": ["отдых 0:30"]
    }
  ]
}
```

Result:

- one linked block with `is_superset = true`
- both exercises contain three sets for three rounds
- Trenda renders them as A1 and A2 from their order
- titles do not contain manually written `A1` or `A2`
- each rest instruction stays with the movement it follows

## Supported Set Shortcuts

The builder script converts these keys into Trenda units:

- `reps` -> `Reps`
- `kg` -> `Kilograms`
- `lb` -> `Pounds`
- `meters` -> `Meters`
- `km` -> `Kilometers`
- `cal` -> `Calories`
- `watts` -> `Watts`
- `seconds` -> `Seconds`
- `minutes` -> `Minutes`
- `hours` -> `Hours`
- `circles` -> `Circles`
- `quantity` -> `Quantity`

The first present key becomes `primary_unit`, the second becomes `secondary_unit`,
and the third becomes `tertiary_unit`.

## Create Output

The script emits a `create-workout` body:

```json
{
  "client_id": 12345,
  "date": "2026-08-05",
  "type": "Workout",
  "title": "",
  "description": "",
  "warmup": [...],
  "workout": [...],
  "cooldown": [...]
}
```

## Update-body Output

The script emits an `update-workout-body` body:

```json
{
  "id": 123456,
  "title": "",
  "description": "",
  "warmup": [...],
  "workout": [...],
  "cooldown": [...]
}
```

The builder always preserves `warmup`, `workout`, and `cooldown`, including
empty arrays. This prevents an omitted section from becoming an accidental
delete during body replacement.
