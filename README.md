# edi — a self-hosted Life RPG

Turn real life into an RPG. Complete real actions (workouts, deep work, learning,
journaling, social, health, projects) → earn XP → level up nine life **attributes**,
build **streaks**, and get **agent suggestions** for what to do next.

Built as a clean, AI-agent-ready architecture: the **web UI, a future CLI, mobile,
and an AI agent all use the same documented backend API**. The agent has **no hidden
data layer** — it calls the exact same service functions, exposed as discoverable tools.

```
┌─────────┐   ┌─────────┐   ┌──────────┐   ┌──────────────┐
│  Web UI │   │   CLI   │   │  Mobile  │   │   AI Agent   │
└────┬────┘   └────┬────┘   └────┬─────┘   └──────┬───────┘
     └─────────────┴──────┬──────┴────────────────┘
                          ▼
                 REST API  /  Agent tool registry   (same endpoints)
                          ▼
                services.Service   ← single source of business logic
                          ▼
                 db.Store (SQLite + migrations, auditable XP events)
```

---

## Quick start

**Requirements:** Go ≥ 1.22, Node ≥ 18, npm. (Tested on Go 1.24, Node 24.)

```bash
# 1. install dependencies
make install

# 2. run backend (:8080) + frontend (:5173) together
make dev
```

Open **http://localhost:5173**. The database (`server/edi.db`) is created and
seeded with demo data automatically on first run.

### One self-hosted binary (production-style)

```bash
make prod        # builds the web client + a single Go binary, serves both on :8080
```

Open **http://localhost:8080**. The Go binary embeds its migrations and serves the
built SPA — copy `bin/edi` + `client/dist` to a server and run it.

### Other commands

```bash
make backend     # just the API on :8080
make frontend    # just the Vite dev server on :5173
make test        # backend Go tests
make reset       # delete the SQLite DB (re-seeds on next start)
```

Environment variables: `EDI_ADDR` (default `:8080`), `EDI_DB` (default
`edi.db`), `EDI_CLIENT_DIR` (default `../client/dist`), `EDI_TOKEN` (optional —
see below; empty means no auth, the localhost default).

### API token auth (for connecting agents / remote clients)

Start the server with a shared secret and every `/api` route (except
`/api/health`) requires `Authorization: Bearer <token>`:

```bash
openssl rand -hex 24 > .edi-token          # generate once (gitignored)
EDI_TOKEN=$(cat .edi-token) make backend   # 401 without the token, 200 with it
```

All clients understand it:

- **curl:** `curl -H "Authorization: Bearer $(cat .edi-token)" localhost:8080/api/dashboard`
- **CLI / MCP:** set `EDI_TOKEN` in the environment (`EDI_TOKEN=$(cat .edi-token) ./bin/edi-cli dashboard`)
- **Web UI:** open `http://host:8080/#token=<secret>` once — the token is stored
  in localStorage and sent automatically afterwards.

---

## Core loop

1. See today's quests on the dashboard.
2. Complete a quest → instant XP + a reward animation.
3. Watch attribute bars, levels, streak, and daily progress update.
4. The agent suggests a useful next quest.
5. Repeat.

## Concepts

- **Attributes** (9): Strength, Discipline, Focus, Health, Wealth, Relationships,
  Learning, Creativity, Spirituality. Each has XP and a level.
- **Level formula (MVP):** `level = floor(sqrt(total_xp / 100)) + 1`.
- **Quests** have a type (`daily/weekly/main/side/boss/recovery`), difficulty, status,
  and per-attribute XP rewards, e.g. `{"strength": 40, "discipline": 10}`.
- **Subtasks (bonus objectives):** a quest can carry optional subtasks, each with its
  own bonus rewards — "Go to the gym" might have "Bike there instead of driving"
  `{health: 15}`. Check them off while the quest is active
  (`POST /quests/:id/subtasks/:sid/toggle`); checked subtasks add their bonus as
  separately-labeled `xp_events` in the same completion transaction. They never
  block completion.
- **Auditable XP:** totals live on `attributes`, but **every** change also writes an
  immutable `xp_events` row. `attributes.total_xp` always equals the sum of its events.
- **Boss** quests get a distinct, intense visual; **recovery** quests feel soft and
  non-punitive.

---

## API

Base: `/api`

