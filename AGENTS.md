# AGENTS.md — working in the edi (Life RPG) codebase

Guidance for AI agents (and humans) editing this repo. Read this before changing code.
(`CLAUDE.md` is a symlink to this file — edit only `AGENTS.md`.)

## What this is

A self-hosted Life RPG. Real-life actions are "quests"; completing them awards XP to
9 life attributes, which level up. Multi-tenant (token per user). Go + PostgreSQL
backend, React + Vite frontend. See `README.md` for the product/run overview.

## The one architectural rule

**Every client — web UI, CLI, mobile, AI agent — goes through the same backend API.
There is no hidden data layer.** Concretely:

- All business logic lives in `server/internal/services` (`services.Service`). This is
  the single source of truth.
- HTTP handlers (`internal/handlers`) are thin: parse → call a `Service` method →
  write JSON. No business logic in handlers.
- The agent tool registry (`internal/agent`) wraps the **same** `Service` methods as
  named tools. The UI never embeds agent logic; the agent never bypasses the service.

**When you add a feature, add it to `services.Service` first**, then expose it via a
handler **and** (if it's an action an agent should take) an `agent` tool. Then mirror
the JSON shape in `client/src/lib/types.ts`.

Two non-web clients prove this and should keep working: `cmd/edi-cli` (terminal)
and `cmd/edi-mcp` (MCP stdio server for AI agents). Both are thin HTTP clients via
`internal/apiclient` — they never touch the DB. If you add an endpoint the CLI
should surface or an agent tool, extend `internal/apiclient` (typed methods) and
the CLI rather than adding a second data path. The MCP server is a pure proxy to
`/api/agent/tools`, so new agent tools appear there automatically.

Telegram is NOT an external client — it is an in-process **transport**, exactly
like the HTTP handlers: `internal/presence` parses messages, calls
`services.Service` methods on the linked user, and formats replies (multi-tenant
needs per-user action without impersonation credentials, which only works
in-process). No business logic in presence, ever.

### Client parity checklist (run it for EVERY new service method)

The rule above drifted once anyway: push times were settable only from
Telegram (no HTTP route), story narration only from Telegram, the mood log only
from the web. To stop that recurring, every new `services.Service` method gets
an explicit decision per client — "not applicable" is fine, "forgot" is not:

| Client | What "exposed" means | Where |
|---|---|---|
| HTTP API | a thin handler + route (`h.forUser(r)`, never `h.svc`) | `handlers/router.go` |
| Agent / MCP / chat | a named tool wrapping the same method | `agent/agent.go` `NewRegistry` |
| CLI | typed `apiclient` method + a command (or at least reachable via `edi-cli invoke <tool>`) | `apiclient/client.go`, `cmd/edi-cli` |
| Web | `api.ts` method + query/mutation hook + UI, shape mirrored in `types.ts` | `client/src/lib` |
| Telegram | a slash command only when it's a frequent pocket action; otherwise free-text chat already reaches it through the agent tool | `presence/runner.go` |

Rules of thumb:
- **A setting the scheduler/UI reads must be writable over HTTP**, not just
  from the transport that happens to use it (`POST /api/telegram/push-times`
  is the precedent; the scheduler re-anchors from the stored value, so it
  does not care which client wrote it).
- **If Telegram can do it, the web and the agent can do it** (`/story` ↔
  `POST /api/story` ↔ `tell_story`; `/boss` ↔ `forge_boss`).
- **If it awards XP, the agent can do it** (`complete_tool` for guided
  instruments) — through the same auditable store path.
- `edi-cli invoke <tool> [json]` is the generic escape hatch: a new agent tool
  is instantly reachable from the shell and from MCP, so register the tool
  first and add a bespoke CLI command only when the output deserves formatting.

Deliberately NOT in the agent registry (keep it that way, and say so here if
you add to the list): user/admin management and tokens, OpenAI connect/config,
Telegram pairing/unlinking (identity + credentials stay UI/CLI-only), and
`POST /api/tools/{key}/assist` (the chat model already *is* the coach; the
consent + crisis gating is a UI-path concern). Free-text chat is CLI +
Telegram (the web has no chat box yet — an open gap, not a rule).

Quick audit: `grep -oE 'add\("[a-z_]+"' server/internal/agent/agent.go`,
`grep HandleFunc server/internal/handlers/router.go`, `edi-cli help`, and
`grep -oE 'request<[^(]*\("/[^"]*' client/src/lib/api.ts` — diff them.

## Commands

```bash
make dev      # backend :8080 + frontend :5173 (Vite proxies /api -> :8080)
make backend  # API only
make frontend # Vite only
make build    # client dist + all Go binaries -> bin/ (edi, edi-cli, edi-mcp)
make prod     # build + run the self-hosted binary on :8080
make test     # go test ./... (needs local Postgres — see make db-setup)
make db-setup # one-time: create the local edi_dev + edi_test Postgres databases
make reset    # drop + recreate edi_dev (re-seeds on next start)
```

Local Postgres must be running on :5432 (Postgres.app / homebrew).

Always run from the repo root. Go module root is `server/` (module `edi`).

## The agentic validation loop (REQUIRED — do not skip)

This codebase is built and maintained with a continuous validation loop, not
"generate code and hope." For **every meaningful change**, run this loop:

1. **Implement a small vertical slice** — the smallest change that produces an
   observable result. Don't build a large unverified system.
2. **Run the relevant command** (build / test / curl / psql query / browser).
3. **Inspect the actual output** — read it, don't assume it.
4. **Verify the feature actually works** against expected behavior.
5. **If it fails:** read the error carefully, fix the root cause, and **rerun**.
6. **Repeat until it genuinely works.** Only then move on.

**A change is not "done" until it has been validated by command/test/HTTP/DB/browser
output.** Never claim something works on assertion alone — show the evidence. If a
tool needed for validation is missing, say so explicitly and use the closest
alternative; don't silently skip the check.

### What to run, by layer

- **Backend:** `cd server && go build ./... && go vet ./... && go test ./...`, then
  `gofmt -w .`. Add/extend a test for the behavior you changed (the suite already
  covers XP math, completion + level-ups, **concurrent** completion with `-race`,
  suggestions, journal, and the JSON array contract).
- **Live API:** start `make backend`, hit it with `curl`, and **confirm the side
  effects in the DB**: `psql edi_dev -c "<query>"` (e.g. verify xp_events were
  written and the audit invariant still holds). Check status codes too —
  client-caused errors must be 400/404, not 500 (and unauthenticated must be 401).
- **Frontend:** `cd client && npm run build` (runs `tsc --noEmit` + Vite build).
- **UI behavior:** drive a real browser via the **`aside` MCP server**
  (configured in `.mcp.json`; the Aside Browser) — load the page, perform the
  action, confirm the DOM/XP updates, and check the **browser console is
  clean** (no errors/warnings). Don't reach for agent-browser/Playwright
  unless aside is unavailable. Watch out for backend failures being hidden by
  the UI; they must surface as a toast/error state.

This loop is exactly how the critical concurrency bug and the nil-slice crash were
caught and fixed — keep using it.

## Invariants you must not break

- **XP is auditable.** `attributes.total_xp` must always equal `SUM(xp_events.amount)`
  for that attribute. Never bump a total without writing an `xp_events` row in the same
  transaction. There's a check you can run:
  ```sql
  SELECT a.key, a.total_xp,
         (SELECT COALESCE(SUM(amount),0) FROM xp_events e
          WHERE e.attribute_key=a.key AND e.user_id=a.user_id) AS sum_events
  FROM attributes a;
  ```
- **Gold is auditable the same way.** The balance is always
  `SUM(gold_events.amount)` computed on read — there is no stored balance
  column. Minting (1g per 10 XP, min 1, `services.GoldForXP` mirrored by
  `db.goldForXP`) happens inside the SAME tx as the xp_event; purchases check
  the balance inside the purchase tx so it can never go negative (regression
  tests: `TestGoldAuditInvariant`, `TestShopPurchaseConcurrentNoOverspend`).
- **Decay is auditable, idempotent, and floored.** Neglected attributes lose
  XP via negative `xp_events` (`source='decay'`, note `decay · YYYY-MM-DD`)
  written by the lazy catch-up (`store.ApplyDecay`) inside one tx — never a
  bare total_xp decrement. Billing is idempotent per attribute per local day
  (the billed dates in the notes are re-read inside the tx), never bills
  ward-covered days or rest periods, and never drops below
  `XPForLevel(LevelForXP(peak_xp)-2)`. `peak_xp` is maintained in the same tx
  as every award and never decreases. Rules live in `services/decay.go`
  (mirrored in `db/decay_math.go`). Regression tests: `TestDecayCatchUp`,
  `TestDecayConcurrentSingleApplication` (run with `-race`),
  `TestDecaySkipsRest`, `TestDecayWardExcludesCoveredDays`.
- **Completion goes through `CompleteQuest`/`SkipQuest` only** — never via a generic
  `PATCH status`. The service rejects `status:completed|skipped` patches on purpose.
- **Quest completion is atomic and idempotent.** `store.CompleteQuest` gates on a
  conditional `UPDATE ... AND status NOT IN ('completed','archived')` and checks
  `RowsAffected()` *inside a `beginUserTx` transaction* (per-user advisory lock).
  Don't reintroduce a read-status-then-write pattern outside such a tx — that
  double-awards XP under concurrent/double-tap requests
  (regression test: `TestCompleteQuestConcurrentNoDoubleAward`, run with `-race`).
- **Subtask bonuses are frozen at completion.** Checked subtasks (bonus objectives)
  are read and awarded *inside* the same completion tx as separately-labeled
  xp_events ("quest · subtask"); level-ups are computed cumulatively per attribute
  (base+bonus counted once — see `TestSubtaskCumulativeLevelUp`). Toggling after
  completion is rejected (`ErrQuestNotCompletable` → 400). Subtasks never block
  completion.
- **Slices serialize as `[]`, never `null`.** Go nil slices marshal to JSON `null`,
  which crashes the frontend. Wrap every list/dashboard slice field with
  `orEmpty(...)` at the service boundary. Regression test:
  `TestEmptyListsSerializeAsArrays`.
- **Level formula** (don't change without updating tests): `level = floor(sqrt(total_xp/100)) + 1`.
  All level math lives in `services/xp.go` as pure functions.

## AI features (ChatGPT-subscription LLM)

- AI runs on the user's own ChatGPT account via "Sign in with ChatGPT" (Codex
  OAuth). All OpenAI code is in `internal/openai` (OAuth PKCE + the `responses`
  API client); tokens live in the `openai_credentials` table (`db/openai_store.go`),
  auto-refreshed in `services/openai.go` (`accessToken`/`completeWithOpenAI`).
- **AI features are gated on a connection — there is no offline/rule fallback.**
  Anything needing the LLM returns `ErrOpenAINotConnected` (→400) when disconnected.
  `GenerateSuggestions` builds a prompt from live state and asks for strict JSON.
- **Model and reasoning effort are both user-selectable** and stored per user in
  the `app_settings` table (`POST /api/openai/config`). Available models come from
  the account via `openai.ListModels` (`codex/models?client_version=…`, exposed at
  `GET /api/openai/models`) — the request needs a recent `CodexClientVersion`
  (bump it if the endpoint returns `[]` or hides newer families — gpt-5.6 needs
  ≥0.144.0). Only ids the account lists actually work (e.g. `gpt-5.6-sol`/
  `gpt-5.6-terra`/`gpt-5.6-luna`/`gpt-5.5`/`gpt-5.4`/`gpt-5.4-mini`;
  arbitrary/`*-codex` ids 400 at generate). `s.openAIModel()`/`s.openAIEffort()`
  resolve setting → env (`EDI_OPENAI_MODEL`/`EDI_OPENAI_EFFORT`) → default
  (`gpt-5.6-sol`/`medium`); both are passed into `openai.Complete`.
- **Connecting works remotely, per user**: the OAuth redirect URI is fixed to
  `localhost:1455`, so on a deployed server the browser lands on a dead page —
  the user copies that URL and pastes it into the UI, which calls
  `POST /api/openai/connect/complete` (code+state exchange happens
  server-side). Pending flows are per-user (`oauthRuntime.pending`), burn on
  every attempt, and expire after 10 min.
- These are OpenAI's **undocumented** endpoints (`chatgpt.com/backend-api/codex/
  responses`, `auth.openai.com`). Verify changes with the opt-in live tests:
  `EDI_LIVE_TEST=1 go test ./internal/openai ./internal/services -run Live`.
  Normal `go test` stays offline (the live tests skip without the env var).
- The `:1455` OAuth callback binds transiently during connect; the "Import from
  Codex CLI" path (`~/.codex/auth.json`) is the no-browser shortcut for testing.

## Tools (guided instruments)

"Tools" are guided exercises that award XP (first: the Daily Mood Log, TEAM-CBT).
They live in `internal/tools` as a registry: each tool implements `Tool`
(`Definition()` + `Validate(payload) → (clean, summary, err)`), rewards come from
the definition. `store.CompleteTool` awards XP the SAME auditable way as quest
completion (xp_events `source='tool'` + attribute bump + streak, in one tx) — keep
the invariant. Add a tool = new type in `internal/tools` + register in
`NewRegistry`; the API (`/api/tools…`) and `tool_entries` storage are generic.
The reward overlay is generic (`lib/reward.tsx` `RewardPayload`), so any
XP-awarding action can call `celebrate({title, xp_events, level_ups, label})`.

Tools can have an **optional AI coach** (`services/tool_assist.go`,
`POST /api/tools/{key}/assist`) that reuses `completeWithOpenAI` (gated on the
ChatGPT connection). Two rules are load-bearing and must not regress: (1) AI
**suggests, never auto-fills** — it proposes distortions/responses the user
edits/accepts; (2) it is self-help, **not therapy** — every prompt carries the
`safetyPreamble`, crisis content returns `crisis=true` (support resources, no
coaching), and the UI requires a one-time privacy opt-in (`lib/aiConsent.tsx`)
before sending private entry text to OpenAI. AI assist is always optional; the
tool works fully without a connection.

## Journal

The journal is a first-class tool: entries support edit/delete/search, and the
**first entry of each local day** awards `journalDailyRewards` (spirituality 10,
discipline 5) through the same auditable path (`xp_events source='journal'` +
streak, one tx in `store.InsertJournal`; regression: `TestJournalDailyXPOnce`).
Deleting an entry never claws back XP. The UI shows per-day mood/energy
sparklines and a 10-week consistency heatmap (single-hue phosphor ramp),
computed client-side from entries.

## Presence: Telegram channel + shell status

Telegram runs **in-process** (`internal/presence`, enabled by
`TELEGRAM_BOT_TOKEN` on the server) on the stdlib-only Bot API client in
`internal/telegram`. Multi-tenant by pairing: each user mints a single-use pair
code (web UI → `POST /api/telegram/pair-code`) and sends `/pair <code>` (or the
`t.me/<bot>?start=<code>` deep link) to the bot; `telegram_links` maps chat ↔
user, and every command runs on `svc.ForUser(linked)`. Commands: /status
/quests /done /ward /rest /briefing /nudge /story /boss /new /unpair — plus
free-text chat (below). Pushes: per-user briefing + conditional nudge at
per-user times (app_settings, read/written by every client through
`GET|POST /api/telegram/push-times` / `edi-cli push-times` / the
`set_push_times` tool / `/briefing HH:MM`; `""` = server default) falling
back to `EDI_BRIEFING_TIME`/`EDI_NUDGE_TIME`. The scheduler stores the HH:MM
it anchored on and re-anchors when the setting changes
(`TestPresenceScheduleFollowsServiceSetting`), re-checks the wall clock
(suspend/DST-safe), skips pushes >10 min late, never replays. **Only ONE
environment may set TELEGRAM_BOT_TOKEN per bot** — concurrent getUpdates
pollers 409; local live-testing needs a second throwaway bot. Isolation
regression: `TestPresenceMultiUserIsolation`. Live-tested against a real bot
(@edi_rpg_bot): deep-link pairing verified end-to-end on the deployed server.
`edi-cli status` prints a fail-silent stats block for shell startup (never
break a shell prompt on server errors).

**Free-text chat** (any message without a leading `/`) goes to the
conversational agent: `internal/agent` `Registry.Chat` runs the user's own
ChatGPT model (`openai.Converse`, native function calling, `store=false` —
the history is replayed each round, including the model's raw output items)
over the SAME tool registry, so "add X as a daily" / "I finished X" / "how's
my streak" become `create_quest` / `list_quests`+`complete_quest` (or
`record_spontaneous_quest`) / `get_dashboard`. Rules: the model never touches
data except through `registry.Invoke` on the user-bound Service; tool errors
are fed back for self-correction (cap: 3 consecutive failures, 8 rounds,
90s); tool results are truncated; conversation history is **in-memory per
chat** (`agent.Sessions`, 2h idle TTL, `/new` clears; a redeploy forgets it —
every action already landed via the service). Presence answers chat off the
poll loop (goroutine, per-session lock, typing indicator kept alive),
**private chats only** (group chatter would spend/act on the paired user's
account — it falls through to help) and escapes the
model text whole (`SendMessage` is HTML). No connection → the connect hint;
slash commands keep working without AI. The same loop is exposed as
`POST /api/agent/chat {message, session?, reset?}` (`edi-cli chat`), gated
like every AI feature (`ErrOpenAINotConnected` → 400). Offline tests inject a
scripted `agent.LLM` (`chat_test.go`, `TestPresenceFreeTextChat`); the wire
contract is checked by `TestLiveConverseToolCall` (`EDI_LIVE_TEST=1`).

## Conventions

### Backend (Go)
- Storage is **PostgreSQL** via `github.com/jackc/pgx/v5/stdlib` (pure Go, no CGO)
  over `database/sql`. DSN: `EDI_DATABASE_URL` → `DATABASE_URL` → localhost
  `edi_dev`.
- **Per-user write serialization is an invariant.** Every read-then-write
  transaction (completion gates, gold balance checks, decay idempotency,
  first-entry-of-day checks) MUST start with `store.beginUserTx(userID)`, which
  takes `pg_advisory_xact_lock(userID)` as its first statement. This is what
  replaced SQLite's single-writer connection; the concurrency regression tests
  (`-race`) only stay honest if new write paths keep doing this.
- Timestamps are `timestamptz`; pass/scan `time.Time` (UTC in, driver converts).
  Local-day math (streaks, decay, journal daily XP) happens in **Go** —
  `localDayBounds`/`localDate` — never with SQL zone names.
- Placeholders are `$1..$n` (pgx has no `?`). Inserts needing the id use
  `INSERT ... RETURNING id` + `QueryRow.Scan` — `LastInsertId` does not work.
- All SQL lives in `internal/db` (`Store`). Services never write SQL directly.
- Tests get isolated stores from `internal/db/dbtest` (a throwaway schema per
  test in `edi_test`, real migrations applied). Skip/fail rule: with
  `EDI_TEST_DATABASE_URL` set the DB must be reachable (CI can never silently
  skip); unset, tests skip loudly when localhost Postgres is absent — which is
  why the Docker build only gates on vet + pure tests, and `.github/workflows/
  ci.yml` (postgres service container) is the full gate.
- **Error → HTTP mapping**: return `services.ErrValidation` (→400) or
  `services.ErrNotFound` (→404) from the service; anything else is 500. Store-level
  sentinels (`db.ErrNotFound`, `db.ErrQuestNotCompletable`, `db.ErrSuggestionNotPending`)
  must be translated to the `services.*` errors in the service layer. Don't return bare
  `fmt.Errorf` for client-caused conditions — it becomes a 500.
- New entity? migration in `server/migrations/NNN_*.sql` (embedded, auto-applied in
  lexical order via `schema_migrations`). Add indexes where you query.

### Frontend (React + TS)
- Data fetching is **TanStack Query** (`lib/queries.ts`). Mutations invalidate the
  relevant query keys in `onSuccess`. Keep keys consistent with `keys` there.
- A **global `MutationCache.onError` shows a toast** (`lib/toast.tsx`) so no action
  fails silently. New mutations get this for free — don't swallow errors.
- Tailwind **v4** (CSS-first): design tokens are in `@theme` in `src/index.css`; there
  is no `tailwind.config.js`. Attribute/quest colors + icons live in `lib/theme.ts`.
- Reward feedback (XP/level-up overlay) is `lib/reward.tsx` — call `celebrate(result)`
  from a completion `onSuccess`.
- **No mock state.** The UI must read real backend data and surface real loading/error
  states (`Spinner`, `EmptyState`, `ErrorBoundary`, toasts). Don't hide backend failures.
- Strict TS (`noUnusedLocals`/`noUnusedParameters`); the build fails on unused symbols.
- **i18n (EN/DE):** no hardcoded UI strings. Every user-visible string goes
  through `lib/i18n.tsx` — `useI18n().t("key", {vars})` in components,
  `t()`/`tp()` (plural pairs `key.one`/`key.other`) anywhere else. Keys live in
  `lib/locales/en.ts` (source) and `de.ts`, which is typed
  `Record<MessageKey, string>` so a missing German string fails `tsc`. Labels
  that used to be module constants (attributes/types/difficulty in `theme.ts`,
  CBT vocab in `cbt.ts`, nav items) resolve lazily via `t()`. Locale is a
  per-device preference (`localStorage edi.locale`, default from
  `navigator.language`); switching remounts the tree (`key={locale}` in
  `main.tsx`). Dates/numbers go through `lib/format.ts` (locale-aware). Keep
  `data-testid`s independent of display text. Server-generated content
  (achievements, loot, titles, AI text, error messages) is not localized here.

## Users & auth (multi-tenant)

Multi-tenant, **token-based, no passwords**: every user owns one bearer token
(48 hex chars, shown once at creation; the server stores only its SHA-256 in
`users.token_hash`). Every client — web UI, CLI, MCP, Telegram — sends it as
`Authorization: Bearer <token>`; the middleware (`handlers/auth.go`) resolves
it to a user id in the request context, and handlers call
`h.forUser(r)` → `svc.ForUser(id)` (a cheap per-request copy). **Never bind a
handler or agent tool to `h.svc` directly** — that's the dev-fallback user.

Two server modes, decided by `EDI_TOKEN`:
- **Set (deployed):** at every boot the server *adopts* it as user 1's token
  (creating a blank admin "Hero" on an empty DB). Idempotent — which makes
  changing the env var + restart the recovery path for user 1's access, and is
  why the admin token-rotation API refuses user 1. Anonymous /api requests are
  401 (except `/api/health`, `GET /api/auth/config`, `POST /api/auth/register`).
- **Unset (localhost dev):** no auth; anonymous requests act as user 1; demo
  data seeds on a fresh DB. This means `EDI_TOKEN` is load-bearing for auth on
  a public deployment — never unset it there.

More users: self-serve registration gated on `EDI_INVITE_CODE` (unset =
registration closed; `POST /api/auth/register {name, invite_code}` → token,
shown once), or the admin API (`POST /api/admin/users`, `POST
/api/admin/users/{id}/token` to re-mint a lost token). User management is
deliberately NOT in the agent tool registry. Isolation is enforced by the
`user_id` filter in every store query — `TestTenantIsolation` is the
regression; extend it when adding entities. The web UI's `TokenGate` handles
401 → paste-token / register; a one-time `/#token=<t>` URL also works.

External agents connect through the MCP server with a user's token, e.g.
`codex mcp add edi --env EDI_API=... --env EDI_TOKEN=... -- bin/edi-mcp`.
CORS stays restricted to loopback origins (`isLoopbackOrigin`) — a token is
not a reason to loosen it.

## Deployment (Railway) — the live instance is real

**Push to `main` deploys.** The `edi-server` Railway service builds this repo's
`Dockerfile` on every push to `frknue/edi@main` (health check `/api/health`;
`PORT` honored; `TZ` must stay set — local-day math depends on it). The image
build gates on vet + pure tests only; the full suite (Postgres-backed, `-race`)
runs in GitHub Actions. `make deploy` (`railway up`) is the out-of-band path.

Data lives in the Railway **Postgres service** (`DATABASE_URL` reference on
edi-server) and is the ONLY copy of the real characters. Rules that keep it
safe:
- **Migrations must be additive** (new tables/columns/indexes). Anything
  destructive or shape-changing needs a backup first and a written revert path.
- **`make backup-prod` before any schema or storage change** — dumps the live
  DB into `backups/` (gitignored).
- Never point a local dev server at the production `DATABASE_URL`.
- The one-shot SQLite-era restore tool is `scripts/sqlite-to-pg.py` (kept for
  the old backups in `backups/`).

## Don't

- Don't add npm/Go dependencies casually — the stack is intentionally lean.
- Don't put logic the agent would need behind UI-only code.
- Don't store derived values (levels, progress) — compute them from `total_xp` on read.
- Don't claim completion without validating via build/test/curl/browser output.
- Don't write a read-then-write transaction without `beginUserTx` (see Backend
  conventions) — it reintroduces the race class the advisory locks closed.
- Don't ship a migration to main without `make backup-prod` — main deploys.
