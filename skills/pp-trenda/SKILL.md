---
name: pp-trenda
description: "Printing Press CLI for Trenda. Внутренний API веб-приложения Trenda Coach (app.trenda.coach) - платформы для..."
license: "Apache-2.0"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - trenda-pp-cli
---

# Trenda — Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `trenda-pp-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer:
   ```bash
   npx -y @mvanhorn/printing-press install trenda --cli-only
   ```
2. Verify: `trenda-pp-cli --version`
3. Ensure `$GOPATH/bin` (or `$HOME/go/bin`) is on `$PATH`.

If the `npx` install fails before this CLI has a public-library category, install Node or use the category-specific Go fallback after publish.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

Внутренний API веб-приложения Trenda Coach (app.trenda.coach) - платформы для
фитнес-тренеров: клиенты, календарь тренировок, конструктор тренировок,
библиотека упражнений, программы и результаты.

Официальной публичной документации у API нет: описание восстановлено по
клиентскому коду приложения и покрывает подмножество, нужное для работы с
календарём и наполнения тренировок. Все вызовы - POST с JSON-телом.
Авторизация - сессионные cookie, выдаваемые при входе коуча.

## HTTP Transport

This CLI uses Chrome-compatible HTTP transport for browser-facing endpoints. It does not require a resident browser process for normal API calls.

## Command Reference

**client** — Клиенты коуча и заметки по ним

- `trenda-pp-cli client get` — Карточка клиента
- `trenda-pp-cli client list` — Клиенты текущего коуча. Идентификатор клиента из этого списка...

**coach** — Профиль текущего коуча

- `trenda-pp-cli coach copy-workout` — Копирует тренировку на другую дату или другому клиенту....
- `trenda-pp-cli coach create-workout` — Добавляет день в календарь клиента. Содержимое передаётся...
- `trenda-pp-cli coach delete-workout` — Удалить тренировку
- `trenda-pp-cli coach get-calendar` — Содержимое календаря клиента: тренировки, дни отдыха и...
- `trenda-pp-cli coach get-current` — Возвращает профиль коуча, которому принадлежит сессия. Дешёвая...
- `trenda-pp-cli coach get-exercise-history` — С какими весами, повторениями и временем клиент выполнял...
- `trenda-pp-cli coach get-exercises-by-ids` — Возвращает упражнения из библиотеки по списку...
- `trenda-pp-cli coach get-last-result` — Клиент задаётся полем user_id. Валидации нет: при отсутствии...
- `trenda-pp-cli coach get-program` — Несмотря на название маршрута, это фильтр-список: принимает...
- `trenda-pp-cli coach list-exercises` — Упражнения, доступные коучу, с их идентификаторами....
- `trenda-pp-cli coach list-metric-groups` — Наборы измеряемых показателей, из которых собираются подходы...
- `trenda-pp-cli coach list-program-workouts` — Шаблонные тренировки внутри программы, пригодные для...
- `trenda-pp-cli coach list-programs` — Программы, доступные коучу, включая чужие публичные....
- `trenda-pp-cli coach update-workout` — Правит свойства дня: дату, тип, название, описание, порядок...
- `trenda-pp-cli coach update-workout-body` — Перезаписывает разминку, основную часть и заминку...


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
trenda-pp-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match — fall back to `--help` or use a narrower query.

## Auth Setup

Sign in once with the coach's own Trenda password:

```bash
trenda-pp-cli auth login
```

The password is read without echo and never stored. What is stored is the
session, in `~/.config/pp-trenda/credentials.json` at mode 0600, together with
the refresh token. Every command reads that store directly - no wrapper, no
helper on `$PATH`, no environment variable to export. When the session expires
mid-command the CLI refreshes it once and retries, so a long-lived agent does
not need to re-authenticate between runs.

- `trenda-pp-cli auth status` — who is signed in, without touching the password
- `trenda-pp-cli auth logout` — clears the stored session
- `trenda-pp-cli doctor` — checks the whole setup

`TRENDA_SESSION` still wins over the stored session when it is set, which is how
a CI job or a one-off shell can run under a different account. Leave it unset
otherwise: a stale value in the environment shadows a perfectly good login and
shows up as exit code 4.

Never print, log, or commit the password, the cookies, or the tokens - not into
command output, not into a scratch file, not into a commit.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable** — JSON on stdout, errors on stderr
- **Filterable** — `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  trenda-pp-cli client list --agent --select id,name,status
  ```
- **Previewable** — `--dry-run` shows the request without sending
- **Offline-friendly** — sync/search commands can use the local SQLite store when available
- **Non-interactive** — never prompts, every input is a flag
- **Explicit retries** — use `--idempotent` only when an already-existing create should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal — piped/agent consumers get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
trenda-pp-cli feedback "the --since flag is inclusive but docs say exclusive"
trenda-pp-cli feedback --stdin < notes.txt
trenda-pp-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.trenda-pp-cli/feedback.jsonl`. They are never POSTed unless `TRENDA_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `TRENDA_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

```
trenda-pp-cli profile save briefing --json
trenda-pp-cli --profile briefing client list
trenda-pp-cli profile list --json
trenda-pp-cli profile show briefing
trenda-pp-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `trenda-pp-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

Install the MCP binary from this CLI's published public-library entry or pre-built release, then register it:

```bash
claude mcp add trenda-pp-mcp -- trenda-pp-mcp
```

Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which trenda-pp-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   trenda-pp-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `trenda-pp-cli <command> --help`.

## CAP Source Routing

When the user asks to transfer CrossFit Affiliate Programming into a client's
calendar, use `pp-cap` to fetch the exact CAP date, preparation-track day, and
level, then use `populating-trenda-workouts` for semantic exercise matching and
the Trenda write. Do not invent a raw payload from CAP output. A multimovement
conditioning WOD remains one `MixedModal`; standalone strength and skill work
may reuse exact Trenda library exercises. CAP's public movement library can
supply verified cues, progressions, and a `youtube_id` for a manual exercise.
Keep CAP labels and source URLs out of client-visible Trenda fields unless the
user explicitly requests attribution.