| Method | Path | Description |
|---|---|---|
| GET | `/dashboard` | Full dashboard payload (character, attributes, today's quests, streak, recent XP, recommended quest, suggestions) |
| GET | `/attributes` | All attributes with derived level/progress |
| GET | `/quests?type=&status=` | List/filter quests |
| POST | `/quests` | Create a quest |
| PATCH | `/quests/:id` | Update a quest (partial) |
| POST | `/quests/:id/complete` | Complete → awards XP, writes events, updates streak, returns refreshed dashboard |
| POST | `/quests/:id/skip` | Skip (increments skip counter) |
| POST | `/quests/:id/archive` | Archive |
| POST | `/quests/:id/subtasks/:sid/toggle` | Check/uncheck a bonus objective (while active) |
| GET | `/xp-events?limit=` | Recent XP audit events |
| GET | `/journal?limit=&q=` | Recent reflections (optional full-text search over notes) |
| POST | `/journal` | Add a reflection (mood/energy 1–10 + notes); first entry of the day awards XP |
| PATCH | `/journal/:id` | Edit a reflection |
| DELETE | `/journal/:id` | Delete a reflection (awarded XP stays — the audit log is immutable) |
| GET | `/agent/suggestions?status=` | List suggestions |
| POST | `/agent/suggestions/generate` | Run the rule engine, persist + return pending |
| POST | `/agent/suggestions/:id/accept` | Accept → creates a quest |
| POST | `/agent/suggestions/:id/dismiss` | Dismiss |
| GET | `/agent/tools` | **Discover** the agent tool catalog (names + JSON Schemas) |
| POST | `/agent/tools/:name/invoke` | **Invoke** a tool — same path a future LLM agent uses |

### Agent-ready by design

`server/internal/agent` wraps the service layer as 13 named tools with JSON Schemas
(`get_dashboard`, `create_quest`, `complete_quest`, `generate_suggestions`,
`accept_suggestion`, …). They are discoverable at `GET /api/agent/tools` and callable
at `POST /api/agent/tools/:name/invoke`. Each tool forwards to the **same**
`services.Service` the REST handlers use — wiring an LLM or MCP bridge later requires
no new data path.

```bash
curl localhost:8080/api/agent/tools | jq '.tools[].name'
curl -X POST localhost:8080/api/agent/tools/get_weakest_attribute/invoke -d '{}'
```

---

## Clients: web · CLI · AI agent (one API, proven)

All clients are thin shells over the REST API above — none touch the database
directly. They share a typed Go API client (`server/internal/apiclient`).

### CLI

```bash
make backend                      # start the API (or `make dev`)
make cli ARGS=dashboard           # character, attributes, today's quests, streak
make cli ARGS="quests --status active"
make cli ARGS='add --title "Cold plunge" --type daily --reward discipline=25 --reward health=15'
make cli ARGS="complete 3"        # shows XP gained + level-ups
make cli ARGS=suggest-gen
make cli ARGS=tools               # the agent tool catalog
# or build once:  ./bin/edi-cli dashboard   (after `make build`)
```

### MCP server (AI agent bridge)

`edi-mcp` is a Model Context Protocol server (stdio JSON-RPC) that exposes the
13 agent tools to an AI client. It is a pure proxy to `/api/agent/tools` — the agent
drives the app through the **same service path** as every other client. A quest the
agent creates is immediately visible to the web UI, CLI, and DB.

Point an MCP-capable client (Claude Desktop / Claude Code) at it — start the API
(`make backend`) first, then add:

```jsonc
{
  "mcpServers": {
    "edi": {
      "command": "/absolute/path/to/bin/edi-mcp",
      "env": { "EDI_API": "http://localhost:8080" }
    }
  }
}
```

(Build the binary with `make build`.) Tools available to the agent: `get_dashboard`,
`list_quests`, `create_quest`, `complete_quest`, `skip_quest`, `archive_quest`,
`create_journal_entry`, `list_journal_entries`, `get_weakest_attribute`,
`generate_suggestions`, `accept_suggestion`, `dismiss_suggestion`, `update_quest`.

#### Example: connecting the OpenAI Codex CLI

```bash
codex mcp add edi \
  --env EDI_API=http://localhost:8080 \
  --env EDI_TOKEN=$(cat .edi-token) \
  -- /absolute/path/to/bin/edi-mcp

codex exec "Call the edi get_weakest_attribute tool and report the result."
```

Add `EDI_TOKEN` only if the server runs with auth enabled; against a tokenless
localhost server it is simply ignored. The same pattern works for any MCP-capable
agent (Claude Desktop/Code, OpenClaw-style bots, …).

---

## AI coach — powered by your ChatGPT subscription

edi's suggestions are generated by a real LLM running on **your own** ChatGPT
Plus/Pro account — no API key, no extra cost. AI features are gated on a
connection; there is no offline fallback.

**Connect** (Agent tab → *Connect your ChatGPT account*):

- **Sign in with ChatGPT** — the same OAuth (PKCE) flow the Codex CLI uses. edi
  opens `auth.openai.com`, catches the `localhost:1455` callback, and stores the
  tokens in `edi.db` (auto-refreshed). Endpoints: `POST /api/openai/connect`
  (returns the auth URL) → browser → `GET /api/openai/status`.
- **Import from Codex CLI** — if you already ran `codex login`, `POST
  /api/openai/import-codex` reads `~/.codex/auth.json` for instant setup.
- **Disconnect** — `POST /api/openai/disconnect` clears the stored tokens.

`generateSuggestions()` builds a prompt from your attributes, weekly XP, streak,
active quests, and recent journal entries, asks the model for 2–4 tailored quests
as strict JSON, and stores them as pending. Accepting one creates a real quest.
Under the hood it calls the ChatGPT backend `responses` endpoint — the same
subscription-billed surface Codex uses. These OpenAI endpoints are undocumented
and may change.

**Model and reasoning effort** are both selectable in the connected bar and
persist per user (`POST /api/openai/config`). The available models come straight
from your account (`GET /api/openai/models` → the ChatGPT `codex/models`
endpoint) — e.g. GPT-5.6-Sol / GPT-5.6-Terra / GPT-5.6-Luna / GPT-5.5 — and each
model's supported reasoning levels drive the effort picker. Defaults are
`gpt-5.6-sol` / `medium`; override with `EDI_OPENAI_MODEL` / `EDI_OPENAI_EFFORT`.

---

## Tools (guided instruments)

The **Tools** tab holds guided exercises that award XP when completed — like
quests, but structured and self-scoring. The first is Dr. David Burns'
**Daily Mood Log** (TEAM-CBT): describe an upsetting moment, rate your emotions
(% before → after), catch the negative thoughts and tag their cognitive
distortions (the 10), then write a truer, kinder response and re-rate belief.
Finishing awards **Health +25 · Spirituality +15 · Discipline +10** (auditable
`xp_events`, `source='tool'`, with the reward overlay + level-ups + streak).

**Optional AI coach** (when a ChatGPT account is connected): each negative
thought has a **✨ Find distortions** helper (the model tags which of the 10
distortions are present, with a one-line why) and **✨ Suggest a response**
(2–3 rational responses, each labeled with the TEAM-CBT method it uses — Examine
the Evidence, Double-Standard, Socratic, …). You always edit/accept; the AI never
auto-fills. It's framed as a supportive coach, **not therapy** — a one-time
privacy opt-in gates it, and content signaling crisis routes to support
resources (988 in the US) instead of coaching. The tool works fully without AI.

Tools are an extensible registry (`server/internal/tools`) — add a new tool by
implementing the `Tool` interface (definition + payload validation) and
registering it; the API and the entry storage are generic.

```
GET  /api/tools                    # available tools + XP rewards
POST /api/tools/{key}/complete     # validate payload, store entry, award XP
GET  /api/tools/{key}/entries      # history
POST /api/tools/{key}/assist       # AI coach: {mode:distortions|responses,…} (needs OpenAI)
```

## Project structure

```
client/                 React + Vite + TypeScript + Tailwind v4 SPA
  src/lib/              api client, types, React Query hooks, theme, reward overlay
  src/components/       CharacterHeader, AttributeCard, QuestCard, XPFeed, …
  src/pages/            Dashboard, Quests, Journal, Suggestions
server/
  main.go               wiring + graceful shutdown + static SPA serving
  migrations/           *.sql (embedded) + schema_migrations runner
  cmd/edi-cli/      terminal client (HTTP, over the REST API)
  cmd/edi-mcp/      MCP stdio server (AI agent bridge, over the tool registry)
  internal/db/          Store: connection, migrations, seed, all SQL
  internal/models/      domain entities + API response shapes
  internal/services/    tool-like business logic (the core; fully unit-tested)
  internal/handlers/    thin HTTP layer (routing, JSON, CORS, middleware)
  internal/agent/        service layer exposed as a discoverable tool registry
  internal/apiclient/    typed Go HTTP client shared by the CLI and MCP server
  internal/openai/       "Sign in with ChatGPT" OAuth + responses API client
  internal/tools/        guided instruments registry (Daily Mood Log)
scripts/dev.sh          run backend + frontend together
Makefile                install / dev / build / prod / test / reset
```

## Tech

- **Backend:** Go, `net/http` (1.22 routing), `database/sql` + `modernc.org/sqlite`
  (pure-Go, no CGO). Single binary; migrations embedded.
- **Frontend:** React 18, Vite 6, TypeScript, Tailwind v4, TanStack Query, Framer
  Motion, lucide-react.
- **DB:** SQLite (WAL). Easy to swap for Postgres later (all SQL is in `db.Store`).

## Tests

```bash
make test   # XP/level math, quest completion + level-ups, CONCURRENT completion
            # (no double-XP, race-tested), suggestion accept/dismiss, dedup,
            # journal validation, and the empty-list JSON contract
```

The frontend is validated end-to-end with browser automation (dashboard render,
quest completion → XP/level update, journal save, suggestion → quest, error toasts,
clean console) rather than unit tests — see the validation report in the commit/PR.

## Known limitations (MVP scope)

- **Single-user.** Fixed user id 1. Auth is an optional shared bearer token
  (`EDI_TOKEN`); CORS is restricted to loopback origins. Set a token (and use
  HTTPS via a reverse proxy) before exposing beyond localhost — there are no
  per-user accounts.
- **Due dates** exist in the data model/API but have no UI date-picker yet.
- **AI features require a connected ChatGPT account** (no offline/rule fallback).
  They use OpenAI's undocumented Codex/ChatGPT backend endpoints, which may change.
- **No automated frontend unit tests** (Vitest) yet; UI is covered by browser e2e.
  The live OpenAI paths are covered by opt-in tests (`EDI_LIVE_TEST=1`).
- SQLite + single connection is intentional for a self-hosted single user; swap
  `db.Store` for Postgres + a pool when scaling to many users.
