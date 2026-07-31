# Btwb CLI

Beyond the Whiteboard (btwb.com) - CrossFit workout tracking.

Two channels:

1. **Whiteboard (member session)** - the athlete's own whiteboard: planned
   workouts (WODs) per gym track, logged results, subscribed tracks.
   Authenticated with a session cookie obtained by a form login at
   `POST /session`. The upstream serves HTML; this client extracts the
   structured workout data and emits the JSON documented here.

2. **Web Widgets (gym key)** - btwb's public widget API at
   `webwidgets.prod.btwb.com`, a real JSON API returning WODs, gym
   activity and workout leaderboards for one or more tracks. Requires the
   gym's Web Widgets key (gym admin: gym menu -> Website Integration).

## Install

The recommended path installs both the `btwb-pp-cli` binary and the `pp-btwb` agent skill in one shot:

```bash
npx -y @mvanhorn/printing-press install btwb
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press install btwb --cli-only
```


### Without Node

The generated install path is category-agnostic until this CLI is published. If `npx` is not available before publish, install Node or use the category-specific Go fallback from the public-library entry after publish.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/btwb-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-btwb --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-btwb --force
```

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

```
Install the pp-btwb skill from https://github.com/mvanhorn/printing-press-library/tree/main/cli-skills/pp-btwb. The skill defines how its required CLI can be installed.
```

## Quick Start

### 1. Install

See [Install](#install) above.

### 2. Set Up Credentials

Get your API key from your API provider's developer portal. The key typically looks like a long alphanumeric string.

```bash
export BTWB_SESSION_COOKIE="<paste-your-key>"
```

You can also persist this in your config file at `~/.config/btwb-pp-cli/config.toml`.

### 3. Verify Setup

```bash
btwb-pp-cli doctor
```

This checks your configuration and credentials.

### 4. Try Your First Command

```bash
btwb-pp-cli tasks mock-value mock-value
```

## Usage

Run `btwb-pp-cli --help` for the full command reference and flag list.

## Commands

### members

Manage members


### tasks

Manage tasks

- **`btwb-pp-cli tasks get-event`** - The complete workout: its description as written by the coach, the
scoring variant, how many members already logged a result, and the
member's own previous result for the same workout.

### webwidgets

Manage webwidgets

- **`btwb-pp-cli webwidgets widget-activities`** - Get the gym's recent activity feed via the gym's widget key
- **`btwb-pp-cli webwidgets widget-leaderboard`** - Get a workout's leaderboard via the gym's widget key
- **`btwb-pp-cli webwidgets widget-wods`** - The gym-facing JSON API. Returns a wodset per track per day, each with
the workout description and optionally the leaderboard and recent
results. Requires the gym's Web Widgets key, not a member session.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
btwb-pp-cli tasks mock-value mock-value

# JSON for scripting and agents
btwb-pp-cli tasks mock-value mock-value --json

# Filter to specific fields
btwb-pp-cli tasks mock-value mock-value --json --select id,name,status

# Dry run — show the request without sending
btwb-pp-cli tasks mock-value mock-value --dry-run

# Agent mode — JSON + compact + no prompts in one flag
btwb-pp-cli tasks mock-value mock-value --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Use with Claude Code

Install the focused skill — it auto-installs the CLI on first invocation:

```bash
npx skills add mvanhorn/printing-press-library/cli-skills/pp-btwb -g
```

Then invoke `/pp-btwb <query>` in Claude Code. The skill is the most efficient path — Claude Code drives the CLI directly without an MCP server in the middle.

<details>
<summary>Use as an MCP server in Claude Code (advanced)</summary>

If you'd rather register this CLI as an MCP server in Claude Code, install the MCP binary first:


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Then register it:

```bash
claude mcp add btwb btwb-pp-mcp -e BTWB_SESSION_COOKIE=<your-key>
```

</details>

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/btwb-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `BTWB_SESSION_COOKIE` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "btwb": {
      "command": "btwb-pp-mcp",
      "env": {
        "BTWB_SESSION_COOKIE": "<your-key>"
      }
    }
  }
}
```

</details>

## Health Check

```bash
btwb-pp-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/btwb-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `BTWB_SESSION_COOKIE` | harvested | Yes | Populated automatically by auth login. |

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `btwb-pp-cli doctor` to check credentials
- Verify the environment variable is set: `echo $BTWB_SESSION_COOKIE`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

---

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
