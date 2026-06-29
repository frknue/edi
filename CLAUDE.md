# CLAUDE.md — working in the edi (Life RPG) codebase

Guidance for AI agents (and humans) editing this repo. Read this before changing code.

## What this is

A self-hosted Life RPG. Real-life actions are "quests"; completing them awards XP to
9 life attributes, which level up. Single-user MVP. Go + SQLite backend, React + Vite
frontend. See `README.md` for the product/run overview.

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

Two non-web clients already prove this and should keep working: `cmd/edi-cli`
(terminal) and `cmd/edi-mcp` (MCP stdio server for AI agents). Both are thin HTTP
clients via `internal/apiclient` — they never touch the DB. If you add an endpoint
the CLI should surface or an agent tool, extend `internal/apiclient` (typed methods)
and the CLI rather than adding a second data path. The MCP server is a pure proxy to
`/api/agent/tools`, so new agent tools appear there automatically.

## Commands

```bash
make dev      # backend :8080 + frontend :5173 (Vite proxies /api -> :8080)
make backend  # API only
make frontend # Vite only
make build    # client dist + single Go binary (bin/edi)
make prod     # build + run the self-hosted binary on :8080
make test     # go test ./...
make reset    # delete SQLite db (re-seeds on next start)
```

Always run from the repo root. Go module root is `server/` (module `edi`).

## The agentic validation loop (REQUIRED — do not skip)

This codebase is built and maintained with a continuous validation loop, not
"generate code and hope." For **every meaningful change**, run this loop:

1. **Implement a small vertical slice** — the smallest change that produces an
   observable result. Don't build a large unverified system.
2. **Run the relevant command** (build / test / curl / SQLite query / browser).
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
  effects in the DB**: `sqlite3 server/edi.db "<query>"` (e.g. verify xp_events
  were written and the audit invariant still holds). Check status codes too —
  client-caused errors must be 400/404, not 500.
- **Frontend:** `cd client && npm run build` (runs `tsc --noEmit` + Vite build).
- **UI behavior:** drive a real browser (the `agent-browser` skill / Playwright) —
  load the page, perform the action, confirm the DOM/XP updates, and check the
  **browser console is clean** (no errors/warnings). Watch out for backend failures
  being hidden by the UI; they must surface as a toast/error state.

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
- **Completion goes through `CompleteQuest`/`SkipQuest` only** — never via a generic
  `PATCH status`. The service rejects `status:completed|skipped` patches on purpose.
- **Quest completion is atomic and idempotent.** `store.CompleteQuest` gates on a
  conditional `UPDATE ... AND status NOT IN ('completed','archived')` and checks
  `RowsAffected()` *inside the transaction*. Don't reintroduce a read-status-then-write
  pattern outside the tx — that double-awards XP under concurrent/double-tap requests
  (regression test: `TestCompleteQuestConcurrentNoDoubleAward`, run with `-race`).
- **Slices serialize as `[]`, never `null`.** Go nil slices marshal to JSON `null`,
  which crashes the frontend. Wrap every list/dashboard slice field with
  `orEmpty(...)` at the service boundary. Regression test:
  `TestEmptyListsSerializeAsArrays`.
- **Level formula** (don't change without updating tests): `level = floor(sqrt(total_xp/100)) + 1`.
  All level math lives in `services/xp.go` as pure functions.

## Conventions

### Backend (Go)
- SQLite driver is `modernc.org/sqlite` (pure Go, **no CGO**). `SetMaxOpenConns(1)` —
  a single writer serializes access; keep it.
- Timestamps: stored as **TEXT, UTC, fixed-width** (`timeLayout` in `db/db.go`). Fixed
  width matters for `created_at >= ?` text comparisons. Use the existing helpers.
- All SQL lives in `internal/db` (`Store`). Services never write SQL directly. Swapping
  to Postgres later = reimplement `Store`, nothing else.
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

## Single-user mode

Fixed `userID = 1` (`main.go`). No auth. CORS is restricted to loopback origins
(`isLoopbackOrigin` in `router.go`) — do not open it up or expose publicly without
adding auth first.

## Don't

- Don't add npm/Go dependencies casually — the stack is intentionally lean.
- Don't put logic the agent would need behind UI-only code.
- Don't store derived values (levels, progress) — compute them from `total_xp` on read.
- Don't claim completion without validating via build/test/curl/browser output.
