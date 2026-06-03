# ASCEND — a self-hosted Life RPG

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

Open **http://localhost:5173**. The database (`server/liferpg.db`) is created and
seeded with demo data automatically on first run.

### One self-hosted binary (production-style)

```bash
make prod        # builds the web client + a single Go binary, serves both on :8080
```

Open **http://localhost:8080**. The Go binary embeds its migrations and serves the
built SPA — copy `bin/liferpg` + `client/dist` to a server and run it.

### Other commands

```bash
make backend     # just the API on :8080
make frontend    # just the Vite dev server on :5173
make test        # backend Go tests
make reset       # delete the SQLite DB (re-seeds on next start)
```

Environment variables: `LIFERPG_ADDR` (default `:8080`), `LIFERPG_DB` (default
`liferpg.db`), `LIFERPG_CLIENT_DIR` (default `../client/dist`).

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
| GET | `/xp-events?limit=` | Recent XP audit events |
| GET | `/journal?limit=` | Recent reflections |
| POST | `/journal` | Add a reflection (mood/energy 1–10 + notes) |
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

## Rule-based suggestions (MVP)

`generateSuggestions()` inspects recent activity and proposes quests:

- **Low attribute this week** → a quest for your weakest stat.
- **3+ Focus quests done** → a harder Focus challenge.
- **A quest skipped repeatedly** → an easier "mini" version.
- **High activity several days** → a recovery quest.

Suggestions are stored; accepting one creates a real quest. (Swap the rules for an
LLM later without touching any client.)

---

## Project structure

```
client/                 React + Vite + TypeScript + Tailwind v4 SPA
  src/lib/              api client, types, React Query hooks, theme, reward overlay
  src/components/       CharacterHeader, AttributeCard, QuestCard, XPFeed, …
  src/pages/            Dashboard, Quests, Journal, Suggestions
server/
  main.go               wiring + graceful shutdown + static SPA serving
  migrations/           *.sql (embedded) + schema_migrations runner
  internal/db/          Store: connection, migrations, seed, all SQL
  internal/models/      domain entities + API response shapes
  internal/services/    tool-like business logic (the core; fully unit-tested)
  internal/handlers/    thin HTTP layer (routing, JSON, CORS, middleware)
  internal/agent/        service layer exposed as a discoverable tool registry
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

- **Single-user, no auth.** Fixed user id 1. CORS is restricted to loopback origins;
  do not expose this to the public internet as-is.
- **Due dates** exist in the data model/API but have no UI date-picker yet.
- **Agent suggestions are rule-based**, not an LLM — by design. The tool registry
  (`/api/agent/tools`) is the seam to add an LLM/MCP bridge with no client changes.
- **No automated frontend unit tests** (Vitest) yet; UI is covered by browser e2e.
- SQLite + single connection is intentional for a self-hosted single user; swap
  `db.Store` for Postgres + a pool when scaling to many users.
