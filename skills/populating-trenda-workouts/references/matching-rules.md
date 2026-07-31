# Matching Rules

Use these rules when deciding whether a BTWB or free-text exercise can reuse a
Trenda library exercise.

## Accept Only When The Core Identity Matches

Core identity consists of:

- movement pattern
- implement or apparatus
- key modifier

All three must survive the mapping.

## Key Modifiers That Change Identity

Treat these as identity-changing, not cosmetic:

- front / back / overhead
- power / squat / split
- hang / floor / block
- strict / kipping / butterfly
- chest-to-bar / pull-up / chin-up / ring muscle-up / bar muscle-up
- dumbbell / kettlebell / barbell / cable / machine / bodyweight
- row erg / ski erg / bike erg / assault bike
- deficit / paused / tempo / single-arm / single-leg / alternating

If the source includes one of these and the Trenda candidate does not, reject the candidate unless the user explicitly says the substitution is acceptable.

## Good Matches

- `Front Squat` -> `Barbell Front Squat`
- `DB Romanian Deadlift` -> `Dumbbell Romanian Deadlift`
- `Chest to Bar Pull-Up` -> `Chest To Bar Pull Up`
- `Bike Erg` -> `Bike Erg`

## Bad Matches

- `Back Squat` -> `Front Squat`
- `Power Snatch` -> `Snatch`
- `Kipping Handstand Push-Up` -> `Strict Handstand Push-Up`
- `Assault Bike` -> `Echo Bike` only if the coach explicitly treats them as equivalent
- `Toes-to-Bar` -> `Hanging Knee Raise`

## BTWB Shorthand

Common shorthands that should be expanded before matching:

- `DU` -> `double under`
- `SU` -> `single under`
- `T2B` or `TTB` -> `toes to bar`
- `C2B` -> `chest to bar pull up`
- `HSPU` -> `handstand push up`
- `BS` -> usually `back squat`
- `FS` -> usually `front squat`
- `PC` -> usually `power clean`
- `PP` -> usually `push press`

Do not trust a shorthand if the surrounding text suggests a different movement.

## When To Go Manual

Choose a manual exercise entry when:

- the best Trenda match changes the implement
- the best Trenda match changes the main movement
- the source exercise is unusually specific and Trenda only has a broader parent movement
- the source is a custom complex or drill
- the movement exists, but only under the wrong modality

For manual entries:

- keep the source title
- use HTML in `description` or `complex_details` for coaching details
- add `video_urls` with a short YouTube demo link when available
