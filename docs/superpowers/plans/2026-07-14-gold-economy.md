# Gold Economy + Reward Shop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every XP award also mints spendable gold (auditable ledger, balance computed on read); the user defines a shop of real-life rewards and purchases them transactionally.

**Architecture:** Mirrors the existing XP machinery 1:1. A `gold_events` table is the immutable ledger (mints positive, purchases negative); balance is always `SUM(gold_events.amount)` — no stored balance. Minting happens inside the existing completion transactions (quest/tool/journal). Shop CRUD + purchase go service-first, then HTTP handler, then agent tool, then apiclient/CLI, then the React client. Spec: `docs/superpowers/specs/2026-07-14-gold-economy-design.md`.

**Tech Stack:** Go 1.x + modernc.org/sqlite (no CGO), net/http stdlib mux, React + TS + TanStack Query + Tailwind v4.

## Global Constraints

- Run all Go commands from `server/` (module `edi`); repo root is `/Users/furkanulker/git/private/edi`.
- Minting rule (exact): `1 gold per 10 XP, minimum 1 for any positive XP award; 0 for non-positive`. Applied **per xp_event**, in the **same transaction**.
- Gold audit invariant: balance == `SUM(gold_events.amount)`; never store a balance column.
- Balance must never go negative: purchase checks balance **inside the purchase tx**.
- Error mapping: client-caused → `services.ErrValidation` (400) or `services.ErrNotFound` (404); store sentinels must be translated in the service layer.
- Every list returned by a service method wraps in `orEmpty(...)` (JSON `[]`, never `null`).
- Timestamps: TEXT, UTC, fixed-width `timeLayout` (`2006-01-02T15:04:05.000000000Z07:00`); use existing `nowString()`/`formatTime()` helpers.
- SQLite has one writer (`SetMaxOpenConns(1)`) — keep it.
- After every Go change: `cd server && go build ./... && go vet ./... && go test ./... && gofmt -w .`
- After every client change: `cd client && npm run build` (runs `tsc --noEmit`).
- Strict TS: no unused locals/params — the build fails on them.
- Commit after every task (messages below).

---

### Task 1: Gold schema, mint math, ledger reads

**Files:**
- Create: `server/migrations/006_gold.sql`
- Modify: `server/internal/services/xp.go` (add `GoldForXP`)
- Modify: `server/internal/services/xp_test.go` (add `TestGoldForXP`)
- Modify: `server/internal/models/models.go` (add `GoldEvent`, `ShopItem`, `ShopItemInput`, `ShopItemPatch`, `PurchaseResult`)
- Create: `server/internal/db/gold_store.go`
- Modify: `server/internal/db/seed.go` (seed gold grant)
- Modify: `server/internal/services/service.go` (add `GoldBalance`, `ListGoldEvents`)
- Test: `server/internal/services/service_test.go` (add `TestSeedGoldGrant`)

**Interfaces:**
- Consumes: existing `nullInt64`, `nowString`, `formatTime`, `orEmpty`, `newTestService`.
- Produces (later tasks rely on these exact signatures):
  - `services.GoldForXP(xp int64) int64`
  - `db: goldForXP(xp int64) int64` (private mirror)
  - `db: insertGoldEventTx(tx *sql.Tx, userID, amount int64, source, label string, shopItemID *int64, nowStr string) (int64, error)`
  - `(*db.Store).GoldBalance(userID int64) (int64, error)`
  - `(*db.Store).ListGoldEvents(userID int64, limit int) ([]models.GoldEvent, error)`
  - `(*services.Service).GoldBalance() (int64, error)`
  - `(*services.Service).ListGoldEvents(limit int) ([]models.GoldEvent, error)`
  - `models.GoldEvent{ID, Amount int64, Source, Label string, ShopItemID *int64, CreatedAt time.Time}`
  - `models.ShopItem{ID int64, UserID int64, Name string, Price int64, CreatedAt time.Time, ArchivedAt *time.Time}`
  - `models.ShopItemInput{Name string, Price int64}`, `models.ShopItemPatch{Name *string, Price *int64}`
  - `models.PurchaseResult{Item ShopItem, Event GoldEvent, Balance int64}`

- [ ] **Step 1: Write the failing mint-math test**

Append to `server/internal/services/xp_test.go`:

```go
func TestGoldForXP(t *testing.T) {
	cases := []struct {
		xp   int64
		want int64
	}{
		{-5, 0}, {0, 0}, {1, 1}, {5, 1}, {9, 1}, {10, 1}, {11, 1}, {19, 1},
		{20, 2}, {40, 4}, {100, 10}, {2520, 252},
	}
	for _, c := range cases {
		if got := GoldForXP(c.xp); got != c.want {
			t.Errorf("GoldForXP(%d) = %d, want %d", c.xp, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd server && go test ./internal/services -run TestGoldForXP`
Expected: FAIL — `undefined: GoldForXP` (compile error).

- [ ] **Step 3: Implement GoldForXP**

Append to `server/internal/services/xp.go`:

```go
// GoldForXP converts one XP award into minted gold: 1 gold per 10 XP,
// minimum 1 for any positive award. Non-positive XP mints nothing.
// db/gold_store.go keeps a private mirror (goldForXP) — change both together.
func GoldForXP(xp int64) int64 {
	if xp <= 0 {
		return 0
	}
	if g := xp / 10; g > 1 {
		return g
	}
	return 1
}
```

- [ ] **Step 4: Run the test — PASS**

Run: `cd server && go test ./internal/services -run TestGoldForXP`
Expected: PASS.

- [ ] **Step 5: Create the migration**

Create `server/migrations/006_gold.sql`:

```sql
-- 006_gold.sql — gold economy. Gold is the spendable twin of XP: every XP
-- award mints gold (1g per 10 XP, min 1), purchases spend it. gold_events is
-- the immutable ledger; the balance is ALWAYS computed as SUM(amount) on read
-- (same audit pattern as xp_events — never store a balance).

CREATE TABLE gold_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL REFERENCES users(id),
    amount       INTEGER NOT NULL,               -- + mint, - purchase
    source       TEXT NOT NULL,                  -- quest, subtask, tool, journal, purchase, grant
    label        TEXT NOT NULL DEFAULT '',       -- human-readable ("quest · subtask" / item name)
    shop_item_id INTEGER,                        -- set for purchases
    created_at   TEXT NOT NULL
);
CREATE INDEX idx_gold_events_user ON gold_events(user_id, created_at);
CREATE INDEX idx_gold_events_source ON gold_events(user_id, source);

CREATE TABLE shop_items (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id),
    name        TEXT NOT NULL,
    price       INTEGER NOT NULL,
    created_at  TEXT NOT NULL,
    archived_at TEXT                             -- archive, never delete (purchase history keeps labels)
);
CREATE INDEX idx_shop_items_user ON shop_items(user_id, archived_at);

-- One-time retroactive grant so the shop is usable on day one: users with
-- existing XP history get gold at the same 10:1 ratio. Fresh databases have no
-- xp_events yet at migration time (seed runs after), so this is a no-op there —
-- Seed() writes its own grant.
INSERT INTO gold_events(user_id, amount, source, label, created_at)
SELECT user_id, MAX(1, SUM(amount) / 10), 'grant', 'Retroactive grant for past XP',
       strftime('%Y-%m-%dT%H:%M:%f', 'now') || '000000Z'
FROM xp_events
GROUP BY user_id
HAVING SUM(amount) > 0;
```

(The `strftime('%f')` fraction is `SS.mmm`; appending `000000Z` produces the fixed-width 9-digit-fraction UTC layout the codebase requires.)

- [ ] **Step 6: Add the models**

Append to `server/internal/models/models.go`:

```go
// GoldEvent is the immutable audit record of a single gold change (mint or spend).
type GoldEvent struct {
	ID         int64     `json:"id"`
	Amount     int64     `json:"amount"` // positive = mint, negative = purchase
	Source     string    `json:"source"` // quest, subtask, tool, journal, purchase, grant
	Label      string    `json:"label,omitempty"`
	ShopItemID *int64    `json:"shop_item_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ShopItem is a self-defined real-life reward purchasable with gold. Items are
// repeatable; archiving (not deleting) removes them from the shop while keeping
// purchase history labels intact.
type ShopItem struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"-"`
	Name       string     `json:"name"`
	Price      int64      `json:"price"`
	CreatedAt  time.Time  `json:"created_at"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

// ShopItemInput is the payload for creating a shop item.
type ShopItemInput struct {
	Name  string `json:"name"`
	Price int64  `json:"price"`
}

// ShopItemPatch is a partial update; nil fields are left untouched.
type ShopItemPatch struct {
	Name  *string `json:"name,omitempty"`
	Price *int64  `json:"price,omitempty"`
}

