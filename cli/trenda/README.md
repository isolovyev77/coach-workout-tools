# Trenda CLI

Внутренний API веб-приложения Trenda Coach (app.trenda.coach) - платформы для
фитнес-тренеров: клиенты, календарь тренировок, конструктор тренировок,
библиотека упражнений, программы и результаты.

Официальной публичной документации у API нет: описание восстановлено по
клиентскому коду приложения и покрывает подмножество, нужное для работы с
календарём и наполнения тренировок. Все вызовы - POST с JSON-телом.
Авторизация - сессионные cookie, выдаваемые при входе коуча.

Learn more at [Trenda](https://app.trenda.coach).

## Install

The recommended path installs both the `trenda-pp-cli` binary and the `pp-trenda` agent skill in one shot:

```bash
npx -y @mvanhorn/printing-press install trenda
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press install trenda --cli-only
```


### Without Node

The generated install path is category-agnostic until this CLI is published. If `npx` is not available before publish, install Node or use the category-specific Go fallback from the public-library entry after publish.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/trenda-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-trenda --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-trenda --force
```

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

```
Install the pp-trenda skill from https://github.com/mvanhorn/printing-press-library/tree/main/cli-skills/pp-trenda. The skill defines how its required CLI can be installed.
```

## Quick Start

### 1. Install

See [Install](#install) above.

### 2. Set Up Credentials

Get your API key from your API provider's developer portal. The key typically looks like a long alphanumeric string.

```bash
export TRENDA_SESSION="<paste-your-key>"
```

You can also persist this in your config file at `~/.config/trenda-pp-cli/config.toml`.

### 3. Verify Setup

```bash
trenda-pp-cli doctor
```

This checks your configuration and credentials.

### 4. Try Your First Command

```bash
trenda-pp-cli client list
```

## Usage

Run `trenda-pp-cli --help` for the full command reference and flag list.

## Commands

### client

Клиенты коуча и заметки по ним

- **`trenda-pp-cli client get`** - Карточка клиента
- **`trenda-pp-cli client list`** - Клиенты текущего коуча. Идентификатор клиента из этого списка нужен для работы с календарём.

### coach

Профиль текущего коуча

- **`trenda-pp-cli coach copy-workout`** - Копирует тренировку на другую дату или другому клиенту. Дешевле, чем собирать заново.
- **`trenda-pp-cli coach create-workout`** - Добавляет день в календарь клиента. Содержимое передаётся тремя массивами
блоков: warmup (разминка), workout (основная часть), cooldown (заминка).
У новых блоков и упражнений идентификатор не указывается, сервер выдаёт его сам.
- **`trenda-pp-cli coach delete-workout`** - Удалить тренировку
- **`trenda-pp-cli coach get-calendar`** - Содержимое календаря клиента: тренировки, дни отдыха и восстановления с их
идентификаторами. Вызывать перед созданием тренировки, чтобы не задублировать день.
- **`trenda-pp-cli coach get-current`** - Возвращает профиль коуча, которому принадлежит сессия. Дешёвая проверка, что сессия жива.
- **`trenda-pp-cli coach get-exercise-history`** - С какими весами, повторениями и временем клиент выполнял упражнения ранее,
от свежих записей к старым. Нужна, чтобы назначать адекватную нагрузку.
Клиент здесь задаётся полем user_id: поле client_id молча игнорируется.
Без exercise_id возвращается история по всем упражнениям клиента, а она
может быть большой: у активного клиента это сотни записей за один ответ.
- **`trenda-pp-cli coach get-exercises-by-ids`** - Возвращает упражнения из библиотеки по списку идентификаторов.
Неизвестные идентификаторы молча отбрасываются. Осторожно: с пустым
телом эндпоинт отдаёт всю библиотеку целиком, это больше тысячи записей.
- **`trenda-pp-cli coach get-last-result`** - Клиент задаётся полем user_id. Валидации нет: при отсутствии данных
приходит 200 и result равный null, а не ошибка.
- **`trenda-pp-cli coach get-program`** - Несмотря на название маршрута, это фильтр-список: принимает массив ids и
всегда возвращает массив. Скалярное поле id не распознаётся, и запрос без
фильтра отдаёт все программы коуча.
- **`trenda-pp-cli coach list-exercises`** - Упражнения, доступные коучу, с их идентификаторами. Идентификатор упражнения
обязателен при сборке тренировки.
- **`trenda-pp-cli coach list-metric-groups`** - Наборы измеряемых показателей, из которых собираются подходы упражнения.
- **`trenda-pp-cli coach list-program-workouts`** - Шаблонные тренировки внутри программы, пригодные для копирования в календарь.
- **`trenda-pp-cli coach list-programs`** - Программы, доступные коучу, включая чужие публичные. Поддерживает
пагинацию через limit и offset.
- **`trenda-pp-cli coach update-workout`** - Правит свойства дня: дату, тип, название, описание, порядок среди тренировок
одного дня. Содержимое тренировки этим вызовом не меняется.
- **`trenda-pp-cli coach update-workout-body`** - Перезаписывает разминку, основную часть и заминку существующей тренировки.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
trenda-pp-cli client list

# JSON for scripting and agents
trenda-pp-cli client list --json

# Filter to specific fields
trenda-pp-cli client list --json --select id,name,status

# Dry run — show the request without sending
trenda-pp-cli client list --dry-run

# Agent mode — JSON + compact + no prompts in one flag
trenda-pp-cli client list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Use with Claude Code

Install the focused skill — it auto-installs the CLI on first invocation:

```bash
npx skills add mvanhorn/printing-press-library/cli-skills/pp-trenda -g
```

Then invoke `/pp-trenda <query>` in Claude Code. The skill is the most efficient path — Claude Code drives the CLI directly without an MCP server in the middle.

<details>
<summary>Use as an MCP server in Claude Code (advanced)</summary>

If you'd rather register this CLI as an MCP server in Claude Code, install the MCP binary first:


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Then register it:

```bash
claude mcp add trenda trenda-pp-mcp -e TRENDA_SESSION=<your-key>
```

</details>

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/trenda-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `TRENDA_SESSION` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "trenda": {
      "command": "trenda-pp-mcp",
      "env": {
        "TRENDA_SESSION": "<your-key>"
      }
    }
  }
}
```

</details>

## Health Check

```bash
trenda-pp-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/trenda-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `TRENDA_SESSION` | per_call | Yes | Set to your API credential. |

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `trenda-pp-cli doctor` to check credentials
- Verify the environment variable is set: `echo $TRENDA_SESSION`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

## HTTP Transport

This CLI uses Chrome-compatible HTTP transport for browser-facing endpoints. It does not require a resident browser process for normal API calls.

---

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
