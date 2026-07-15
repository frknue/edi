# Presence — Telegram Bot + Shell Status — Design

Date: 2026-07-15
Status: Approved (Phase 3 of the engagement roadmap)

## Why

Phases 1–2 (gold economy, decay & stakes — shipped) made edi worth opening.
Phase 3 makes edi reach you when you don't: a two-way Telegram companion and
an `edi-cli status` block for new shells. Both are pure API clients — the
architecture's "every client goes through the same API" rule, proven again.

Roadmap context: Phase 4 = achievements/titles + AI weekly chronicle.

## Part 1: Telegram bot (`cmd/edi-telegram`)

A new standalone binary following the `edi-cli`/`edi-mcp` precedent: a thin
HTTP client via `internal/apiclient`, never touching the DB. It talks to
Telegram's Bot API with plain `net/http` — **no new Go dependencies**
(the Bot API is HTTPS + JSON).

**It requires zero new server endpoints.** Dashboard, ListQuests,
CompleteQuest, WardAttribute, and SetRestMode already cover every feature.

### internal/telegram package

Minimal Bot API client, isolated so the bot binary stays a thin command:

- `New(token string) *Client` (plus overridable `BaseURL` for tests)
- `GetUpdates(offset int64, timeoutSec int) ([]Update, error)` — long poll
- `SendMessage(chatID int64, html string) error` — parse_mode HTML
  (MarkdownV2 escaping is error-prone; HTML is not)
- `Update` carries update_id, chat id, and message text; everything else is
  ignored.

### Configuration (env)

| var                 | required | default | meaning                          |
|---------------------|----------|---------|----------------------------------|
| `TELEGRAM_BOT_TOKEN`| yes      | —       | from @BotFather                  |
| `TELEGRAM_CHAT_ID`  | no       | —       | the ONLY chat the bot serves     |
| `EDI_API`           | no       | `http://localhost:8080` | backend    |
| `EDI_TOKEN`         | no       | —       | bearer auth (matches server)     |
| `EDI_BRIEFING_TIME` | no       | `08:00` | local HH:MM, morning briefing    |
| `EDI_NUDGE_TIME`    | no       | `20:00` | local HH:MM, evening nudge       |

**Pairing mode:** when `TELEGRAM_CHAT_ID` is unset, the bot answers `/start`
from any chat with that chat's ID and setup instructions, and does nothing
else. With it set, messages from any other chat are silently ignored.

### Commands

| command          | behavior                                                        |
|------------------|-----------------------------------------------------------------|
| `/status`        | compact dashboard: level, streak, gold, quests open/done, decaying attributes, rest banner |
| `/quests`        | active quests with their REAL quest IDs (no session state)      |
| `/done <id>`     | complete quest → reply with XP, gold, level-ups, streak         |
| `/ward <attr>`   | buy a Maintenance Ward for the attribute                        |
| `/rest on\|off`  | toggle rest mode                                                |
| `/help`, `/start`| command list                                                    |

API errors (not enough gold, unknown quest/attribute, quest already
completed) relay as friendly plain messages — never stack traces. Unknown
commands get the help text. There are deliberately NO destructive commands
(no delete/archive).

### Scheduled pushes

- **Morning briefing** at `EDI_BRIEFING_TIME`: streak, gold, character
  level, today's active quests with IDs, decaying attributes with projected
  daily loss, rest banner when rest mode is on.
- **Evening nudge** at `EDI_NUDGE_TIME`: sent ONLY when
  `daily_progress.completed_today == 0` AND at least one active quest
  exists AND rest mode is off. Shows the easiest open quest (first by
  difficulty order trivial→easy→medium→hard→boss, ties by lowest total
  reward). Silent on productive days and during rest.
- Scheduler: compute the next local fire time for each push, sleep on a
  `time.Timer`, fire, reschedule for the next day. No persistence — a push
  missed while the bot was down is skipped, not replayed.
- Server unreachable at fire time: retry 3× with 30s spacing, then skip
  silently.

### Structure

- `cmd/edi-telegram/main.go` — env parsing, wiring, run loop.
- `cmd/edi-telegram/bot.go` — command dispatch + push logic, written as
  pure-ish functions over `models.Dashboard` / apiclient results so
  formatting and decisions are unit-testable without networks:
  - `formatBriefing(d models.Dashboard) string`
  - `formatStatus(d models.Dashboard) string`
  - `formatQuests(quests []models.Quest) string`
  - `nudgeQuest(d models.Dashboard) (*models.Quest, bool)` — nil,false = no nudge
  - `nextFire(now time.Time, hhmm string) time.Time`
  - `handleCommand(text string, api *apiclient.Client) (reply string)`

## Part 2: `edi-cli status` (shell presence)

New subcommand on the existing CLI:

- One `Dashboard()` fetch with a **hard 1-second timeout** (override the
  apiclient's HTTP client timeout for this command only).
- Prints a compact CRT-flavored block: character level, streak, gold, open
  quest count + completed today, decaying attributes (worst first), rest
  banner when on.
- **On ANY error (server down, timeout, bad response): print nothing, exit
  0.** This makes it safe in `.zshrc` — a dead server never breaks or slows
  a new shell beyond 1s.
- README documents the snippet: `edi-cli status` in `.zshrc`/`.bashrc`.

## Build & docs

- `Makefile`: `make build` also builds `bin/edi-telegram`.
- README: new "Presence" section — BotFather setup (3 steps), env vars,
  pairing mode, running `bin/edi-telegram`, the `.zshrc` snippet. Agent
  tool count unchanged (the bot uses typed endpoints, not agent tools).

## Security

- Single-chat lockdown via `TELEGRAM_CHAT_ID`; all other chats ignored.
- The bot holds `EDI_TOKEN` like any other client; it can complete quests
  and spend gold but cannot delete or archive anything.
- No inbound ports: long-polling only (fits self-hosted-behind-NAT).

## Testing

- Unit (offline, no network): `formatBriefing`/`formatStatus`/
  `formatQuests` (golden-ish assertions on key content), `nudgeQuest`
  (completed>0 → none; rest on → none; picks trivial over hard; no quests →
  none), `nextFire` (before/after the time today, midnight rollover),
  `handleCommand` parse table (all commands + unknown input).
- Integration (offline): `httptest` fakes for BOTH ends — a stub Telegram
  server and the REAL edi handlers over a seeded temp DB — drive
  `/quests` → `/done <id>` and assert the completion actually landed (XP
  event written) and the reply contains the reward.
- `edi-cli status`: unit-test the formatter; verify fail-silent behavior
  against a dead port (exit 0, empty output).
- Live validation: real BotFather token, real phone — briefing forced by
  setting `EDI_BRIEFING_TIME` a minute ahead, `/done` round-trip, pairing
  mode. (Requires the user's token; validated at the end with them.)

## Out of scope (later phases)

Decay alarms, level-up pushes, free-text LLM chat through the agent
registry (Phase 5 candidate), webhooks, multi-user/multi-chat, message
edit/reaction handling, systemd/launchd unit files.