// PurchaseResult is returned after buying a shop item.
type PurchaseResult struct {
	Item    ShopItem  `json:"item"`
	Event   GoldEvent `json:"event"`
	Balance int64     `json:"balance"` // balance after the purchase
}
```

- [ ] **Step 7: Create the gold store**

Create `server/internal/db/gold_store.go`:

```go
package db

import (
	"database/sql"

	"edi/internal/models"
)

// goldForXP mirrors services.GoldForXP without importing services (avoids an
// import cycle, same trick as levelForXP in store.go). Keep both in sync:
// 1 gold per 10 XP, minimum 1 for any positive award.
func goldForXP(xp int64) int64 {
	if xp <= 0 {
		return 0
	}
	if g := xp / 10; g > 1 {
		return g
	}
	return 1
}

// insertGoldEventTx writes one gold ledger row inside an existing transaction.
// Positive amounts mint, negative amounts spend.
func insertGoldEventTx(tx *sql.Tx, userID, amount int64, source, label string, shopItemID *int64, nowStr string) (int64, error) {
	res, err := tx.Exec(
		`INSERT INTO gold_events(user_id, amount, source, label, shop_item_id, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
		userID, amount, source, label, nullInt64(shopItemID), nowStr)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// GoldBalance computes the spendable balance as SUM(gold_events.amount) — the
// same auditable compute-on-read pattern as attribute XP. Never stored.
func (s *Store) GoldBalance(userID int64) (int64, error) {
	var bal int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM gold_events WHERE user_id = ?`, userID).Scan(&bal)
	return bal, err
}

// ListGoldEvents returns the most recent gold ledger rows (mints and purchases).
func (s *Store) ListGoldEvents(userID int64, limit int) ([]models.GoldEvent, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.Query(
		`SELECT id, amount, source, label, shop_item_id, created_at
		 FROM gold_events WHERE user_id = ? ORDER BY id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.GoldEvent
	for rows.Next() {
		var e models.GoldEvent
		var itemID sql.NullInt64
		var created string
		if err := rows.Scan(&e.ID, &e.Amount, &e.Source, &e.Label, &itemID, &created); err != nil {
			return nil, err
		}
		if itemID.Valid {
			v := itemID.Int64
			e.ShopItemID = &v
		}
		e.CreatedAt = mustParseTime(created)
		out = append(out, e)
	}
	return out, rows.Err()
}
```

- [ ] **Step 8: Seed a gold grant on fresh databases**

In `server/internal/db/seed.go`, immediately after the attributes/xp_events loop (after the closing `}` of `for i, a := range DefaultAttributes {...}`, before the streaks insert), add:

```go
	// Gold grant matching the seeded XP (10:1, same rule as the migration's
	// retroactive grant) so the shop is usable immediately on a fresh install.
	var totalSeedXP int64
	for _, xp := range startingXP {
		totalSeedXP += xp
	}
	if _, err := tx.Exec(
		`INSERT INTO gold_events(user_id, amount, source, label, created_at) VALUES(?, ?, 'grant', 'Starting gold', ?)`,
		userID, goldForXP(totalSeedXP), formatTime(now)); err != nil {
		return err
	}
```

- [ ] **Step 9: Write the failing seed-grant test**

Append to `server/internal/services/service_test.go`:

```go
func TestSeedGoldGrant(t *testing.T) {
	svc := newTestService(t)
	bal, err := svc.GoldBalance()
	if err != nil {
		t.Fatalf("gold balance: %v", err)
	}
	// Seed XP totals 2520 across attributes -> 252 gold at 10:1.
	if bal != 252 {
		t.Errorf("seed gold balance = %d, want 252", bal)
	}
	events, err := svc.ListGoldEvents(10)
	if err != nil {
		t.Fatalf("list gold events: %v", err)
	}
	if len(events) != 1 || events[0].Source != "grant" || events[0].Amount != 252 {
		t.Errorf("expected one grant event of 252, got %+v", events)
	}
}
```

- [ ] **Step 10: Run it to verify it fails**

Run: `cd server && go test ./internal/services -run TestSeedGoldGrant`
Expected: FAIL — `svc.GoldBalance undefined` (compile error).

- [ ] **Step 11: Add the service methods**

In `server/internal/services/service.go`, after the `ListXPEvents` method, add:

```go
// GoldBalance returns the spendable gold balance (SUM of the ledger, computed
// on read — same audit pattern as XP).
func (s *Service) GoldBalance() (int64, error) {
	return s.store.GoldBalance(s.userID)
}

// ListGoldEvents returns the most recent gold ledger rows.
func (s *Service) ListGoldEvents(limit int) ([]models.GoldEvent, error) {
	events, err := s.store.ListGoldEvents(s.userID, limit)
	return orEmpty(events), err
}
```

- [ ] **Step 12: Full backend check — PASS**

Run: `cd server && go build ./... && go vet ./... && go test ./... && gofmt -w .`
Expected: all packages build, all tests pass (including `TestSeedGoldGrant`).

- [ ] **Step 13: Commit**

```bash
git add server/migrations/006_gold.sql server/internal/models/models.go server/internal/db/gold_store.go server/internal/db/seed.go server/internal/services/xp.go server/internal/services/xp_test.go server/internal/services/service.go server/internal/services/service_test.go
git commit -m "feat: gold economy foundation — ledger schema, mint math, retroactive grant"
```

---

### Task 2: Mint gold in every completion path

**Files:**
- Modify: `server/internal/db/store.go` (`CompleteQuest`, `InsertJournal`)
- Modify: `server/internal/db/tool_store.go` (`CompleteTool`)
- Modify: `server/internal/models/models.go` (`Gold` on result structs, `GoldBalance` on `Dashboard`)
- Modify: `server/internal/services/service.go` (`CompleteQuest`, `CreateJournalEntry`, `GetDashboard`)
- Modify: `server/internal/services/tools.go` (`CompleteTool` — adapt to new store signature)
- Test: `server/internal/services/service_test.go`

**Interfaces:**
- Consumes: `goldForXP`, `insertGoldEventTx` from Task 1.
- Produces (signature changes — all callers must be updated in this task):
  - `(*db.Store).CompleteQuest(userID, questID int64) (models.Quest, []models.XPEvent, []models.LevelUp, int64, error)` — new 4th return: gold minted.
  - `(*db.Store).CompleteTool(userID int64, toolKey, toolName string, data []byte, summary string, rewards map[string]int64) (models.ToolEntry, []models.XPEvent, []models.LevelUp, int64, error)`
  - `(*db.Store).InsertJournal(userID int64, in models.JournalInput, dailyRewards map[string]int64) (models.JournalEntry, []models.XPEvent, []models.LevelUp, int64, error)`
  - `models.CompletionResult`, `models.ToolCompletionResult`, `models.JournalCreateResult` each gain `Gold int64 \`json:"gold"\``.
  - `models.Dashboard` gains `GoldBalance int64 \`json:"gold_balance"\``.

- [ ] **Step 1: Write the failing tests**

Append to `server/internal/services/service_test.go`:

```go
func TestCompleteQuestMintsGold(t *testing.T) {
	svc := newTestService(t)
	balBefore, _ := svc.GoldBalance()

	workout := findQuestByTitle(t, svc, "30 minute workout") // {strength:40, discipline:10}
	result, err := svc.CompleteQuest(workout.ID)
	if err != nil {
		t.Fatalf("complete quest: %v", err)
	}
	// 40 XP -> 4g, 10 XP -> 1g. Minted per xp_event.
	if result.Gold != 5 {
		t.Errorf("result.Gold = %d, want 5", result.Gold)
	}
	balAfter, _ := svc.GoldBalance()
	if balAfter != balBefore+5 {
		t.Errorf("balance = %d, want %d", balAfter, balBefore+5)
	}
	if result.Dashboard.GoldBalance != balAfter {
		t.Errorf("dashboard gold_balance = %d, want %d", result.Dashboard.GoldBalance, balAfter)
	}
	events, _ := svc.ListGoldEvents(5)
	if len(events) < 2 || events[0].Source != "quest" || events[1].Source != "quest" {
		t.Errorf("expected two quest mint events, got %+v", events)
	}
}

func TestJournalDailyGoldOnce(t *testing.T) {
	svc := newTestService(t)
	balBefore, _ := svc.GoldBalance()

	first, err := svc.CreateJournalEntry(models.JournalInput{Mood: 7, Energy: 6, Notes: "first"})
	if err != nil {
		t.Fatalf("first entry: %v", err)
	}
	// journalDailyRewards: spirituality 10 -> 1g, discipline 5 -> 1g.
	if first.Gold != 2 {
		t.Errorf("first.Gold = %d, want 2", first.Gold)
	}

	second, err := svc.CreateJournalEntry(models.JournalInput{Mood: 5, Energy: 5, Notes: "second"})
	if err != nil {
		t.Fatalf("second entry: %v", err)
	}
	if second.Gold != 0 {
		t.Errorf("second.Gold = %d, want 0 (daily XP already awarded)", second.Gold)
	}
	balAfter, _ := svc.GoldBalance()
	if balAfter != balBefore+2 {
		t.Errorf("balance = %d, want %d", balAfter, balBefore+2)
	}
}

// TestGoldAuditInvariant checks balance == SUM(gold_events.amount) directly in
// SQL after a mixed sequence, mirroring the XP audit check.
func TestGoldAuditInvariant(t *testing.T) {
	svc := newTestService(t)
	workout := findQuestByTitle(t, svc, "30 minute workout")
	if _, err := svc.CompleteQuest(workout.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := svc.CreateJournalEntry(models.JournalInput{Mood: 6, Energy: 6, Notes: "x"}); err != nil {
		t.Fatalf("journal: %v", err)
	}

	bal, err := svc.GoldBalance()
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	var sum int64
	if err := svc.store.DB().QueryRow(`SELECT COALESCE(SUM(amount),0) FROM gold_events WHERE user_id = 1`).Scan(&sum); err != nil {
		t.Fatalf("sum query: %v", err)
	}
	if bal != sum {
		t.Errorf("audit broken: balance %d != SUM(gold_events) %d", bal, sum)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd server && go test ./internal/services -run 'TestCompleteQuestMintsGold|TestJournalDailyGoldOnce|TestGoldAuditInvariant'`
Expected: FAIL — `result.Gold undefined` etc. (compile errors).

- [ ] **Step 3: Add the model fields**

In `server/internal/models/models.go`:

To `CompletionResult` add (after `LevelUps`):
```go
	Gold      int64     `json:"gold"` // gold minted by this completion
```
To `ToolCompletionResult` add (after `LevelUps`):
```go
	Gold     int64     `json:"gold"`
```
To `JournalCreateResult` add (after `LevelUps`):
```go
	Gold     int64      `json:"gold"`
```
To `Dashboard` add (after `Streak`):
```go
	GoldBalance      int64             `json:"gold_balance"`
```

- [ ] **Step 4: Mint in `store.CompleteQuest`**

In `server/internal/db/store.go`, `CompleteQuest`:

1. Change the signature and ALL its `return` statements to carry a gold total:
```go
func (s *Store) CompleteQuest(userID, questID int64) (models.Quest, []models.XPEvent, []models.LevelUp, int64, error) {
```
Every existing `return models.Quest{}, nil, nil, err` becomes `return models.Quest{}, nil, nil, 0, err` (and the error variants likewise).

2. Extend the `award` struct with a gold source and tag the two award kinds:
```go
	type award struct {
		key    string
		amount int64
		note   string
		src    string // gold_events source: "quest" or "subtask"
	}
	var awards []award
	for _, key := range orderedKeys(rewards) {
		if rewards[key] != 0 {
			awards = append(awards, award{key, rewards[key], title, "quest"})
		}
	}
	for _, st := range doneSubs {
		for _, key := range orderedKeys(st.AttributeRewards) {
			if st.AttributeRewards[key] != 0 {
				awards = append(awards, award{key, st.AttributeRewards[key], title + " · " + st.Title, "subtask"})
			}
		}
	}
```

3. Inside the awards loop, right after the `UPDATE attributes SET total_xp ...` exec succeeds, mint gold (declare `var goldTotal int64` next to `var events []models.XPEvent`):
```go
		if g := goldForXP(a.amount); g > 0 {
			if _, err := insertGoldEventTx(tx, userID, g, a.src, a.note, nil, nowStr); err != nil {
				return models.Quest{}, nil, nil, 0, err
			}
			goldTotal += g
		}
```

4. The two success-path returns at the end become:
```go
	return updated, events, levelUps, goldTotal, nil
```
(and the post-commit `GetQuest` error return: `return models.Quest{}, nil, nil, 0, err`).

- [ ] **Step 5: Mint in `store.CompleteTool`**

In `server/internal/db/tool_store.go`, `CompleteTool`: change the signature to

```go
func (s *Store) CompleteTool(userID int64, toolKey, toolName string, data []byte, summary string, rewards map[string]int64) (models.ToolEntry, []models.XPEvent, []models.LevelUp, int64, error) {
```

update every error return to include `0`, declare `var goldTotal int64` next to `var events []models.XPEvent`, and after the attribute bump inside the rewards loop add:

```go
		if g := goldForXP(amount); g > 0 {
			if _, err := insertGoldEventTx(tx, userID, g, "tool", toolName, nil, nowStr); err != nil {
				return models.ToolEntry{}, nil, nil, 0, err
			}
			goldTotal += g
		}
```

Final return: `..., events, levelUps, goldTotal, nil`.

- [ ] **Step 6: Mint in `store.InsertJournal`**

In `server/internal/db/store.go`, `InsertJournal`: change the signature to

```go
func (s *Store) InsertJournal(userID int64, in models.JournalInput, dailyRewards map[string]int64) (models.JournalEntry, []models.XPEvent, []models.LevelUp, int64, error) {
```

update every error return to include `0`, declare `var goldTotal int64` next to `var events []models.XPEvent`, and inside the first-entry-of-the-day rewards loop, after the attribute bump, add:

```go
			if g := goldForXP(amount); g > 0 {
				if _, err := insertGoldEventTx(tx, userID, g, "journal", "Daily reflection", nil, nowStr); err != nil {
					return models.JournalEntry{}, nil, nil, 0, err
				}
				goldTotal += g
			}
```

Final return: `return entry, events, levelUps, goldTotal, err`.

- [ ] **Step 7: Wire the service layer**

In `server/internal/services/service.go`:

`CompleteQuest` becomes:
```go
	quest, events, levelUps, gold, err := s.store.CompleteQuest(s.userID, id)
```
(error handling unchanged) and the result gains the field:
```go
	return models.CompletionResult{
		Quest:     quest,
		XPEvents:  orEmpty(events),
		LevelUps:  orEmpty(levelUps),
		Gold:      gold,
		Dashboard: dash,
	}, nil
```

`CreateJournalEntry` becomes:
```go
	entry, events, levelUps, gold, err := s.store.InsertJournal(s.userID, in, journalDailyRewards)
	if err != nil {
		return models.JournalCreateResult{}, err
	}
	return models.JournalCreateResult{Entry: entry, XPEvents: orEmpty(events), LevelUps: orEmpty(levelUps), Gold: gold}, nil
```

`GetDashboard`: after the `suggestions` fetch add
```go
	goldBalance, err := s.store.GoldBalance(s.userID)
	if err != nil {
		return models.Dashboard{}, err
	}
```
and set `GoldBalance: goldBalance,` in the returned `models.Dashboard` literal.

In `server/internal/services/tools.go`, find the `s.store.CompleteTool(...)` call, take the new `gold` return value, and set `Gold: gold` in the `models.ToolCompletionResult` literal it builds (mirroring `CompleteQuest`).

- [ ] **Step 8: Fix remaining compile errors, run everything — PASS**

Run: `cd server && go build ./... 2>&1 | head -30` — fix any caller of the three changed store methods you missed (compiler will list them: likely test files). Then:

Run: `cd server && go test ./... -race && go vet ./... && gofmt -w .`
Expected: PASS, including the three new tests and all pre-existing ones (`TestCompleteQuestConcurrentNoDoubleAward` still passes — minting is inside the same gated tx, so a rejected duplicate completion mints nothing).

- [ ] **Step 9: Commit**

```bash
git add server/internal
git commit -m "feat: mint gold on every XP award (quest, subtask, tool, journal) in the same tx"
```

---

### Task 3: Shop — store + service (CRUD + transactional purchase)

**Files:**
- Modify: `server/internal/db/db.go` (add `ErrInsufficientGold` sentinel)
- Create: `server/internal/db/shop_store.go`
- Create: `server/internal/services/shop.go`
- Test: `server/internal/services/shop_test.go`

**Interfaces:**
- Consumes: `models.ShopItem*`, `models.PurchaseResult`, `insertGoldEventTx`, `GoldBalance` (Task 1).
- Produces:
  - `db.ErrInsufficientGold` sentinel
  - `(*db.Store).ListShopItems(userID int64) ([]models.ShopItem, error)` (active only)
  - `(*db.Store).InsertShopItem(userID int64, in models.ShopItemInput) (models.ShopItem, error)`
  - `(*db.Store).UpdateShopItem(userID, id int64, p models.ShopItemPatch) (models.ShopItem, error)` (active only; `db.ErrNotFound` otherwise)
  - `(*db.Store).ArchiveShopItem(userID, id int64) error`
  - `(*db.Store).PurchaseShopItem(userID, itemID int64) (models.PurchaseResult, error)`
  - `(*services.Service).ListShopItems() ([]models.ShopItem, error)`
  - `(*services.Service).CreateShopItem(in models.ShopItemInput) (models.ShopItem, error)`
  - `(*services.Service).UpdateShopItem(id int64, p models.ShopItemPatch) (models.ShopItem, error)`
  - `(*services.Service).ArchiveShopItem(id int64) error`
  - `(*services.Service).PurchaseShopItem(id int64) (models.PurchaseResult, error)`

- [ ] **Step 1: Write the failing tests**

Create `server/internal/services/shop_test.go`:

```go
package services

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"edi/internal/db"
	"edi/internal/models"
)

func TestShopCreateValidation(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.CreateShopItem(models.ShopItemInput{Name: "  ", Price: 10}); !errors.Is(err, ErrValidation) {
		t.Errorf("blank name: got %v, want ErrValidation", err)
	}
	if _, err := svc.CreateShopItem(models.ShopItemInput{Name: "Takeout", Price: 0}); !errors.Is(err, ErrValidation) {
		t.Errorf("zero price: got %v, want ErrValidation", err)
	}
	item, err := svc.CreateShopItem(models.ShopItemInput{Name: " Gaming evening ", Price: 50})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.Name != "Gaming evening" || item.Price != 50 {
		t.Errorf("item = %+v, want trimmed name and price 50", item)
	}
	items, err := svc.ListShopItems()
	if err != nil || len(items) != 1 {
		t.Fatalf("list = %v items, err %v; want 1", len(items), err)
	}
}

func TestShopPurchaseHappyPath(t *testing.T) {
	svc := newTestService(t) // seed balance: 252
	item, _ := svc.CreateShopItem(models.ShopItemInput{Name: "Takeout", Price: 30})
	res, err := svc.PurchaseShopItem(item.ID)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	if res.Balance != 222 {
		t.Errorf("balance after = %d, want 222", res.Balance)
	}
	if res.Event.Amount != -30 || res.Event.Source != "purchase" || res.Event.ShopItemID == nil || *res.Event.ShopItemID != item.ID {
		t.Errorf("event = %+v, want -30 purchase referencing item %d", res.Event, item.ID)
	}
	bal, _ := svc.GoldBalance()
	if bal != 222 {
		t.Errorf("stored balance = %d, want 222", bal)
	}
}

func TestShopPurchaseInsufficientGold(t *testing.T) {
	svc := newTestService(t)
	item, _ := svc.CreateShopItem(models.ShopItemInput{Name: "New keyboard", Price: 999999})
	_, err := svc.PurchaseShopItem(item.ID)
	if !errors.Is(err, ErrValidation) {
		t.Errorf("got %v, want ErrValidation (400)", err)
	}
	if err == nil || !strings.Contains(err.Error(), "gold") {
		t.Errorf("error should mention gold: %v", err)
	}
	bal, _ := svc.GoldBalance()
	if bal != 252 {
		t.Errorf("balance changed to %d on failed purchase, want 252", bal)
	}
}

func TestShopArchivedAndMissing(t *testing.T) {
	svc := newTestService(t)
	item, _ := svc.CreateShopItem(models.ShopItemInput{Name: "Movie night", Price: 20})
	if err := svc.ArchiveShopItem(item.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	items, _ := svc.ListShopItems()
	if len(items) != 0 {
		t.Errorf("archived item still listed: %+v", items)
	}
	if _, err := svc.PurchaseShopItem(item.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("purchase archived: got %v, want ErrNotFound", err)
	}
	if _, err := svc.PurchaseShopItem(99999); !errors.Is(err, ErrNotFound) {
		t.Errorf("purchase missing: got %v, want ErrNotFound", err)
	}
	if _, err := svc.UpdateShopItem(item.ID, models.ShopItemPatch{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update archived: got %v, want ErrNotFound", err)
	}
}

// TestShopPurchaseConcurrentNoOverspend is the gold sibling of
// TestCompleteQuestConcurrentNoDoubleAward: two racing purchases of an item
// priced at the full balance — exactly one may succeed, balance must not go
// negative. Run with -race.
func TestShopPurchaseConcurrentNoOverspend(t *testing.T) {
	svc := newTestService(t)
	bal, _ := svc.GoldBalance() // 252
	item, _ := svc.CreateShopItem(models.ShopItemInput{Name: "Everything", Price: bal})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.PurchaseShopItem(item.ID)
		}(i)
	}
	wg.Wait()

	okCount := 0
	for _, e := range errs {
		if e == nil {
			okCount++
		} else if !errors.Is(e, ErrValidation) {
			t.Errorf("unexpected error kind: %v", e)
		}
	}
	if okCount != 1 {
		t.Errorf("%d purchases succeeded, want exactly 1", okCount)
	}
	after, _ := svc.GoldBalance()
	if after != 0 {
		t.Errorf("balance = %d, want 0 (never negative)", after)
	}
}

// Extend the JSON array contract to the new list endpoints: a user with no
// shop items / gold events must serialize [] not null.
func TestShopListsSerializeAsArrays(t *testing.T) {
	store, err := db.Open(t.TempDir() + "/bare.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	svc := New(store, 1) // no Seed: completely empty user

	items, err := svc.ListShopItems()
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if b, _ := json.Marshal(items); string(b) != "[]" {
		t.Errorf("shop items marshal = %s, want []", b)
	}
	events, err := svc.ListGoldEvents(10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if b, _ := json.Marshal(events); string(b) != "[]" {
		t.Errorf("gold events marshal = %s, want []", b)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd server && go test ./internal/services -run 'TestShop'`
Expected: FAIL — `svc.CreateShopItem undefined` (compile error).

- [ ] **Step 3: Add the sentinel**

In `server/internal/db/db.go`, extend the sentinel `var (...)` block:

```go
	// ErrInsufficientGold — the balance cannot cover the purchase.
	ErrInsufficientGold = errors.New("not enough gold")
```

- [ ] **Step 4: Create the shop store**

Create `server/internal/db/shop_store.go`:

```go
package db

import (
	"database/sql"
	"strings"

	"edi/internal/models"
)

func scanShopItem(scanner interface{ Scan(...any) error }) (models.ShopItem, error) {
	var it models.ShopItem
	var created string
	var archived sql.NullString
	if err := scanner.Scan(&it.ID, &it.UserID, &it.Name, &it.Price, &created, &archived); err != nil {
		return it, err
	}
	it.CreatedAt = mustParseTime(created)
	it.ArchivedAt = parseTimePtr(archived)
	return it, nil
}

const shopColumns = `id, user_id, name, price, created_at, archived_at`

// ListShopItems returns active (non-archived) items, oldest first.
func (s *Store) ListShopItems(userID int64) ([]models.ShopItem, error) {
	rows, err := s.db.Query(`SELECT `+shopColumns+` FROM shop_items WHERE user_id = ? AND archived_at IS NULL ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ShopItem
	for rows.Next() {
		it, err := scanShopItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) GetShopItem(userID, id int64) (models.ShopItem, error) {
	row := s.db.QueryRow(`SELECT `+shopColumns+` FROM shop_items WHERE id = ? AND user_id = ?`, id, userID)
	it, err := scanShopItem(row)
	if err == sql.ErrNoRows {
		return it, ErrNotFound
	}
	return it, err
}

func (s *Store) InsertShopItem(userID int64, in models.ShopItemInput) (models.ShopItem, error) {
	res, err := s.db.Exec(`INSERT INTO shop_items(user_id, name, price, created_at) VALUES(?, ?, ?, ?)`,
		userID, in.Name, in.Price, nowString())
	if err != nil {
		return models.ShopItem{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetShopItem(userID, id)
}

// UpdateShopItem patches an ACTIVE item; archived/missing -> ErrNotFound.
func (s *Store) UpdateShopItem(userID, id int64, p models.ShopItemPatch) (models.ShopItem, error) {
	var sets []string
	var args []any
	if p.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *p.Name)
	}
	if p.Price != nil {
		sets = append(sets, "price = ?")
		args = append(args, *p.Price)
	}
	if len(sets) == 0 {
		// Nothing to change — still 404 if the item isn't active.
		it, err := s.GetShopItem(userID, id)
		if err == nil && it.ArchivedAt != nil {
			return models.ShopItem{}, ErrNotFound
		}
		return it, err
	}
	args = append(args, id, userID)
	res, err := s.db.Exec(`UPDATE shop_items SET `+strings.Join(sets, ", ")+` WHERE id = ? AND user_id = ? AND archived_at IS NULL`, args...)
	if err != nil {
		return models.ShopItem{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.ShopItem{}, ErrNotFound
	}
	return s.GetShopItem(userID, id)
}

// ArchiveShopItem hides an item from the shop; the row (and purchase-history
// labels) stay. Idempotent archiving of an archived item -> ErrNotFound.
func (s *Store) ArchiveShopItem(userID, id int64) error {
	res, err := s.db.Exec(`UPDATE shop_items SET archived_at = ? WHERE id = ? AND user_id = ? AND archived_at IS NULL`,
		nowString(), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PurchaseShopItem spends gold on an active item, atomically: the balance
// check and the negative ledger write happen inside one transaction on the
// single writer connection, so racing purchases serialize and the balance can
// never go negative (gold sibling of the CompleteQuest gate).
func (s *Store) PurchaseShopItem(userID, itemID int64) (models.PurchaseResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.PurchaseResult{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	nowStr := nowString()

	var it models.ShopItem
	var created string
	err = tx.QueryRow(`SELECT id, user_id, name, price, created_at FROM shop_items WHERE id = ? AND user_id = ? AND archived_at IS NULL`,
		itemID, userID).Scan(&it.ID, &it.UserID, &it.Name, &it.Price, &created)
	if err == sql.ErrNoRows {
		return models.PurchaseResult{}, ErrNotFound
	}
	if err != nil {
		return models.PurchaseResult{}, err
	}
	it.CreatedAt = mustParseTime(created)

	var balance int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM gold_events WHERE user_id = ?`, userID).Scan(&balance); err != nil {
		return models.PurchaseResult{}, err
	}
	if balance < it.Price {
		return models.PurchaseResult{}, ErrInsufficientGold
	}

	evID, err := insertGoldEventTx(tx, userID, -it.Price, "purchase", it.Name, &it.ID, nowStr)
	if err != nil {
		return models.PurchaseResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.PurchaseResult{}, err
	}

	itemID2 := it.ID
	return models.PurchaseResult{
		Item: it,
		Event: models.GoldEvent{
			ID: evID, Amount: -it.Price, Source: "purchase", Label: it.Name,
			ShopItemID: &itemID2, CreatedAt: mustParseTime(nowStr),
		},
		Balance: balance - it.Price,
	}, nil
}
```

- [ ] **Step 5: Create the shop service**

Create `server/internal/services/shop.go`:

```go
package services

import (
	"errors"
	"strings"

	"edi/internal/db"
	"edi/internal/models"
)

// The reward shop: self-defined real-life rewards purchasable with gold.
// Items are repeatable; big one-time rewards are simply archived after buying.

func validateShopFields(name string, price int64) error {
	if strings.TrimSpace(name) == "" {
		return validationErr("name is required")
	}
	if price <= 0 {
		return validationErr("price must be greater than 0")
	}
	return nil
}

// ListShopItems returns the active reward-shop items.
func (s *Service) ListShopItems() ([]models.ShopItem, error) {
	items, err := s.store.ListShopItems(s.userID)
	return orEmpty(items), err
}

// CreateShopItem validates and adds a reward to the shop.
func (s *Service) CreateShopItem(in models.ShopItemInput) (models.ShopItem, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := validateShopFields(in.Name, in.Price); err != nil {
		return models.ShopItem{}, err
	}
	return s.store.InsertShopItem(s.userID, in)
}

// UpdateShopItem applies a partial patch to an active item.
func (s *Service) UpdateShopItem(id int64, p models.ShopItemPatch) (models.ShopItem, error) {
	if p.Name != nil {
		trimmed := strings.TrimSpace(*p.Name)
		if trimmed == "" {
			return models.ShopItem{}, validationErr("name is required")
		}
		p.Name = &trimmed
	}
	if p.Price != nil && *p.Price <= 0 {
		return models.ShopItem{}, validationErr("price must be greater than 0")
	}
	item, err := s.store.UpdateShopItem(s.userID, id, p)
	if errors.Is(err, db.ErrNotFound) {
		return models.ShopItem{}, ErrNotFound
	}
	return item, err
}

// ArchiveShopItem removes an item from the shop (history keeps its label).
func (s *Service) ArchiveShopItem(id int64) error {
	err := s.store.ArchiveShopItem(s.userID, id)
	if errors.Is(err, db.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// PurchaseShopItem spends gold on an item. Insufficient balance is a client
// condition (400), not a server error.
func (s *Service) PurchaseShopItem(id int64) (models.PurchaseResult, error) {
	res, err := s.store.PurchaseShopItem(s.userID, id)
	switch {
	case errors.Is(err, db.ErrNotFound):
		return models.PurchaseResult{}, ErrNotFound
	case errors.Is(err, db.ErrInsufficientGold):
		return models.PurchaseResult{}, validationErr("not enough gold")
	}
	return res, err
}
```

- [ ] **Step 6: Run all tests with race detector — PASS**

Run: `cd server && go test ./... -race && go vet ./... && gofmt -w .`
Expected: PASS including all five `TestShop*` tests.

- [ ] **Step 7: Commit**

```bash
git add server/internal
git commit -m "feat: reward shop — CRUD + transactional gold purchase (never overspends)"
```

---

### Task 4: HTTP endpoints + agent tools

**Files:**
- Modify: `server/internal/handlers/handlers.go`
- Modify: `server/internal/handlers/router.go`
- Modify: `server/internal/agent/agent.go`

**Interfaces:**
- Consumes: all `(*services.Service)` shop/gold methods from Tasks 1–3; existing `writeJSON`, `writeError`, `decodeBody`, `pathID`, `queryInt` handler helpers; existing `add(...)`, `decode`, `decodeID`, `idSchema` agent helpers.
- Produces routes:
  - `GET /api/shop` → `[]ShopItem` · `POST /api/shop` → 201 `ShopItem`
  - `PATCH /api/shop/{id}` → `ShopItem` · `POST /api/shop/{id}/archive` → `{"archived":true}`
  - `POST /api/shop/{id}/purchase` → `PurchaseResult` · `GET /api/gold/events` → `[]GoldEvent`
- Produces agent tools: `list_shop_items`, `create_shop_item`, `update_shop_item`, `archive_shop_item`, `purchase_shop_item`, `list_gold_events` (auto-exposed via MCP).

- [ ] **Step 1: Add the handlers**

Append to `server/internal/handlers/handlers.go` (after the tools section):

```go
// --- gold economy / reward shop ----------------------------------------------

func (h *Handlers) listShop(w http.ResponseWriter, _ *http.Request) {
	items, err := h.svc.ListShopItems()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) createShopItem(w http.ResponseWriter, r *http.Request) {
	var in models.ShopItemInput
	if err := decodeBody(r, &in); err != nil {
		writeError(w, err)
		return
	}
	item, err := h.svc.CreateShopItem(in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handlers) updateShopItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var patch models.ShopItemPatch
	if err := decodeBody(r, &patch); err != nil {
		writeError(w, err)
		return
	}
	item, err := h.svc.UpdateShopItem(id, patch)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handlers) archiveShopItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.ArchiveShopItem(id); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"archived": true})
}

func (h *Handlers) purchaseShopItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := h.svc.PurchaseShopItem(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) listGoldEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.svc.ListGoldEvents(queryInt(r, "limit", 30))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}
```

- [ ] **Step 2: Register the routes**

In `server/internal/handlers/router.go`, after the journal block, add:

```go
	// Gold economy — reward shop + auditable gold ledger.
	mux.HandleFunc("GET /api/shop", h.listShop)
	mux.HandleFunc("POST /api/shop", h.createShopItem)
	mux.HandleFunc("PATCH /api/shop/{id}", h.updateShopItem)
	mux.HandleFunc("POST /api/shop/{id}/archive", h.archiveShopItem)
	mux.HandleFunc("POST /api/shop/{id}/purchase", h.purchaseShopItem)
	mux.HandleFunc("GET /api/gold/events", h.listGoldEvents)
```

- [ ] **Step 3: Add the agent tools**

In `server/internal/agent/agent.go`, before the `for i, t := range r.tools` index loop, add:

```go
	add("list_shop_items", "List the reward shop: self-defined real-life rewards purchasable with gold.",
		emptySchema, func(json.RawMessage) (any, error) { return svc.ListShopItems() })

	add("create_shop_item", "Add a reward to the shop (a real-life indulgence with a gold price).",
		`{"type":"object","required":["name","price"],"properties":{"name":{"type":"string"},"price":{"type":"integer","minimum":1}}}`,
		func(in json.RawMessage) (any, error) {
			var p models.ShopItemInput
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			return svc.CreateShopItem(p)
		})

	add("update_shop_item", "Update the name or price of an active shop item.",
		`{"type":"object","required":["item_id"],"properties":{"item_id":{"type":"integer"},"name":{"type":"string"},"price":{"type":"integer","minimum":1}}}`,
		func(in json.RawMessage) (any, error) {
			id, err := decodeID(in, "item_id")
			if err != nil {
				return nil, err
			}
			var p models.ShopItemPatch
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			return svc.UpdateShopItem(id, p)
		})

	add("archive_shop_item", "Archive a shop item so it no longer appears in the shop (purchase history keeps its label).",
		idSchema("item_id"),
		func(in json.RawMessage) (any, error) {
			id, err := decodeID(in, "item_id")
			if err != nil {
				return nil, err
			}
			if err := svc.ArchiveShopItem(id); err != nil {
				return nil, err
			}
			return map[string]bool{"archived": true}, nil
		})

	add("purchase_shop_item", "Spend gold to buy a reward from the shop. Fails with a validation error if the balance is too low.",
		idSchema("item_id"),
		func(in json.RawMessage) (any, error) {
			id, err := decodeID(in, "item_id")
			if err != nil {
				return nil, err
			}
			return svc.PurchaseShopItem(id)
		})

	add("list_gold_events", "List recent gold ledger entries (mints and purchases). The balance is SUM(amount) and also appears on the dashboard as gold_balance.",
		`{"type":"object","properties":{"limit":{"type":"integer"}}}`,
		func(in json.RawMessage) (any, error) {
			var p struct {
				Limit int `json:"limit"`
			}
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			return svc.ListGoldEvents(p.Limit)
		})
```

Note: `update_shop_item` decodes `item_id` alongside patch fields at the top level — same pattern as `update_quest`.

- [ ] **Step 4: Build + test**

Run: `cd server && go build ./... && go vet ./... && go test ./... && gofmt -w .`
Expected: PASS.

- [ ] **Step 5: Live API validation (curl + sqlite3)**

Start the backend: `make backend` (background). Then:

```bash
curl -s localhost:8080/api/shop                       # expect: [] (or existing items)
curl -s -X POST localhost:8080/api/shop -d '{"name":"Gaming evening","price":50}'   # expect: 201 item JSON
curl -s localhost:8080/api/dashboard | python3 -c "import sys,json; print(json.load(sys.stdin)['gold_balance'])"   # expect: a positive number (retroactive grant)
ITEM_ID=$(curl -s localhost:8080/api/shop | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['id'])")
curl -s -X POST localhost:8080/api/shop/$ITEM_ID/purchase                            # expect: 200 with negative event + reduced balance
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8080/api/shop/99999/purchase   # expect: 404
curl -s -X POST localhost:8080/api/shop -d '{"name":"Too rich","price":9999999}'
RICH_ID=$(curl -s localhost:8080/api/shop | python3 -c "import sys,json; print([i['id'] for i in json.load(sys.stdin) if i['name']=='Too rich'][0])")
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8080/api/shop/$RICH_ID/purchase  # expect: 400
curl -s localhost:8080/api/agent/tools | python3 -c "import sys,json; print([t['name'] for t in json.load(sys.stdin)['tools'] if 'shop' in t['name'] or 'gold' in t['name']])"  # expect the 6 new tools
```

Confirm side effects in the DB:

```bash
sqlite3 server/edi.db "SELECT source, amount, label FROM gold_events ORDER BY id DESC LIMIT 5;"
sqlite3 server/edi.db "SELECT (SELECT COALESCE(SUM(amount),0) FROM gold_events WHERE user_id=1);"
```
The SUM must equal the `gold_balance` the dashboard reported after the purchase. Also complete a quest via curl and confirm a `source='quest'` gold row lands:

```bash
QID=$(curl -s 'localhost:8080/api/quests?status=active' | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['id'])")
curl -s -X POST localhost:8080/api/quests/$QID/complete | python3 -c "import sys,json; print(json.load(sys.stdin)['gold'])"   # expect: > 0
sqlite3 server/edi.db "SELECT source, amount FROM gold_events ORDER BY id DESC LIMIT 2;"
```

Stop the backend when done.

- [ ] **Step 6: Commit**

```bash
git add server/internal
git commit -m "feat: shop + gold HTTP endpoints and agent tools (MCP picks them up automatically)"
```

---

### Task 5: apiclient + CLI

**Files:**
- Modify: `server/internal/apiclient/client.go`
- Modify: `server/cmd/edi-cli/main.go`

**Interfaces:**
- Consumes: routes from Task 4, `models.ShopItem`, `models.ShopItemInput`, `models.PurchaseResult`, `models.GoldEvent`.
- Produces:
  - `(*apiclient.Client).ListShopItems() ([]models.ShopItem, error)`
  - `(*apiclient.Client).CreateShopItem(in models.ShopItemInput) (models.ShopItem, error)`
  - `(*apiclient.Client).PurchaseShopItem(id int64) (models.PurchaseResult, error)`
  - `(*apiclient.Client).ListGoldEvents(limit int) ([]models.GoldEvent, error)`
  - CLI commands: `shop`, `shop-add --name N --price P`, `buy <id>`, `gold`

- [ ] **Step 1: Add apiclient methods**

Append to `server/internal/apiclient/client.go` (before the agent tool surface section):

```go
// --- gold economy / reward shop ----------------------------------------------

func (c *Client) ListShopItems() ([]models.ShopItem, error) {
	var out []models.ShopItem
	err := c.do(http.MethodGet, "/api/shop", nil, &out)
	return out, err
}

func (c *Client) CreateShopItem(in models.ShopItemInput) (models.ShopItem, error) {
	var it models.ShopItem
	err := c.do(http.MethodPost, "/api/shop", in, &it)
	return it, err
}

func (c *Client) PurchaseShopItem(id int64) (models.PurchaseResult, error) {
	var r models.PurchaseResult
	err := c.do(http.MethodPost, fmt.Sprintf("/api/shop/%d/purchase", id), nil, &r)
	return r, err
}

func (c *Client) ListGoldEvents(limit int) ([]models.GoldEvent, error) {
	var out []models.GoldEvent
	err := c.do(http.MethodGet, fmt.Sprintf("/api/gold/events?limit=%d", limit), nil, &out)
	return out, err
}
```

- [ ] **Step 2: Add CLI commands**

In `server/cmd/edi-cli/main.go`:

1. Extend the usage doc comment's command list:
```
//	shop                            List reward shop items
//	shop-add --name N --price P     Add a reward to the shop
//	buy <id>                        Purchase a shop item (spends gold)
//	gold                            Gold balance + recent ledger
```

2. Add cases to the `run` switch:
```go
	case "shop":
		return cmdShop(c)
	case "shop-add":
		return cmdShopAdd(c, args)
	case "buy":
		return cmdBuy(c, args)
	case "gold":
		return cmdGold(c)
```

3. Add the command functions (match the file's existing print style; `red(...)` helper already exists — check for a green/plain style used by `cmdComplete` and mirror it):
```go
func cmdShop(c *apiclient.Client) error {
	items, err := c.ListShopItems()
	if err != nil {
		return err
	}
	dash, err := c.Dashboard()
	if err != nil {
		return err
	}
	fmt.Printf("Gold: %dg\n\n", dash.GoldBalance)
	if len(items) == 0 {
		fmt.Println("The shop is empty. Add rewards with: edi-cli shop-add --name \"Gaming evening\" --price 50")
		return nil
	}
	for _, it := range items {
		fmt.Printf("  [%d] %-40s %6dg\n", it.ID, it.Name, it.Price)
	}
	return nil
}

func cmdShopAdd(c *apiclient.Client, args []string) error {
	fs := flag.NewFlagSet("shop-add", flag.ExitOnError)
	name := fs.String("name", "", "reward name (required)")
	price := fs.Int64("price", 0, "gold price (required, > 0)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	it, err := c.CreateShopItem(models.ShopItemInput{Name: *name, Price: *price})
	if err != nil {
		return err
	}
	fmt.Printf("Added [%d] %s — %dg\n", it.ID, it.Name, it.Price)
	return nil
}

func cmdBuy(c *apiclient.Client, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: buy <item-id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid id %q", args[0])
	}
	res, err := c.PurchaseShopItem(id)
	if err != nil {
		return err
	}
	fmt.Printf("Purchased %q for %dg. Balance: %dg. Enjoy it — you earned it.\n", res.Item.Name, res.Item.Price, res.Balance)
	return nil
}

func cmdGold(c *apiclient.Client) error {
	dash, err := c.Dashboard()
	if err != nil {
		return err
	}
	fmt.Printf("Gold: %dg\n\nRecent ledger:\n", dash.GoldBalance)
	events, err := c.ListGoldEvents(15)
	if err != nil {
		return err
	}
	for _, e := range events {
		sign := "+"
		if e.Amount < 0 {
			sign = ""
		}
		fmt.Printf("  %s%dg  %-9s %s\n", sign, e.Amount, e.Source, e.Label)
	}
	return nil
}
```
Also add `usage()` lines for the new commands if `usage()` enumerates commands explicitly.

- [ ] **Step 3: Build + validate against the live server**

```bash
cd server && go build ./... && go vet ./... && gofmt -w .
make backend   # background
cd server && go run ./cmd/edi-cli gold        # expect balance + grant line
go run ./cmd/edi-cli shop-add --name "Coffee out" --price 15
go run ./cmd/edi-cli shop                     # expect the item listed with balance
go run ./cmd/edi-cli buy <printed id>         # expect purchase confirmation, reduced balance
```
Stop the backend. Inspect the actual output — do not assume.

- [ ] **Step 4: Commit**

```bash
git add server/internal/apiclient server/cmd/edi-cli
git commit -m "feat: gold + shop in apiclient and edi-cli (shop, shop-add, buy, gold)"
```

---

### Task 6: Client data layer (types, api, queries)

**Files:**
- Modify: `client/src/lib/types.ts`
- Modify: `client/src/lib/api.ts`
- Modify: `client/src/lib/queries.ts`

**Interfaces:**
- Consumes: Task 4 routes.
- Produces (used by Tasks 7–8): `ShopItem`, `ShopItemInput`, `GoldEvent`, `PurchaseResult` types; `Dashboard.gold_balance`; `gold` on the three result types; hooks `useShopItems()`, `useGoldEvents(limit?)`, `useCreateShopItem()`, `useUpdateShopItem()`, `useArchiveShopItem()`, `usePurchaseShopItem()`.

- [ ] **Step 1: types.ts**

Add after the `Streak` interface:

```ts
export interface ShopItem {
  id: number;
  name: string;
  price: number;
  created_at: string;
  archived_at?: string;
}

export interface ShopItemInput {
  name: string;
  price: number;
}

export interface GoldEvent {
  id: number;
  amount: number; // positive = mint, negative = purchase
  source: string; // quest, subtask, tool, journal, purchase, grant
  label?: string;
  shop_item_id?: number;
  created_at: string;
}

export interface PurchaseResult {
  item: ShopItem;
  event: GoldEvent;
  balance: number;
}
```

And extend existing interfaces:
- `Dashboard`: add `gold_balance: number;` (after `streak`).
- `CompletionResult`, `ToolCompletionResult`, `JournalCreateResult`: add `gold: number;` (after `level_ups`).

- [ ] **Step 2: api.ts**

Add `GoldEvent, PurchaseResult, ShopItem, ShopItemInput` to the type import. Append inside the `api` object (after the journal block):

```ts
  listShop: () => request<ShopItem[]>("/shop"),
  createShopItem: (input: ShopItemInput) =>
    request<ShopItem>("/shop", { method: "POST", body: JSON.stringify(input) }),
  updateShopItem: (id: number, patch: { name?: string; price?: number }) =>
    request<ShopItem>(`/shop/${id}`, { method: "PATCH", body: JSON.stringify(patch) }),
  archiveShopItem: (id: number) =>
    request<{ archived: boolean }>(`/shop/${id}/archive`, { method: "POST" }),
  purchaseShopItem: (id: number) =>
    request<PurchaseResult>(`/shop/${id}/purchase`, { method: "POST" }),
  listGoldEvents: (limit = 30) => request<GoldEvent[]>(`/gold/events?limit=${limit}`),
```

- [ ] **Step 3: queries.ts**

Add to `keys`:
```ts
  shop: ["shop"] as const,
  goldEvents: ["gold-events"] as const,
```

Add `qc.invalidateQueries({ queryKey: ["gold-events"] });` inside `useInvalidateAll` (completions mint gold; the dashboard invalidation already refreshes the balance).

Add `ShopItemInput` to the type import. Append at the end of the file:

```ts
// --- gold economy / reward shop ----------------------------------------------

export function useShopItems() {
  return useQuery({ queryKey: keys.shop, queryFn: api.listShop });
}

export function useGoldEvents(limit = 30) {
  return useQuery({ queryKey: [...keys.goldEvents, limit], queryFn: () => api.listGoldEvents(limit) });
}

export function useCreateShopItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ShopItemInput) => api.createShopItem(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.shop }),
  });
}

export function useUpdateShopItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }: { id: number; patch: { name?: string; price?: number } }) =>
      api.updateShopItem(id, patch),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.shop }),
  });
}

export function useArchiveShopItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.archiveShopItem(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.shop }),
  });
}

export function usePurchaseShopItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.purchaseShopItem(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["dashboard"] });
      qc.invalidateQueries({ queryKey: ["gold-events"] });
    },
  });
}
```

(The global `MutationCache.onError` toast covers purchase failures — e.g. "not enough gold" — for free.)

- [ ] **Step 4: Build**

Run: `cd client && npm run build`
Expected: clean `tsc` + Vite build. (`useUpdateShopItem` is exported but unused until Task 7 — module exports don't trip `noUnusedLocals`; if the build complains anyway, fold that hook into Task 7 instead.)

- [ ] **Step 5: Commit**

```bash
git add client/src/lib
git commit -m "feat(client): gold + shop types, API calls, and query hooks"
```

---

### Task 7: Shop page + navigation

**Files:**
- Create: `client/src/pages/Shop.tsx`
- Modify: `client/src/App.tsx`

**Interfaces:**
- Consumes: Task 6 hooks, `Btn`/`EmptyState`/`SectionTitle`/`Spinner` from `components/ui`, `pushToast` from `lib/toast`, `relativeTime` from `lib/format`.
- Produces: `ShopPage` component; `"shop"` view in `App.tsx`.

- [ ] **Step 1: Create the Shop page**

Create `client/src/pages/Shop.tsx`:

```tsx
import { useState } from "react";
import { Archive, Coins, Plus, ShoppingCart } from "lucide-react";
import {
  useArchiveShopItem,
  useCreateShopItem,
  useDashboard,
  useGoldEvents,
  usePurchaseShopItem,
  useShopItems,
} from "../lib/queries";
import { pushToast } from "../lib/toast";
import { Btn, EmptyState, SectionTitle, Spinner } from "../components/ui";
import { relativeTime } from "../lib/format";
import type { ShopItem } from "../lib/types";

function ItemRow({ item, balance }: { item: ShopItem; balance: number }) {
  const purchase = usePurchaseShopItem();
  const archive = useArchiveShopItem();
  const [arming, setArming] = useState(false);
  const affordable = balance >= item.price;

  const buy = () => {
    if (!arming) {
      setArming(true);
      window.setTimeout(() => setArming(false), 3000);
      return;
    }
    setArming(false);
    purchase.mutate(item.id, {
      onSuccess: (res) =>
        pushToast(`Purchased "${res.item.name}" for ${res.item.price}g — enjoy it, you earned it.`, "success"),
    });
  };

  return (
    <div className="hud-panel flex items-center gap-3 p-3.5" data-testid={`shop-item-${item.id}`}>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium text-ink">{item.name}</div>
        <div className="tabnum text-xs" style={{ color: "var(--color-gold)" }}>
          {item.price}g
        </div>
      </div>
      <Btn
        variant={affordable ? "primary" : "ghost"}
        disabled={!affordable || purchase.isPending}
        onClick={buy}
        data-testid={`buy-${item.id}`}
      >
        <ShoppingCart size={14} />
        {arming ? "Confirm?" : affordable ? "Buy" : "Too costly"}
      </Btn>
      <button
        onClick={() => archive.mutate(item.id)}
        className="text-faint transition-colors hover:text-ink"
        aria-label={`Archive ${item.name}`}
        title="Archive (remove from shop)"
      >
        <Archive size={16} />
      </button>
    </div>
  );
}

function AddItemForm() {
  const create = useCreateShopItem();
  const [name, setName] = useState("");
  const [price, setPrice] = useState("");

  const submit = () => {
    const p = parseInt(price, 10);
    if (!name.trim() || !p || p <= 0) {
      pushToast("A reward needs a name and a price above 0.", "error");
      return;
    }
    create.mutate(
      { name: name.trim(), price: p },
      {
        onSuccess: () => {
          setName("");
          setPrice("");
        },
      },
    );
  };

  return (
    <div className="hud-panel flex flex-col gap-2 p-3.5 sm:flex-row sm:items-center">
      <input
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder='Reward, e.g. "Guilt-free gaming evening"'
        className="flex-1 rounded-md border border-edge bg-transparent px-3 py-2 text-sm text-ink outline-none placeholder:text-faint"
        data-testid="shop-name"
      />
      <input
        value={price}
        onChange={(e) => setPrice(e.target.value.replace(/\D/g, ""))}
        placeholder="Price (g)"
        inputMode="numeric"
        className="w-full rounded-md border border-edge bg-transparent px-3 py-2 text-sm text-ink outline-none placeholder:text-faint sm:w-28"
        data-testid="shop-price"
      />
      <Btn variant="primary" onClick={submit} disabled={create.isPending} data-testid="shop-add">
        <Plus size={14} />
        Add
      </Btn>
    </div>
  );
}

export function ShopPage() {
  const dashboard = useDashboard();
  const items = useShopItems();
  const ledger = useGoldEvents(30);

  if (dashboard.isLoading || items.isLoading) return <Spinner label="Opening the shop…" />;
  if (dashboard.isError || items.isError || !dashboard.data || !items.data) {
    return (
      <EmptyState
        title="Couldn't reach the backend"
        hint={((dashboard.error ?? items.error) as Error)?.message ?? "Is the Go server running on :8080?"}
      />
    );
  }

  const balance = dashboard.data.gold_balance;
  const purchases = (ledger.data ?? []).filter((e) => e.source === "purchase");

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-display text-xl font-bold tracking-tight text-ink">Reward Shop</h1>
          <p className="text-sm text-faint">XP is progress. Gold is permission — spend it guilt-free.</p>
        </div>
        <div className="flex items-center gap-2" data-testid="gold-balance">
          <Coins size={20} style={{ color: "var(--color-gold)" }} />
          <span className="tabnum text-2xl font-bold" style={{ color: "var(--color-goldhi)" }}>
            {balance}g
          </span>
        </div>
      </div>

      <AddItemForm />

      <section>
        <SectionTitle hint="Define your own rewards; buy them with earned gold.">Wares</SectionTitle>
        {items.data.length === 0 ? (
          <EmptyState
            icon={<ShoppingCart size={20} />}
            title="The shop is empty"
            hint="Add real-life rewards above — a takeout night, a lazy morning, that gadget."
          />
        ) : (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {items.data.map((it) => (
              <ItemRow key={it.id} item={it} balance={balance} />
            ))}
          </div>
        )}
      </section>

      <section>
        <SectionTitle hint="Every purchase is a ledger entry — same audit trail as XP.">
          Purchase history
        </SectionTitle>
        {purchases.length === 0 ? (
          <EmptyState title="No purchases yet" hint="Earn gold by completing quests, then treat yourself." />
        ) : (
          <div className="space-y-1.5">
            {purchases.map((e) => (
              <div key={e.id} className="flex items-center justify-between rounded-md border border-edge px-3 py-2 text-sm">
                <span className="text-ink">{e.label}</span>
                <span className="flex items-center gap-3">
                  <span className="tabnum" style={{ color: "var(--color-gold)" }}>
                    {e.amount}g
                  </span>
                  <span className="text-xs text-faint">{relativeTime(e.created_at)}</span>
                </span>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
```

Adjust `Btn` prop usage to its actual signature in `components/ui.tsx` if it differs (e.g., if `Btn` doesn't forward `disabled`/`data-testid`, extend it there minimally). Check whether `relativeTime` exists in `lib/format` (Journal imports it) — it does.

- [ ] **Step 2: Wire into App.tsx**

In `client/src/App.tsx`:
1. Import: `import { ShopPage } from "./pages/Shop";` and add `Store` to the lucide import.
2. Extend the type: `type View = "dashboard" | "quests" | "shop" | "moodlog" | "journal" | "agent";`
3. Desktop nav: after `{topItem("quests", "Quests", ScrollText)}` add `{topItem("shop", "Shop", Store)}`.
4. Main content: add `{view === "shop" && <ShopPage />}` after the quests line.
5. Mobile bottom nav array: add `{ id: "shop", label: "Shop", Icon: Store },` after the quests entry.

- [ ] **Step 3: Build**

Run: `cd client && npm run build`
Expected: clean build, no unused-symbol errors.

- [ ] **Step 4: Commit**

```bash
git add client/src
git commit -m "feat(client): Reward Shop page — balance, wares, buy with confirm, purchase history"
```

---

### Task 8: Gold feedback — reward overlay, header, celebrate call sites

**Files:**
- Modify: `client/src/lib/reward.tsx`
- Modify: `client/src/components/CharacterHeader.tsx`
- Modify: `client/src/pages/Dashboard.tsx`
- Modify: `client/src/pages/Quests.tsx`
- Modify: `client/src/pages/Journal.tsx`
- Modify: `client/src/components/DailyMoodLog.tsx`

**Interfaces:**
- Consumes: `gold` field on the three completion result types (Task 6), `gold_balance` on `Dashboard`.
- Produces: `RewardPayload.gold?: number`; `CharacterHeader` gains a required `gold: number` prop.

- [ ] **Step 1: Extend RewardPayload + overlay**

In `client/src/lib/reward.tsx`:
1. Add `Coins` to the lucide import.
2. Add to `RewardPayload`:
```ts
  gold?: number; // gold minted alongside the XP
```
3. In `RewardOverlay`, directly under the `+{totalXP} XP` motion.div, add:
```tsx
            {typeof result.gold === "number" && result.gold > 0 && (
              <motion.div
                className="mt-1.5 flex items-center justify-center gap-1.5 text-base font-semibold"
                style={{ color: "var(--color-gold)" }}
                initial={{ opacity: 0, y: 6 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.2 }}
              >
                <Coins size={16} />
                +{result.gold} gold
              </motion.div>
            )}
```

- [ ] **Step 2: Pass gold at every celebrate call site**

Add `gold: res.gold,` to the `celebrate({...})` payloads in:
- `client/src/pages/Quests.tsx` (`handleComplete`)
- `client/src/pages/Dashboard.tsx` (`handleComplete`)
- `client/src/pages/Journal.tsx` (journal-create onSuccess)
- `client/src/components/DailyMoodLog.tsx` (tool-complete onSuccess)

- [ ] **Step 3: Show the balance in CharacterHeader**

In `client/src/components/CharacterHeader.tsx`:
1. Add `Coins` to the lucide import.
2. Extend the props:
```tsx
export function CharacterHeader({
  character,
  streak,
  daily,
  gold,
}: {
  character: CharacterSummary;
  streak: Streak;
  daily: DailyProgress;
  gold: number;
}) {
```
3. In the streak+daily cluster (the `flex items-center gap-5 border-t ...` div), add as the FIRST child:
```tsx
          <div className="text-center" data-testid="header-gold">
            <div className="flex items-center justify-center gap-1.5">
              <Coins size={20} style={{ color: "var(--color-gold)" }} />
              <span className="tabnum text-2xl font-bold text-ink">{gold}</span>
            </div>
            <div className="mt-0.5 font-display text-[10px] uppercase tracking-wider text-faint">
              Gold
            </div>
          </div>
```
4. In `client/src/pages/Dashboard.tsx`, update the usage:
```tsx
      <CharacterHeader character={data.character} streak={data.streak} daily={data.daily_progress} gold={data.gold_balance} />
```

- [ ] **Step 4: Build**

Run: `cd client && npm run build`
Expected: clean build.

- [ ] **Step 5: Commit**

```bash
git add client/src
git commit -m "feat(client): gold in the celebration overlay and character header"
```

---

### Task 9: End-to-end validation, docs, wrap-up

**Files:**
- Modify: `CLAUDE.md` (gold invariant)
- Modify: `README.md` (feature mention, if it lists features)

- [ ] **Step 1: Full automated pass**

```bash
cd server && go build ./... && go vet ./... && go test ./... -race && gofmt -l .
cd ../client && npm run build
```
Expected: everything green; `gofmt -l` prints nothing.

- [ ] **Step 2: Browser validation (agent-browser skill / Playwright)**

Start `make dev`. In a real browser session:
1. Load the app — header shows the Gold stat; console has no errors/warnings.
2. Navigate to Shop — balance renders; add an item ("Test reward", 10g); it appears without reload.
3. Click Buy → button arms ("Confirm?") → click again → success toast, balance drops by 10 in BOTH the shop header and (navigate back) the dashboard header, purchase appears in history.
4. Complete a quest from the dashboard — celebration overlay shows `+N gold` under the XP.
5. Create a shop item priced above the balance — Buy is disabled ("Too costly").
6. Re-check the console: still clean.

- [ ] **Step 3: DB audit check**

```bash
sqlite3 server/edi.db "SELECT (SELECT COALESCE(SUM(amount),0) FROM gold_events WHERE user_id=1) AS gold_sum;"
curl -s localhost:8080/api/dashboard | python3 -c "import sys,json; print(json.load(sys.stdin)['gold_balance'])"
```
The two numbers must match. Also re-run the XP audit query from CLAUDE.md to confirm nothing regressed.

- [ ] **Step 4: Document the invariant**

In `CLAUDE.md`, "Invariants you must not break", add after the XP-audit bullet:

```markdown
- **Gold is auditable the same way.** The balance is always
  `SUM(gold_events.amount)` computed on read — there is no stored balance
  column. Minting (1g per 10 XP, min 1, `services.GoldForXP` mirrored by
  `db.goldForXP`) happens inside the SAME tx as the xp_event; purchases check
  the balance inside the purchase tx so it can never go negative (regression
  tests: `TestGoldAuditInvariant`, `TestShopPurchaseConcurrentNoOverspend`).
```

If `README.md` has a feature list, add one line about the gold economy + reward shop.

- [ ] **Step 5: Final commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: gold economy invariant + feature notes"
```

---

## Self-review notes

- Spec coverage: minting rule (T1/T2), gold_events + shop_items schema + backfill (T1), audit invariant + no stored balance (T1/T2/T9), service methods + handlers + agent tools (T3/T4), apiclient/CLI (T5), frontend shop page / header / overlay (T6–T8), tests incl. concurrency + array contract (T2/T3), live curl + sqlite + browser validation (T4/T9).
- Type consistency: store completion methods all return `(..., int64, error)` with gold before error; `GoldEvent.Label` (not Note); agent tool id field is `item_id`; frontend hook names match between Tasks 6 and 7.
- Known judgment calls the executor may adapt: exact `Btn` props in `ui.tsx`, exact print helpers in `edi-cli`, exact literal position of `GoldBalance` in the Dashboard struct.
