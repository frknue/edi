# Decay & Stakes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Neglected attributes bleed real XP (lazily applied, auditable, floored at peak−2 levels), escapable via a 30-gold Maintenance Ward or a free-but-loud rest mode.

**Architecture:** No daemon. A single `ApplyDecay` catch-up runs inside attribute-touching service methods, billing owed idle days as negative `xp_events` (source `decay`) in one idempotent transaction — same tx discipline as quest completion and shop purchase. A new audited `peak_xp` column anchors the floor. Wards are paid through the existing gold ledger (`gold_events`, source `ward`). Rest mode lives in `app_settings`. Spec: `docs/superpowers/specs/2026-07-14-decay-stakes-design.md`.

**Tech Stack:** Go 1.x + modernc.org/sqlite (no CGO), net/http stdlib mux, React + TS + TanStack Query + Tailwind v4.

## Global Constraints

- Run all Go commands from `server/` (module `edi`); repo root is `/Users/furkanulker/git/private/edi`.
- Decay rules (exact): idle clock = local days since latest of (last positive xp_event for the attribute, rest_ended_at). Grace = idle days 1–3. From idle day 4: each idle day costs `max(5, floor(total_xp/100))` XP using the running total as days apply sequentially. Floor = `XPForLevel(LevelForXP(peak_xp) - 2)`; a partial final bleed down to exactly the floor is allowed; never below the floor, never below 0.
- Exclusions: no decay while rest mode is on; idle days covered by a ward window are not billed; ward does NOT reset the idle clock, rest end DOES.
- Idempotent per attribute per local day, enforced INSIDE the tx (billed dates are encoded in the event note `decay · YYYY-MM-DD`).
- Decay never mints/removes gold (`GoldForXP(<=0)=0` already guarantees this — do not add gold logic to decay).
- Ward: flat 30 gold, 7 days, extends from current expiry when re-warded; paid via negative `gold_events` (source `ward`, label `Maintenance Ward · <attribute name>`) inside the same tx as the ward insert, balance checked in-tx.
- XP audit invariant unchanged: every `total_xp` change pairs with an `xp_events` row in the same tx. `peak_xp` never decreases and is maintained in the same tx as awards.
- Error mapping: insufficient gold → `services.ErrValidation` (400); unknown attribute → `services.ErrNotFound` (404).
- Every service-returned list wraps in `orEmpty(...)`; timestamps in SQL use the existing fixed-width helpers (`nowString()`, `formatTime`).
- SQLite single writer (`SetMaxOpenConns(1)`) — decay/ward txs rely on it, keep it.
- After every Go change: `cd server && go build ./... && go vet ./... && go test ./... && gofmt -w .` (use `-race` where a step says so). After every client change: `cd client && npm run build`.
- Commit after every task (messages given per task).

---

### Task 1: Migration, models, decay math

**Files:**
- Create: `server/migrations/007_decay.sql`
- Create: `server/internal/services/decay.go`
- Modify: `server/internal/services/xp_test.go` (append math tests)
- Modify: `server/internal/models/models.go` (append Ward/decay/rest types; extend `Attribute`, `Dashboard`)
- Modify: `server/internal/db/store.go` (add `xpForLevel` mirror next to `levelForXP`)
- Create: `server/internal/db/decay_math.go` (private mirrors)

**Interfaces:**
- Consumes: existing `LevelForXP`, `XPForLevel` (services/xp.go), `levelForXP` (db/store.go).
- Produces (later tasks rely on these exact names):
  - `services.DecayGraceDays = 3`, `services.DecayMinPerDay int64 = 5`, `services.WardCostGold int64 = 30`, `services.WardDays = 7`
  - `services.DailyDecay(totalXP int64) int64`
  - `services.DecayFloor(peakXP int64) int64`
  - db private: `decayDailyAmount(totalXP int64) int64`, `decayFloorXP(peakXP int64) int64`, `xpForLevel(level int) int64`
  - `models.Ward{ID int64, AttributeKey string, ExpiresAt, CreatedAt time.Time}`
  - `models.WardResult{Ward Ward, Balance int64}`
  - `models.RestState{On bool, Since *time.Time}`
  - `models.AttributeDecay{State string, IdleDays int, WardedUntil *time.Time, ProjectedDailyLoss int64, FloorLevel int}`
  - `models.Attribute` gains `PeakXP int64 \`json:"-"\`` and `Decay *AttributeDecay \`json:"decay,omitempty"\``
  - `models.Dashboard` gains `RestMode bool \`json:"rest_mode"\``, `RestSince *time.Time \`json:"rest_since,omitempty"\``, `DecayedToday int64 \`json:"decayed_today"\``

- [ ] **Step 1: Write the failing math tests**

Append to `server/internal/services/xp_test.go`:

```go
func TestDailyDecay(t *testing.T) {
	cases := []struct {
		total int64
		want  int64
	}{
		{-10, 0}, {0, 0}, {1, 1}, {3, 3}, {4, 4}, {5, 5}, {6, 5}, {100, 5},
		{499, 5}, {500, 5}, {600, 6}, {2520, 25}, {10000, 100},
	}
	for _, c := range cases {
		if got := DailyDecay(c.total); got != c.want {
			t.Errorf("DailyDecay(%d) = %d, want %d", c.total, got, c.want)
		}
	}
}

func TestDecayFloor(t *testing.T) {
	cases := []struct {
		peak int64
		want int64
	}{
		{0, 0},    // peak level 1 -> floor level -1 -> clamp 0
		{99, 0},   // level 1
		{100, 0},  // level 2 -> floor level 0 -> clamp 0
		{400, 0},  // level 3 -> floor level 1 -> 0 XP
		{900, 100},  // level 4 -> floor level 2 -> 100 XP
		{1600, 400}, // level 5 -> floor level 3 -> 400 XP
	}
	for _, c := range cases {
		if got := DecayFloor(c.peak); got != c.want {
			t.Errorf("DecayFloor(%d) = %d, want %d", c.peak, got, c.want)
		}
	}
}
```

(DailyDecay caps at the total itself so a 3-XP attribute loses at most 3 — the floor logic in the engine clamps further.)

- [ ] **Step 2: Run to verify failure**

Run: `cd server && go test ./internal/services -run 'TestDailyDecay|TestDecayFloor'`
Expected: FAIL — `undefined: DailyDecay` (compile error).

- [ ] **Step 3: Implement the math**

Create `server/internal/services/decay.go`:

```go
package services

// Decay & stakes: neglected attributes bleed XP. Pure math lives here (like
// the level formula in xp.go); db/decay_math.go keeps private mirrors to
// avoid an import cycle — change both together.

const (
	// DecayGraceDays is how many idle days cost nothing.
	DecayGraceDays = 3
	// DecayMinPerDay is the minimum XP one billable idle day costs.
	DecayMinPerDay int64 = 5
	// WardCostGold is the flat gold price of a 7-day Maintenance Ward.
	WardCostGold int64 = 30
	// WardDays is how long one ward purchase shields an attribute.
	WardDays = 7
)

// DailyDecay returns the XP one billable idle day costs an attribute with
// the given current total: 1% of the total, minimum DecayMinPerDay, never
// more than the total itself. Non-positive totals cost nothing.
func DailyDecay(totalXP int64) int64 {
	if totalXP <= 0 {
		return 0
	}
	d := totalXP / 100
	if d < DecayMinPerDay {
		d = DecayMinPerDay
	}
	if d > totalXP {
		d = totalXP
	}
	return d
}

// DecayFloor returns the XP an attribute can never decay below: the start of
// (peak level - 2). Peaks at level <=3 floor at 0.
func DecayFloor(peakXP int64) int64 {
	return XPForLevel(LevelForXP(peakXP) - 2)
}
```

(`XPForLevel` already clamps levels < 1 to level 1 → 0 XP.)

- [ ] **Step 4: Run the tests — PASS**

Run: `cd server && go test ./internal/services -run 'TestDailyDecay|TestDecayFloor'`
Expected: PASS.

- [ ] **Step 5: Create the migration**

Create `server/migrations/007_decay.sql`:

```sql
-- 007_decay.sql — decay & stakes. Neglected attributes bleed XP as negative
-- xp_events (source='decay'), floored at the XP threshold of (peak level - 2).
-- peak_xp is maintained in the same tx as every award (stored-but-auditable,
-- like total_xp: it must equal the running max of cumulative event sums).
-- Wards are gold-bought decay shields; windows matter historically (days
-- covered by a lapsed ward are still excluded from billing), so rows are
-- never deleted.

ALTER TABLE attributes ADD COLUMN peak_xp INTEGER NOT NULL DEFAULT 0;

-- Decay never existed before this migration, so current totals are the peaks.
UPDATE attributes SET peak_xp = total_xp;

CREATE TABLE wards (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id),
    attribute_key TEXT NOT NULL,
    expires_at    TEXT NOT NULL,
    created_at    TEXT NOT NULL
);
CREATE INDEX idx_wards_user_attr ON wards(user_id, attribute_key, expires_at);
```

- [ ] **Step 6: Add the models**

Append to `server/internal/models/models.go`:

```go
// Ward is a gold-bought decay shield for one attribute. Rows are never
// deleted: lapsed windows still exclude the days they covered from billing.
type Ward struct {
	ID           int64     `json:"id"`
	AttributeKey string    `json:"attribute_key"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// WardResult is returned after buying a ward.
type WardResult struct {
	Ward    Ward  `json:"ward"`
	Balance int64 `json:"balance"` // gold balance after the purchase
}

// RestState reports whether decay is paused (vacation/sick mode).
type RestState struct {
	On    bool       `json:"on"`
	Since *time.Time `json:"since,omitempty"`
}

// AttributeDecay describes an attribute's decay state, computed on read.
// State is one of: fresh (active today), grace (idle 1-3 days), decaying
// (idle beyond grace), warded (shielded by an active ward), rest (rest mode).
type AttributeDecay struct {
	State              string     `json:"state"`
	IdleDays           int        `json:"idle_days"`
	WardedUntil        *time.Time `json:"warded_until,omitempty"`
	ProjectedDailyLoss int64      `json:"projected_daily_loss"` // 0 unless decaying
	FloorLevel         int        `json:"floor_level"`
}
```

In the `Attribute` struct, after `TotalXP`, add:

```go
	PeakXP  int64  `json:"-"` // highest total_xp ever reached; anchors the decay floor
```

and after the derived-fields comment block's `Progress` field, add:

```go
	Decay *AttributeDecay `json:"decay,omitempty"` // computed on read
```

In the `Dashboard` struct, after `GoldBalance`, add:

```go
	RestMode     bool       `json:"rest_mode"`
	RestSince    *time.Time `json:"rest_since,omitempty"`
	DecayedToday int64      `json:"decayed_today"` // XP removed by this request's decay catch-up
```

- [ ] **Step 7: Add the db mirrors**

Create `server/internal/db/decay_math.go`:

```go
package db

// Private mirrors of services.DailyDecay / services.DecayFloor (and the
// xpForLevel inverse), duplicated to avoid a db->services import cycle —
// the same trick as levelForXP/goldForXP. Keep in sync with
// services/decay.go and services/xp.go.

const (
	decayGraceDays        = 3
	decayMinPerDay  int64 = 5
)

func decayDailyAmount(totalXP int64) int64 {
	if totalXP <= 0 {
		return 0
	}
	d := totalXP / 100
	if d < decayMinPerDay {
		d = decayMinPerDay
	}
	if d > totalXP {
		d = totalXP
	}
	return d
}

func decayFloorXP(peakXP int64) int64 {
	return xpForLevel(levelForXP(peakXP) - 2)
}
```

Append to `server/internal/db/store.go` (next to `levelForXP`):

```go
// xpForLevel mirrors services.XPForLevel (kept in sync, avoids the cycle).
func xpForLevel(level int) int64 {
	if level < 1 {
		level = 1
	}
	l := int64(level - 1)
	return l * l * 100
}
```

- [ ] **Step 8: Full backend check + commit**

Run: `cd server && go build ./... && go vet ./... && go test ./... && gofmt -w .`
Expected: PASS (new column defaults keep existing queries working).

```bash
git add server/migrations/007_decay.sql server/internal/services/decay.go server/internal/services/xp_test.go server/internal/models/models.go server/internal/db/decay_math.go server/internal/db/store.go
git commit -m "feat: decay foundation — peak_xp + wards schema, decay math, models"
```

---

### Task 2: peak_xp maintenance in every award path

**Files:**
- Modify: `server/internal/db/store.go` (CompleteQuest awards loop ~line 399, InsertJournal rewards loop ~line 615, `ListAttributes` select ~line 57)
- Modify: `server/internal/db/tool_store.go` (CompleteTool rewards loop ~line 60)
- Modify: `server/internal/db/seed.go` (seed inserts set peak_xp)
- Test: `server/internal/services/service_test.go`

**Interfaces:**
- Consumes: `attributes.peak_xp` column (Task 1).
- Produces: every XP award updates `peak_xp = MAX(peak_xp, total_xp + amount)` in the same tx; `Store.ListAttributes` scans `PeakXP`; decay (Task 5) reads `PeakXP` for the floor.

- [ ] **Step 1: Write the failing test**

Append to `server/internal/services/service_test.go`:

```go
func TestPeakXPMaintainedOnAward(t *testing.T) {
	svc := newTestService(t)
	before, _ := svc.ListAttributes()
	strBefore := attrByKey(before, "strength") // seed: 520 total, 520 peak

	if strBefore.PeakXP != strBefore.TotalXP {
		t.Fatalf("seed peak %d != total %d", strBefore.PeakXP, strBefore.TotalXP)
	}

	workout := findQuestByTitle(t, svc, "30 minute workout") // {strength:40, discipline:10}
	if _, err := svc.CompleteQuest(workout.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	after, _ := svc.ListAttributes()
	strAfter := attrByKey(after, "strength")
	if strAfter.PeakXP != strBefore.TotalXP+40 {
		t.Errorf("peak = %d, want %d (raised with the award)", strAfter.PeakXP, strBefore.TotalXP+40)
	}

	// peak_xp is running-max auditable: it never decreases.
	var peak int64
	if err := svc.store.DB().QueryRow(`SELECT peak_xp FROM attributes WHERE user_id=1 AND key='strength'`).Scan(&peak); err != nil {
		t.Fatalf("peak query: %v", err)
	}
	if peak != strAfter.PeakXP {
		t.Errorf("stored peak %d != listed peak %d", peak, strAfter.PeakXP)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd server && go test ./internal/services -run TestPeakXPMaintainedOnAward`
Expected: FAIL — `strBefore.PeakXP` is 0 (column defaulted, never scanned/maintained). (If it fails on compile instead, that's also a valid RED.)

- [ ] **Step 3: Maintain peak in the three award paths**

In `server/internal/db/store.go` CompleteQuest awards loop, replace

```go
		if _, err := tx.Exec(`UPDATE attributes SET total_xp = total_xp + ? WHERE user_id = ? AND key = ?`, a.amount, userID, a.key); err != nil {
```

with

```go
		if _, err := tx.Exec(`UPDATE attributes SET total_xp = total_xp + ?, peak_xp = MAX(peak_xp, total_xp + ?) WHERE user_id = ? AND key = ?`, a.amount, a.amount, userID, a.key); err != nil {
```

In `store.go` InsertJournal rewards loop, replace

```go
			if _, err := tx.Exec(`UPDATE attributes SET total_xp = total_xp + ? WHERE user_id = ? AND key = ?`, amount, userID, key); err != nil {
```

with

```go
			if _, err := tx.Exec(`UPDATE attributes SET total_xp = total_xp + ?, peak_xp = MAX(peak_xp, total_xp + ?) WHERE user_id = ? AND key = ?`, amount, amount, userID, key); err != nil {
```

In `server/internal/db/tool_store.go` CompleteTool rewards loop, make the identical replacement (same SQL, variables `amount, amount, userID, key`).

(Decay in Task 5 uses plain `total_xp = total_xp - ?` — peak intentionally untouched there.)

- [ ] **Step 4: Scan peak in ListAttributes and seed it**

In `store.go` `ListAttributes`, change the query and scan:

```go
	rows, err := s.db.Query(`SELECT id, user_id, key, name, total_xp, peak_xp FROM attributes WHERE user_id = ? ORDER BY id`, userID)
```
```go
		if err := rows.Scan(&a.ID, &a.UserID, &a.Key, &a.Name, &a.TotalXP, &a.PeakXP); err != nil {
```

In `server/internal/db/seed.go`, change the attribute insert to include peak:

```go
		if _, err := tx.Exec(`INSERT INTO attributes(user_id, key, name, total_xp, peak_xp, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
			userID, a.Key, a.Name, xp, xp, formatTime(now.AddDate(0, 0, -10))); err != nil {
```

- [ ] **Step 5: Run — PASS, then full suite + commit**

Run: `cd server && go test ./internal/services -run TestPeakXPMaintainedOnAward` → PASS.
Run: `cd server && go build ./... && go vet ./... && go test ./... -race && gofmt -w .` → PASS.

```bash
git add server/internal/db server/internal/services/service_test.go
git commit -m "feat: maintain peak_xp in every award tx (decay floor anchor)"
```

---

### Task 3: Maintenance Ward — store + service

**Files:**
- Create: `server/internal/db/ward_store.go`
- Create: `server/internal/services/ward.go`
- Test: `server/internal/services/decay_test.go` (new file, ward tests)

**Interfaces:**
- Consumes: `models.Ward`, `models.WardResult`, `insertGoldEventTx`, `db.ErrInsufficientGold`, `db.ErrNotFound`, `services.WardCostGold`, `services.WardDays`.
- Produces:
  - `(*db.Store).CreateWard(userID int64, attrKey, attrName string, cost int64, days int) (models.Ward, int64, error)` — returns ward + balance after
  - `(*db.Store).ListWards(userID int64, attrKey string) ([]models.Ward, error)` — all windows, oldest first
  - `(*db.Store).ActiveWardExpiry(userID int64, attrKey string, now time.Time) (*time.Time, error)`
  - `(*services.Service).WardAttribute(key string) (models.WardResult, error)`

- [ ] **Step 1: Write the failing tests**

Create `server/internal/services/decay_test.go`:

```go
package services

import (
	"errors"
	"testing"
	"time"
)

func TestWardAttribute(t *testing.T) {
	svc := newTestService(t) // seed balance: 252 gold
	res, err := svc.WardAttribute("strength")
	if err != nil {
		t.Fatalf("ward: %v", err)
	}
	if res.Balance != 252-WardCostGold {
		t.Errorf("balance = %d, want %d", res.Balance, 252-WardCostGold)
	}
	wantExpiry := time.Now().Add(time.Duration(WardDays) * 24 * time.Hour)
	if diff := res.Ward.ExpiresAt.Sub(wantExpiry); diff < -time.Minute || diff > time.Minute {
		t.Errorf("expiry %v not ~7 days out", res.Ward.ExpiresAt)
	}

	// Re-warding extends from the CURRENT expiry (stacking).
	res2, err := svc.WardAttribute("strength")
	if err != nil {
		t.Fatalf("re-ward: %v", err)
	}
	if got := res2.Ward.ExpiresAt.Sub(res.Ward.ExpiresAt); got < 7*24*time.Hour-time.Minute || got > 7*24*time.Hour+time.Minute {
		t.Errorf("stacked expiry extends %v, want ~7 days beyond the first", got)
	}

	// Gold ledger has two 'ward' spends with the attribute name in the label.
	events, _ := svc.ListGoldEvents(10, "ward")
	if len(events) != 2 || events[0].Amount != -WardCostGold || events[0].Label != "Maintenance Ward · Strength" {
		t.Errorf("ward gold events = %+v", events)
	}
}

func TestWardErrors(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.WardAttribute("nonsense"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown attribute: got %v, want ErrNotFound", err)
	}
	// Drain the balance: 252 gold buys 8 wards (240), 9th must fail.
	for i := 0; i < 8; i++ {
		if _, err := svc.WardAttribute("focus"); err != nil {
			t.Fatalf("ward %d: %v", i, err)
		}
	}
	if _, err := svc.WardAttribute("focus"); !errors.Is(err, ErrValidation) {
		t.Errorf("insufficient gold: got %v, want ErrValidation", err)
	}
	bal, _ := svc.GoldBalance()
	if bal != 252-8*WardCostGold {
		t.Errorf("balance = %d, want %d (failed ward must not charge)", bal, 252-8*WardCostGold)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd server && go test ./internal/services -run 'TestWard'`
Expected: FAIL — `svc.WardAttribute undefined` (compile error).

- [ ] **Step 3: Create the ward store**

Create `server/internal/db/ward_store.go`:

```go
package db

import (
	"database/sql"
	"time"

	"edi/internal/models"
)

func scanWard(scanner interface{ Scan(...any) error }) (models.Ward, error) {
	var w models.Ward
	var expires, created string
	if err := scanner.Scan(&w.ID, &w.AttributeKey, &expires, &created); err != nil {
		return w, err
	}
	w.ExpiresAt = mustParseTime(expires)
	w.CreatedAt = mustParseTime(created)
	return w, nil
}

// ListWards returns every ward window for an attribute, oldest first. Lapsed
// windows still matter: the decay engine excludes the days they covered.
func (s *Store) ListWards(userID int64, attrKey string) ([]models.Ward, error) {
	rows, err := s.db.Query(
		`SELECT id, attribute_key, expires_at, created_at FROM wards
		 WHERE user_id = ? AND attribute_key = ? ORDER BY id`, userID, attrKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Ward
	for rows.Next() {
		w, err := scanWard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ActiveWardExpiry returns the latest future expiry for an attribute, or nil.
func (s *Store) ActiveWardExpiry(userID int64, attrKey string, now time.Time) (*time.Time, error) {
	var expires sql.NullString
	err := s.db.QueryRow(
		`SELECT MAX(expires_at) FROM wards WHERE user_id = ? AND attribute_key = ? AND expires_at > ?`,
		userID, attrKey, formatTime(now)).Scan(&expires)
	if err != nil {
		return nil, err
	}
	return parseTimePtr(expires), nil
}

// CreateWard buys decay protection for one attribute: balance check, gold
// spend (source 'ward'), and the ward insert happen in ONE tx on the single
// writer connection — the same never-overspend discipline as shop purchases.
// A still-active ward extends from its current expiry (stacking).
func (s *Store) CreateWard(userID int64, attrKey, attrName string, cost int64, days int) (models.Ward, int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.Ward{}, 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	nowStr := formatTime(now)

	var balance int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM gold_events WHERE user_id = ?`, userID).Scan(&balance); err != nil {
		return models.Ward{}, 0, err
	}
	if balance < cost {
		return models.Ward{}, 0, ErrInsufficientGold
	}

	// Extend from the current active expiry when one exists.
	base := now
	var current sql.NullString
	if err := tx.QueryRow(
		`SELECT MAX(expires_at) FROM wards WHERE user_id = ? AND attribute_key = ? AND expires_at > ?`,
		userID, attrKey, nowStr).Scan(&current); err != nil {
		return models.Ward{}, 0, err
	}
	if cur := parseTimePtr(current); cur != nil {
		base = *cur
	}
	expires := base.Add(time.Duration(days) * 24 * time.Hour)

	res, err := tx.Exec(`INSERT INTO wards(user_id, attribute_key, expires_at, created_at) VALUES(?, ?, ?, ?)`,
		userID, attrKey, formatTime(expires), nowStr)
	if err != nil {
		return models.Ward{}, 0, err
	}
	id, _ := res.LastInsertId()

	if _, err := insertGoldEventTx(tx, userID, -cost, "ward", "Maintenance Ward · "+attrName, nil, nowStr); err != nil {
		return models.Ward{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return models.Ward{}, 0, err
	}
	return models.Ward{ID: id, AttributeKey: attrKey, ExpiresAt: expires, CreatedAt: now}, balance - cost, nil
}
```

- [ ] **Step 4: Create the ward service**

Create `server/internal/services/ward.go`:

```go
package services

import (
	"errors"

	"edi/internal/db"
	"edi/internal/models"
)

// WardAttribute buys a Maintenance Ward: WardCostGold gold shields one
// attribute from decay for WardDays days (extends a still-active ward).
func (s *Service) WardAttribute(key string) (models.WardResult, error) {
	names, err := s.store.AttributeNames(s.userID)
	if err != nil {
		return models.WardResult{}, err
	}
	name, ok := names[key]
	if !ok {
		return models.WardResult{}, ErrNotFound
	}
	ward, balance, err := s.store.CreateWard(s.userID, key, name, WardCostGold, WardDays)
	switch {
	case errors.Is(err, db.ErrInsufficientGold):
		return models.WardResult{}, validationErr("not enough gold for a ward (%dg)", WardCostGold)
	case err != nil:
		return models.WardResult{}, err
	}
	return models.WardResult{Ward: ward, Balance: balance}, nil
}
```

- [ ] **Step 5: Run — PASS, full suite, commit**

Run: `cd server && go test ./internal/services -run 'TestWard'` → PASS.
Run: `cd server && go build ./... && go vet ./... && go test ./... && gofmt -w .` → PASS.

```bash
git add server/internal/db/ward_store.go server/internal/services/ward.go server/internal/services/decay_test.go
git commit -m "feat: Maintenance Ward — gold-bought decay shield (transactional, stacking)"
```

---

### Task 4: Rest mode

**Files:**
- Create: `server/internal/services/rest.go`
- Test: `server/internal/services/decay_test.go` (append)

**Interfaces:**
- Consumes: `(*db.Store).GetSetting(userID, key)`, `SetSetting(userID, key, value)` (openai_store.go), `models.RestState`.
- Produces:
  - `(*services.Service).SetRestMode(on bool) (models.RestState, error)`
  - `(*services.Service).RestState() (models.RestState, error)`
  - private `(*services.Service).restEndedAt() (*time.Time, error)` — consumed by decay (Task 5)
  - Settings keys (exact): `rest_mode` ("1" when on, "" when off), `rest_since` (RFC3339), `rest_ended_at` (RFC3339)

- [ ] **Step 1: Write the failing test**

Append to `server/internal/services/decay_test.go`:

```go
func TestRestMode(t *testing.T) {
	svc := newTestService(t)

	st, err := svc.RestState()
	if err != nil || st.On {
		t.Fatalf("initial rest state = %+v, %v; want off", st, err)
	}

	on, err := svc.SetRestMode(true)
	if err != nil || !on.On || on.Since == nil {
		t.Fatalf("rest on = %+v, %v; want on with since", on, err)
	}

	off, err := svc.SetRestMode(false)
	if err != nil || off.On {
		t.Fatalf("rest off = %+v, %v", off, err)
	}
	ended, err := svc.restEndedAt()
	if err != nil || ended == nil {
		t.Fatalf("rest_ended_at = %v, %v; want a timestamp", ended, err)
	}
	if time.Since(*ended) > time.Minute {
		t.Errorf("rest_ended_at %v not recent", *ended)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd server && go test ./internal/services -run TestRestMode`
Expected: FAIL — `svc.RestState undefined` (compile error).

- [ ] **Step 3: Implement rest mode**

Create `server/internal/services/rest.go`:

```go
package services

import (
	"time"

	"edi/internal/models"
)

// Rest mode pauses ALL decay (vacation / sick weeks). It is free but loud:
// the dashboard shows a banner while it is on. Turning it off restarts every
// attribute's idle clock from that moment (see the decay engine).
const (
	settingRestMode    = "rest_mode"    // "1" on, "" off
	settingRestSince   = "rest_since"   // RFC3339
	settingRestEndedAt = "rest_ended_at" // RFC3339, written when turned off
)

// SetRestMode turns rest mode on or off and returns the new state.
func (s *Service) SetRestMode(on bool) (models.RestState, error) {
	if on {
		if err := s.store.SetSetting(s.userID, settingRestMode, "1"); err != nil {
			return models.RestState{}, err
		}
		if err := s.store.SetSetting(s.userID, settingRestSince, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return models.RestState{}, err
		}
	} else {
		if err := s.store.SetSetting(s.userID, settingRestMode, ""); err != nil {
			return models.RestState{}, err
		}
		if err := s.store.SetSetting(s.userID, settingRestEndedAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return models.RestState{}, err
		}
	}
	return s.RestState()
}

// RestState reports the current rest mode.
func (s *Service) RestState() (models.RestState, error) {
	mode, err := s.store.GetSetting(s.userID, settingRestMode)
	if err != nil {
		return models.RestState{}, err
	}
	st := models.RestState{On: mode == "1"}
	if st.On {
		if raw, err := s.store.GetSetting(s.userID, settingRestSince); err == nil && raw != "" {
			if t, err := time.Parse(time.RFC3339, raw); err == nil {
				st.Since = &t
			}
		}
	}
	return st, nil
}

// restEndedAt returns when rest mode was last turned off (nil if never).
// The decay engine treats it as an idle-clock reset point.
func (s *Service) restEndedAt() (*time.Time, error) {
	raw, err := s.store.GetSetting(s.userID, settingRestEndedAt)
	if err != nil || raw == "" {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, nil // unreadable timestamp: treat as never
	}
	return &t, nil
}
```

(Check `GetSetting`'s behavior for a missing key in `db/openai_store.go` — it returns `"", nil` on `sql.ErrNoRows`; if it doesn't, translate that here.)

- [ ] **Step 4: Run — PASS, full suite, commit**

Run: `cd server && go test ./internal/services -run TestRestMode` → PASS, then the full check.

```bash
git add server/internal/services/rest.go server/internal/services/decay_test.go
git commit -m "feat: rest mode — pause decay via app_settings, idle clocks restart on wake"
```

---

### Task 5: The decay engine (lazy catch-up)

**Files:**
- Create: `server/internal/db/decay_store.go`
- Modify: `server/internal/services/decay.go` (service ApplyDecay + wiring)
- Modify: `server/internal/services/service.go` (wire into GetDashboard, ListAttributes, CompleteQuest, CreateJournalEntry)
- Modify: `server/internal/services/tools.go` (wire into CompleteTool)
- Test: `server/internal/services/decay_test.go` (append)

**Interfaces:**
- Consumes: `decayDailyAmount`, `decayFloorXP`, `decayGraceDays` (Task 1), wards table (Task 3), `restEndedAt()` (Task 4), `PeakXP` scanning (Task 2).
- Produces:
  - `(*db.Store).ApplyDecay(userID int64, restEndedAt *time.Time, now time.Time) (int64, error)` — total XP removed
  - `(*services.Service).ApplyDecay() (int64, error)` — no-op (0) while rest mode is on
  - Decay xp_events: `source='decay'`, `amount` negative, `note` = `decay · YYYY-MM-DD` (the billed local date), `source_id` NULL

- [ ] **Step 1: Write the failing tests**

Append to `server/internal/services/decay_test.go` (also add `"fmt"`, `"strings"`, `"sync"` to the file's imports as needed):

```go
// backdateAttribute makes an attribute look idle by shifting all its positive
// xp_events into the past. Test-only trick; the engine only reads times.
func backdateAttribute(t *testing.T, svc *Service, key string, daysAgo int) {
	t.Helper()
	ts := time.Now().UTC().AddDate(0, 0, -daysAgo).Format("2006-01-02T15:04:05.000000000Z07:00")
	if _, err := svc.store.DB().Exec(
		`UPDATE xp_events SET created_at = ? WHERE user_id = 1 AND attribute_key = ? AND amount > 0`, ts, key); err != nil {
		t.Fatalf("backdate: %v", err)
	}
}

func TestDecayCatchUp(t *testing.T) {
	svc := newTestService(t)
	backdateAttribute(t, svc, "strength", 10) // 520 XP, 10 idle days

	removed, err := svc.ApplyDecay()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Idle days 4..10 = 7 billable days at max(5, 520/100)=5 each = 35.
	if removed != 35 {
		t.Errorf("removed = %d, want 35", removed)
	}
	attrs, _ := svc.ListAttributes()
	if got := attrByKey(attrs, "strength").TotalXP; got != 485 {
		t.Errorf("strength total = %d, want 485", got)
	}
	// Idempotent: a second catch-up the same day removes nothing.
	again, err := svc.ApplyDecay()
	if err != nil || again != 0 {
		t.Errorf("second apply removed %d (err %v), want 0", again, err)
	}
	// Audit invariant intact: total == SUM(events).
	var sum int64
	svc.store.DB().QueryRow(`SELECT COALESCE(SUM(amount),0) FROM xp_events WHERE user_id=1 AND attribute_key='strength'`).Scan(&sum)
	if sum != 485 {
		t.Errorf("event sum = %d, want 485", sum)
	}
	// Peak untouched by decay.
	if got := attrByKey(attrs, "strength").PeakXP; got != 520 {
		t.Errorf("peak = %d, want 520", got)
	}
}

func TestDecayGraceAndFloor(t *testing.T) {
	svc := newTestService(t)

	// 3 idle days = still in grace: nothing happens.
	backdateAttribute(t, svc, "focus", 3)
	if removed, _ := svc.ApplyDecay(); removed != 0 {
		t.Errorf("grace period removed %d, want 0", removed)
	}

	// Floor: spirituality has 60 XP (peak level 1 -> floor 0), min-5 bleed,
	// long idleness must stop at 0, never negative.
	backdateAttribute(t, svc, "spirituality", 60)
	if _, err := svc.ApplyDecay(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	attrs, _ := svc.ListAttributes()
	if got := attrByKey(attrs, "spirituality").TotalXP; got < 0 {
		t.Errorf("spirituality total = %d, went below 0", got)
	}
}

func TestDecaySkipsRest(t *testing.T) {
	svc := newTestService(t)

	// Rest mode on: no decay at all.
	backdateAttribute(t, svc, "wealth", 10)
	if _, err := svc.SetRestMode(true); err != nil {
		t.Fatalf("rest on: %v", err)
	}
	if removed, _ := svc.ApplyDecay(); removed != 0 {
		t.Errorf("decay ran during rest: removed %d", removed)
	}
	// Rest off: EVERY idle clock restarts — still no decay right after.
	if _, err := svc.SetRestMode(false); err != nil {
		t.Fatalf("rest off: %v", err)
	}
	if removed, _ := svc.ApplyDecay(); removed != 0 {
		t.Errorf("decay after rest-off removed %d, want 0 (clock restarted)", removed)
	}
}

// Separate service from the rest test: turning rest off resets ALL idle
// clocks, which would zero out the ward scenario below.
func TestDecayWardExcludesCoveredDays(t *testing.T) {
	svc := newTestService(t)

	// A fresh ward covers today, so today's bill is excluded, but the
	// uncovered past days still bill. learning: 300 XP, idle 6 days -> days
	// 4,5,6 billable; day 6 (today) covered by ward -> 2 days * 5 = 10.
	if _, err := svc.WardAttribute("learning"); err != nil {
		t.Fatalf("ward: %v", err)
	}
	backdateAttribute(t, svc, "learning", 6)
	removed, err := svc.ApplyDecay()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if removed != 10 {
		t.Errorf("warded decay removed %d, want 10 (today excluded by ward)", removed)
	}
}

func TestDecayConcurrentSingleApplication(t *testing.T) {
	svc := newTestService(t)
	backdateAttribute(t, svc, "creativity", 8) // 170 XP: days 4..8 = 5 days * 5 = 25

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.ApplyDecay()
		}()
	}
	wg.Wait()

	var n int
	svc.store.DB().QueryRow(`SELECT COUNT(1) FROM xp_events WHERE user_id=1 AND attribute_key='creativity' AND source='decay'`).Scan(&n)
	if n != 5 {
		t.Errorf("decay events = %d, want exactly 5 (no double billing)", n)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd server && go test ./internal/services -run 'TestDecay'`
Expected: FAIL — `svc.ApplyDecay undefined` (compile error).

- [ ] **Step 3: Implement the engine**

Create `server/internal/db/decay_store.go`:

```go
package db

import (
	"fmt"
	"strings"
	"time"
)

// dayFormat ("2006-01-02") is shared with the streak code in store.go.

// localDate truncates a time to its local calendar date.
func localDate(t time.Time) time.Time {
	l := t.Local()
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.Local)
}

// localDaysBetween counts whole local calendar days from a to b (DST-safe).
func localDaysBetween(a, b time.Time) int {
	return int(localDate(b).Sub(localDate(a)).Hours()/24 + 0.5)
}

// ApplyDecay is the lazy decay catch-up: for every attribute it bills each
// idle day beyond the grace period as ONE negative xp_events row
// (source='decay', note 'decay · YYYY-MM-DD') plus the matching total_xp
// decrement — all in one transaction, idempotent per attribute per local day
// (the billed dates encoded in the notes are re-read inside the tx; the
// single-writer connection serializes racing callers). peak_xp is never
// touched. Days covered by a ward window are excluded; restEndedAt (nullable)
// resets the idle anchor; the caller skips the call entirely while rest mode
// is on. Returns the total XP removed.
func (s *Store) ApplyDecay(userID int64, restEndedAt *time.Time, now time.Time) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	nowStr := formatTime(now)
	today := localDate(now)

	type attrRow struct {
		key   string
		total int64
		peak  int64
	}
	rows, err := tx.Query(`SELECT key, total_xp, peak_xp FROM attributes WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	var attrs []attrRow
	for rows.Next() {
		var a attrRow
		if err := rows.Scan(&a.key, &a.total, &a.peak); err != nil {
			rows.Close()
			return 0, err
		}
		attrs = append(attrs, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var totalRemoved int64
	for _, a := range attrs {
		// Idle anchor: last positive activity, or rest end, whichever is later.
		var lastAct string
		err := tx.QueryRow(
			`SELECT MAX(created_at) FROM xp_events WHERE user_id = ? AND attribute_key = ? AND amount > 0`,
			userID, a.key).Scan(&lastAct)
		if err != nil || lastAct == "" {
			continue // never trained: nothing to decay from
		}
		anchor := mustParseTime(lastAct)
		if restEndedAt != nil && restEndedAt.After(anchor) {
			anchor = *restEndedAt
		}

		idleDays := localDaysBetween(anchor, now)
		if idleDays <= decayGraceDays {
			continue
		}

		// Dates already billed (from decay-event notes), inside the tx.
		billed := map[string]bool{}
		brows, err := tx.Query(
			`SELECT note FROM xp_events WHERE user_id = ? AND attribute_key = ? AND source = 'decay'`,
			userID, a.key)
		if err != nil {
			return 0, err
		}
		for brows.Next() {
			var note string
			if err := brows.Scan(&note); err != nil {
				brows.Close()
				return 0, err
			}
			if d, ok := strings.CutPrefix(note, "decay · "); ok {
				billed[d] = true
			}
		}
		brows.Close()
		if err := brows.Err(); err != nil {
			return 0, err
		}

		// Ward windows (lapsed ones still exclude the days they covered).
		type window struct{ from, to time.Time }
		var wards []window
		wrows, err := tx.Query(
			`SELECT created_at, expires_at FROM wards WHERE user_id = ? AND attribute_key = ?`, userID, a.key)
		if err != nil {
			return 0, err
		}
		for wrows.Next() {
			var created, expires string
			if err := wrows.Scan(&created, &expires); err != nil {
				wrows.Close()
				return 0, err
			}
			wards = append(wards, window{localDate(mustParseTime(created)), localDate(mustParseTime(expires))})
		}
		wrows.Close()
		if err := wrows.Err(); err != nil {
			return 0, err
		}
		covered := func(day time.Time) bool {
			for _, w := range wards {
				if !day.Before(w.from) && !day.After(w.to) {
					return true
				}
			}
			return false
		}

		floor := decayFloorXP(a.peak)
		running := a.total
		anchorDay := localDate(anchor)
		for d := decayGraceDays + 1; d <= idleDays; d++ {
			day := anchorDay.AddDate(0, 0, d)
			if day.After(today) {
				break
			}
			dayStr := day.Format(dayFormat)
			if billed[dayStr] || covered(day) {
				continue
			}
			if running <= floor {
				break
			}
			amt := decayDailyAmount(running)
			if running-amt < floor {
				amt = running - floor // partial final bleed down to the floor
			}
			if amt <= 0 {
				break
			}
			if _, err := tx.Exec(
				`INSERT INTO xp_events(user_id, attribute_key, amount, source, source_id, note, created_at) VALUES(?, ?, ?, 'decay', NULL, ?, ?)`,
				userID, a.key, -amt, fmt.Sprintf("decay · %s", dayStr), nowStr); err != nil {
				return 0, err
			}
			if _, err := tx.Exec(
				`UPDATE attributes SET total_xp = total_xp - ? WHERE user_id = ? AND key = ?`,
				amt, userID, a.key); err != nil {
				return 0, err
			}
			running -= amt
			totalRemoved += amt
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return totalRemoved, nil
}
```

- [ ] **Step 4: Service wrapper + wiring**

Append to `server/internal/services/decay.go`:

```go
import "time" // add to the file's imports

// ApplyDecay runs the lazy decay catch-up unless rest mode is on. It is
// called at the top of attribute-touching reads and before completions, so
// decay is always applied before new state is read or awarded. Returns the
// XP removed by this call (0 when nothing was owed).
func (s *Service) ApplyDecay() (int64, error) {
	rest, err := s.RestState()
	if err != nil {
		return 0, err
	}
	if rest.On {
		return 0, nil
	}
	ended, err := s.restEndedAt()
	if err != nil {
		return 0, err
	}
	return s.store.ApplyDecay(s.userID, ended, time.Now().UTC())
}
```

Wire it in `server/internal/services/service.go`:

- `ListAttributes`: first line of the method body becomes
```go
	if _, err := s.ApplyDecay(); err != nil {
		return nil, err
	}
```
- `GetDashboard`: as the FIRST statement, add
```go
	decayed, err := s.ApplyDecay()
	if err != nil {
		return models.Dashboard{}, err
	}
```
(the nested `s.ListAttributes()` re-applies and finds nothing owed — idempotent by design) and set `DecayedToday: decayed,` in the returned `models.Dashboard` literal. (RestMode/RestSince wiring happens in Task 6.)
- `CompleteQuest`: before `s.store.CompleteQuest(...)`, add
```go
	if _, err := s.ApplyDecay(); err != nil {
		return models.CompletionResult{}, err
	}
```
- `CreateJournalEntry`: before `s.store.InsertJournal(...)`, add
```go
	if _, err := s.ApplyDecay(); err != nil {
		return models.JournalCreateResult{}, err
	}
```
- In `server/internal/services/tools.go`, `CompleteTool`: before the `s.store.CompleteTool(...)` call, add the same two lines returning `models.ToolCompletionResult{}`.

- [ ] **Step 5: Run — PASS, full suite with -race, commit**

Run: `cd server && go test ./internal/services -run 'TestDecay' -race` → PASS (all five).
Run: `cd server && go build ./... && go vet ./... && go test ./... -race && gofmt -w .` → PASS. NOTE: pre-existing tests that complete quests still pass — seed data is recent, so no decay is owed in them.

```bash
git add server/internal/db/decay_store.go server/internal/services
git commit -m "feat: decay engine — lazy idempotent catch-up billing idle days as negative xp_events"
```

---

### Task 6: Decay status enrichment + dashboard fields

**Files:**
- Modify: `server/internal/db/decay_store.go` (add `DecayInputs`)
- Modify: `server/internal/services/decay.go` (add `decayStatus` enrichment)
- Modify: `server/internal/services/service.go` (enrich in `ListAttributes` + `GetDashboard` rest fields)
- Test: `server/internal/services/decay_test.go` (append)

**Interfaces:**
- Consumes: engine pieces from Task 5, `ActiveWardExpiry` semantics from Task 3, `RestState` from Task 4.
- Produces:
  - `db.DecayInput{LastActivity *time.Time, WardExpiry *time.Time}`
  - `(*db.Store).DecayInputs(userID int64, now time.Time) (map[string]db.DecayInput, error)`
  - Every `models.Attribute` returned by `ListAttributes`/`GetDashboard` carries a non-nil `Decay` with `State` ∈ `fresh|grace|decaying|warded|rest`, `IdleDays`, `WardedUntil`, `ProjectedDailyLoss` (0 unless decaying), `FloorLevel`.
  - `Dashboard.RestMode`, `Dashboard.RestSince` populated.

- [ ] **Step 1: Write the failing test**

Append to `server/internal/services/decay_test.go`:

```go
func TestDecayStatusOnAttributes(t *testing.T) {
	svc := newTestService(t)
	backdateAttribute(t, svc, "health", 2)   // grace
	backdateAttribute(t, svc, "wealth", 10)  // decaying
	if _, err := svc.WardAttribute("focus"); err != nil {
		t.Fatalf("ward: %v", err)
	}

	attrs, err := svc.ListAttributes()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	get := func(key string) *models.AttributeDecay {
		d := attrByKey(attrs, key).Decay
		if d == nil {
			t.Fatalf("%s has no decay status", key)
		}
		return d
	}
	if d := get("strength"); d.State != "fresh" || d.ProjectedDailyLoss != 0 {
		t.Errorf("strength = %+v, want fresh", d)
	}
	if d := get("health"); d.State != "grace" || d.IdleDays != 2 {
		t.Errorf("health = %+v, want grace/2", d)
	}
	if d := get("wealth"); d.State != "decaying" || d.ProjectedDailyLoss == 0 {
		t.Errorf("wealth = %+v, want decaying with projected loss", d)
	}
	if d := get("focus"); d.State != "warded" || d.WardedUntil == nil {
		t.Errorf("focus = %+v, want warded", d)
	}

	// Rest mode overrides every state.
	if _, err := svc.SetRestMode(true); err != nil {
		t.Fatalf("rest: %v", err)
	}
	attrs, _ = svc.ListAttributes()
	if d := attrByKey(attrs, "wealth").Decay; d == nil || d.State != "rest" {
		t.Errorf("wealth under rest = %+v, want rest", d)
	}

	dash, err := svc.GetDashboard()
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if !dash.RestMode || dash.RestSince == nil {
		t.Errorf("dashboard rest = %v/%v, want on with since", dash.RestMode, dash.RestSince)
	}
}
```

(Add `"edi/internal/models"` to the test file's imports.)

Note: `ListAttributes` applied decay BEFORE this test's assertions, so "wealth" has already been billed — its `decaying` state and non-zero projection refer to the ongoing bleed, which is exactly what the UI shows.

- [ ] **Step 2: Run to verify failure**

Run: `cd server && go test ./internal/services -run TestDecayStatusOnAttributes`
Expected: FAIL — decay statuses are nil.

- [ ] **Step 3: Implement DecayInputs**

Append to `server/internal/db/decay_store.go`:

```go
// DecayInput feeds the read-side decay status for one attribute.
type DecayInput struct {
	LastActivity *time.Time // last positive xp_event (nil if never trained)
	WardExpiry   *time.Time // latest ACTIVE ward expiry (nil if none)
}

// DecayInputs returns per-attribute-key inputs for decay status display.
func (s *Store) DecayInputs(userID int64, now time.Time) (map[string]DecayInput, error) {
	out := map[string]DecayInput{}

	rows, err := s.db.Query(
		`SELECT attribute_key, MAX(created_at) FROM xp_events
		 WHERE user_id = ? AND amount > 0 GROUP BY attribute_key`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, created string
		if err := rows.Scan(&key, &created); err != nil {
			return nil, err
		}
		t := mustParseTime(created)
		out[key] = DecayInput{LastActivity: &t}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	wrows, err := s.db.Query(
		`SELECT attribute_key, MAX(expires_at) FROM wards
		 WHERE user_id = ? AND expires_at > ? GROUP BY attribute_key`, userID, formatTime(now))
	if err != nil {
		return nil, err
	}
	defer wrows.Close()
	for wrows.Next() {
		var key, expires string
		if err := wrows.Scan(&key, &expires); err != nil {
			return nil, err
		}
		t := mustParseTime(expires)
		in := out[key]
		in.WardExpiry = &t
		out[key] = in
	}
	return out, wrows.Err()
}
```

- [ ] **Step 4: Implement enrichment + wire**

Append to `server/internal/services/decay.go`:

```go
// decayStatus computes the read-side decay state for one attribute.
func decayStatus(a models.Attribute, in db.DecayInput, rest models.RestState, restEnded *time.Time, now time.Time) *models.AttributeDecay {
	d := &models.AttributeDecay{FloorLevel: LevelForXP(DecayFloor(a.PeakXP))}

	anchor := time.Time{}
	if in.LastActivity != nil {
		anchor = *in.LastActivity
	}
	if restEnded != nil && restEnded.After(anchor) {
		anchor = *restEnded
	}
	if !anchor.IsZero() {
		d.IdleDays = localDaysBetween(anchor, now)
	}
	d.WardedUntil = in.WardExpiry

	switch {
	case rest.On:
		d.State = "rest"
	case in.WardExpiry != nil:
		d.State = "warded"
	case d.IdleDays == 0:
		d.State = "fresh"
	case d.IdleDays <= DecayGraceDays:
		d.State = "grace"
	default:
		d.State = "decaying"
		d.ProjectedDailyLoss = DailyDecay(a.TotalXP)
		if DecayFloor(a.PeakXP) >= a.TotalXP {
			d.ProjectedDailyLoss = 0 // already at the floor: nothing more to lose
		}
	}
	return d
}

// localDaysBetween mirrors db.localDaysBetween (kept in sync).
func localDaysBetween(a, b time.Time) int {
	al, bl := a.Local(), b.Local()
	ad := time.Date(al.Year(), al.Month(), al.Day(), 0, 0, 0, 0, time.Local)
	bd := time.Date(bl.Year(), bl.Month(), bl.Day(), 0, 0, 0, 0, time.Local)
	return int(bd.Sub(ad).Hours()/24 + 0.5)
}
```

(Add `"edi/internal/db"` and `"edi/internal/models"` to decay.go's imports.)

In `server/internal/services/service.go` `ListAttributes`, after the enrich loop builds `out`, add before `return`:

```go
	rest, err := s.RestState()
	if err != nil {
		return nil, err
	}
	restEnded, err := s.restEndedAt()
	if err != nil {
		return nil, err
	}
	inputs, err := s.store.DecayInputs(s.userID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Decay = decayStatus(out[i], inputs[out[i].Key], rest, restEnded, time.Now().UTC())
	}
```
(add `"time"` to service.go imports if missing).

In `GetDashboard`, after the `goldBalance` fetch add:

```go
	rest, err := s.RestState()
	if err != nil {
		return models.Dashboard{}, err
	}
```
and set `RestMode: rest.On, RestSince: rest.Since,` in the returned literal (DecayedToday was wired in Task 5).

- [ ] **Step 5: Run — PASS, full suite, commit**

Run: `cd server && go test ./internal/services -run TestDecayStatusOnAttributes` → PASS, then full check with `-race`.

```bash
git add server/internal/db/decay_store.go server/internal/services
git commit -m "feat: per-attribute decay status + rest fields on the dashboard payload"
```

---

### Task 7: HTTP endpoints + agent tools

**Files:**
- Modify: `server/internal/handlers/handlers.go`
- Modify: `server/internal/handlers/router.go`
- Modify: `server/internal/agent/agent.go`

**Interfaces:**
- Consumes: `WardAttribute(key)`, `SetRestMode(on)`, `RestState()` from Tasks 3–4.
- Produces routes: `POST /api/attributes/{key}/ward` → `WardResult`; `POST /api/rest` (body `{"on":bool}`) → `RestState`; `GET /api/rest` → `RestState`. Agent tools: `ward_attribute` (`{attribute_key}`), `set_rest_mode` (`{on}`).

- [ ] **Step 1: Handlers**

Append to `server/internal/handlers/handlers.go`:

```go
// --- decay & stakes -----------------------------------------------------------

func (h *Handlers) wardAttribute(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.WardAttribute(r.PathValue("key"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) setRest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On bool `json:"on"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, err)
		return
	}
	state, err := h.svc.SetRestMode(body.On)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *Handlers) getRest(w http.ResponseWriter, _ *http.Request) {
	state, err := h.svc.RestState()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
```

- [ ] **Step 2: Routes**

In `server/internal/handlers/router.go`, after the gold/shop block:

```go
	// Decay & stakes — ward purchases and rest mode.
	mux.HandleFunc("POST /api/attributes/{key}/ward", h.wardAttribute)
	mux.HandleFunc("GET /api/rest", h.getRest)
	mux.HandleFunc("POST /api/rest", h.setRest)
```

- [ ] **Step 3: Agent tools**

In `server/internal/agent/agent.go`, before the index loop:

```go
	add("ward_attribute", "Buy a Maintenance Ward: spend 30 gold to shield one attribute from decay for 7 days (extends an active ward).",
		`{"type":"object","required":["attribute_key"],"properties":{"attribute_key":{"type":"string"}}}`,
		func(in json.RawMessage) (any, error) {
			var p struct {
				AttributeKey string `json:"attribute_key"`
			}
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			if p.AttributeKey == "" {
				return nil, fmt.Errorf("%w: attribute_key is required", services.ErrValidation)
			}
			return svc.WardAttribute(p.AttributeKey)
		})

	add("set_rest_mode", "Turn rest mode on or off. While on, ALL attribute decay is paused (vacation/sick weeks); turning it off restarts every idle clock.",
		`{"type":"object","required":["on"],"properties":{"on":{"type":"boolean"}}}`,
		func(in json.RawMessage) (any, error) {
			var p struct {
				On bool `json:"on"`
			}
			if err := decode(in, &p); err != nil {
				return nil, err
			}
			return svc.SetRestMode(p.On)
		})
```

- [ ] **Step 4: Build, live-validate, commit**

Run: `cd server && go build ./... && go vet ./... && go test ./... && gofmt -w .` → PASS.

Live check (start `make backend`, kill any :8080 squatter first; kill the server when done):

```bash
curl -s -X POST localhost:8080/api/attributes/strength/ward            # 200 {ward, balance}
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8080/api/attributes/bogus/ward   # 404
curl -s -X POST localhost:8080/api/rest -d '{"on":true}'               # {"on":true,"since":...}
curl -s localhost:8080/api/rest                                        # same
curl -s -X POST localhost:8080/api/rest -d '{"on":false}'              # {"on":false}
curl -s localhost:8080/api/dashboard | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['rest_mode'], d['decayed_today'], d['attributes'][0]['decay'])"
sqlite3 server/edi.db "SELECT source, amount, label FROM gold_events WHERE source='ward';"
```

```bash
git add server/internal/handlers server/internal/agent
git commit -m "feat: ward + rest HTTP endpoints and agent tools"
```

---

### Task 8: apiclient + CLI

**Files:**
- Modify: `server/internal/apiclient/client.go`
- Modify: `server/cmd/edi-cli/main.go`

**Interfaces:**
- Consumes: routes from Task 7; `models.WardResult`, `models.RestState`; `models.Attribute.Decay` (rides along in the dashboard JSON automatically).
- Produces: `(*apiclient.Client).WardAttribute(key string) (models.WardResult, error)`, `(*apiclient.Client).SetRestMode(on bool) (models.RestState, error)`; CLI commands `ward <attribute>` and `rest on|off`; `edi-cli dashboard` shows non-fresh decay states.

- [ ] **Step 1: apiclient methods**

Append to `server/internal/apiclient/client.go` (before the agent tool surface section):

```go
// --- decay & stakes -----------------------------------------------------------

func (c *Client) WardAttribute(key string) (models.WardResult, error) {
	var r models.WardResult
	err := c.do(http.MethodPost, "/api/attributes/"+url.PathEscape(key)+"/ward", nil, &r)
	return r, err
}

func (c *Client) SetRestMode(on bool) (models.RestState, error) {
	var r models.RestState
	err := c.do(http.MethodPost, "/api/rest", map[string]bool{"on": on}, &r)
	return r, err
}
```

- [ ] **Step 2: CLI commands**

In `server/cmd/edi-cli/main.go`:

1. Doc comment usage block, after the gold lines:
```
//	ward <attribute>                Buy a 7-day decay ward for an attribute (30g)
//	rest on|off                     Pause/resume all attribute decay
```
2. `run` switch cases:
```go
	case "ward":
		return cmdWard(c, args)
	case "rest":
		return cmdRest(c, args)
```
3. Command functions:
```go
func cmdWard(c *apiclient.Client, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ward <attribute-key>")
	}
	res, err := c.WardAttribute(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Warded %s until %s. Balance: %dg\n",
		res.Ward.AttributeKey, res.Ward.ExpiresAt.Local().Format("2006-01-02 15:04"), res.Balance)
	return nil
}

func cmdRest(c *apiclient.Client, args []string) error {
	if len(args) != 1 || (args[0] != "on" && args[0] != "off") {
		return fmt.Errorf("usage: rest on|off")
	}
	state, err := c.SetRestMode(args[0] == "on")
	if err != nil {
		return err
	}
	if state.On {
		fmt.Println("Rest mode ON — decay paused. Recover well.")
	} else {
		fmt.Println("Rest mode OFF — idle clocks restarted from now.")
	}
	return nil
}
```
4. `usage()` function: add matching lines for `ward` and `rest`, matching the neighbors' exact column alignment and lowercase description style.
5. In `cmdDashboard`, find the loop that prints attributes and append a decay marker to each line when the state isn't fresh (adapt to the loop's existing print format):
```go
		if a.Decay != nil && a.Decay.State != "fresh" {
			fmt.Printf("  [%s", a.Decay.State)
			if a.Decay.State == "decaying" {
				fmt.Printf(": %dd idle, -%d/day", a.Decay.IdleDays, a.Decay.ProjectedDailyLoss)
			}
			fmt.Print("]")
		}
```

- [ ] **Step 3: Build, live-validate, commit**

`cd server && go build ./... && go vet ./... && go test ./... && gofmt -w .` → PASS. Then against a running backend: `go run ./cmd/edi-cli ward strength`, `go run ./cmd/edi-cli rest on`, `go run ./cmd/edi-cli rest off`, `go run ./cmd/edi-cli dashboard` — read the actual outputs. Kill the backend after.

```bash
git add server/internal/apiclient server/cmd/edi-cli
git commit -m "feat: ward + rest in apiclient and edi-cli; decay states in CLI dashboard"
```

---

### Task 9: Client data layer

**Files:**
- Modify: `client/src/lib/types.ts`
- Modify: `client/src/lib/api.ts`
- Modify: `client/src/lib/queries.ts`

**Interfaces:**
- Consumes: Task 7 routes.
- Produces (Task 10 depends on the exact names): types `AttributeDecay`, `Ward`, `WardResult`, `RestState`; `Attribute.decay?`; `Dashboard.rest_mode/rest_since?/decayed_today`; api `wardAttribute(key)`, `setRestMode(on)`; hooks `useWardAttribute()`, `useSetRestMode()`.

- [ ] **Step 1: types.ts**

After the `Attribute` interface, add:

```ts
export interface AttributeDecay {
  state: "fresh" | "grace" | "decaying" | "warded" | "rest";
  idle_days: number;
  warded_until?: string;
  projected_daily_loss: number;
  floor_level: number;
}

export interface Ward {
  id: number;
  attribute_key: string;
  expires_at: string;
  created_at: string;
}

export interface WardResult {
  ward: Ward;
  balance: number;
}

export interface RestState {
  on: boolean;
  since?: string;
}
```

In `Attribute`, after `progress`, add:
```ts
  decay?: AttributeDecay;
```

In `Dashboard`, after `gold_balance`, add:
```ts
  rest_mode: boolean;
  rest_since?: string;
  decayed_today: number;
```

- [ ] **Step 2: api.ts**

Add `RestState, WardResult` to the type import. After the gold/shop block in the `api` object:

```ts
  wardAttribute: (key: string) =>
    request<WardResult>(`/attributes/${key}/ward`, { method: "POST" }),
  setRestMode: (on: boolean) =>
    request<RestState>("/rest", { method: "POST", body: JSON.stringify({ on }) }),
```

- [ ] **Step 3: queries.ts**

Append:

```ts
export function useWardAttribute() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (key: string) => api.wardAttribute(key),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["dashboard"] });
      qc.invalidateQueries({ queryKey: ["attributes"] });
      qc.invalidateQueries({ queryKey: ["gold-events"] });
    },
  });
}

export function useSetRestMode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (on: boolean) => api.setRestMode(on),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["dashboard"] });
      qc.invalidateQueries({ queryKey: ["attributes"] });
    },
  });
}
```

- [ ] **Step 4: Build + commit**

Run: `cd client && npm run build` → clean.

```bash
git add client/src/lib
git commit -m "feat(client): decay/ward/rest types, API calls, and mutation hooks"
```

---

### Task 10: Decay UI — attribute cards, rest banner, XP feed

**Files:**
- Modify: `client/src/components/AttributeCard.tsx`
- Modify: `client/src/pages/Dashboard.tsx`
- Modify: `client/src/components/XPFeed.tsx`

**Interfaces:**
- Consumes: Task 9 types/hooks; existing `getAttr` theme meta, `pushToast`.
- Produces: decaying attributes visibly rust + show a ward button; a rest banner with toggle on the dashboard; a decay-loss alert when `decayed_today > 0`; negative XP amounts render in rust color with a `-` sign.

- [ ] **Step 1: AttributeCard decay treatment + ward button**

Replace the full contents of `client/src/components/AttributeCard.tsx` with:

```tsx
import { motion } from "framer-motion";
import { Shield, TrendingDown } from "lucide-react";
import type { Attribute } from "../lib/types";
import { getAttr } from "../lib/theme";
import { useWardAttribute } from "../lib/queries";
import { pushToast } from "../lib/toast";
import { ProgressBar } from "./ui";

const RUST = "#ff6a3d";
const WARD_COST = 30;

function DecayBadge({ attribute }: { attribute: Attribute }) {
  const d = attribute.decay;
  if (!d || d.state === "fresh" || d.state === "rest") return null;
  if (d.state === "warded") {
    return (
      <div className="mt-2 flex items-center gap-1.5 text-[10px] uppercase tracking-wider" style={{ color: "var(--color-gold)" }}>
        <Shield size={11} />
        warded until {d.warded_until ? new Date(d.warded_until).toLocaleDateString() : "?"}
      </div>
    );
  }
  if (d.state === "grace") {
    return (
      <div className="mt-2 text-[10px] uppercase tracking-wider text-faint">
        idle {d.idle_days}d — decays in {4 - d.idle_days}d
      </div>
    );
  }
  return (
    <div className="mt-2 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider" style={{ color: RUST }}>
      <TrendingDown size={11} />
      rusting · {d.idle_days}d idle · -{d.projected_daily_loss} XP/day
    </div>
  );
}

export function AttributeCard({
  attribute,
  index = 0,
  goldBalance,
}: {
  attribute: Attribute;
  index?: number;
  goldBalance?: number;
}) {
  const meta = getAttr(attribute.key);
  const Icon = meta.Icon;
  const ward = useWardAttribute();
  const decaying = attribute.decay?.state === "decaying";
  const canWard =
    typeof goldBalance === "number" &&
    goldBalance >= WARD_COST &&
    (attribute.decay?.state === "grace" || attribute.decay?.state === "decaying");

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, delay: index * 0.04, ease: [0.16, 1, 0.3, 1] }}
      className="hud-panel hud-panel-hover group relative overflow-hidden p-4"
      style={decaying ? { borderColor: `${RUST}55`, filter: "saturate(0.75)" } : undefined}
    >
      <div
        className="pointer-events-none absolute -right-6 -top-6 h-20 w-20 rounded-full opacity-50 transition-opacity group-hover:opacity-90"
        style={{ background: `radial-gradient(circle, ${decaying ? RUST : meta.color}33, transparent 70%)` }}
      />
      <div className="relative flex items-center justify-between">
        <div className="flex items-center gap-2.5">
          <div
            className="grid h-9 w-9 place-items-center rounded-lg"
            style={{ background: `${meta.color}1a`, color: meta.color }}
          >
            <Icon size={18} />
          </div>
          <div>
            <div className="text-sm font-semibold text-ink">{meta.label}</div>
            <div className="tabnum text-[11px] text-faint">{attribute.total_xp.toLocaleString()} XP</div>
          </div>
        </div>
        <div className="flex items-center gap-1.5">
          {canWard && (
            <button
              onClick={() =>
                ward.mutate(attribute.key, {
                  onSuccess: (res) =>
                    pushToast(`${meta.label} warded for 7 days (-${WARD_COST}g, ${res.balance}g left).`, "success"),
                })
              }
              disabled={ward.isPending}
              title={`Ward for 7 days (${WARD_COST}g)`}
              aria-label={`Ward ${meta.label}`}
              className="grid h-8 w-8 place-items-center rounded-md transition-colors hover:bg-white/[0.06]"
              style={{ color: "var(--color-gold)" }}
              data-testid={`ward-${attribute.key}`}
            >
              <Shield size={15} />
            </button>
          )}
          <div
            className="flex h-8 min-w-8 items-center justify-center rounded-md px-2 font-display text-sm font-bold"
            style={{ background: `${meta.color}14`, color: meta.color }}
          >
            {attribute.level}
          </div>
        </div>
      </div>

      <div className="relative mt-3.5">
        <ProgressBar value={attribute.progress} color={decaying ? RUST : meta.color} height={6} shimmer={false} />
        <div className="tabnum mt-1.5 flex justify-between text-[10px] text-faint">
          <span>Lv {attribute.level}</span>
          <span>
            {attribute.xp_into_level}/{attribute.xp_for_next_level}
          </span>
        </div>
        <DecayBadge attribute={attribute} />
      </div>
    </motion.div>
  );
}
```

In `client/src/pages/Dashboard.tsx`, pass the balance to each card:
```tsx
            <AttributeCard key={a.key} attribute={a} index={i} goldBalance={data.gold_balance} />
```

- [ ] **Step 2: Rest banner + decay alert on the dashboard**

In `client/src/pages/Dashboard.tsx`, directly under `<CharacterHeader ... />`, add (import `Moon` from lucide-react, `useSetRestMode` from queries):

```tsx
      {data.rest_mode && (
        <div
          className="flex items-center justify-between rounded-lg border px-4 py-3"
          style={{ borderColor: "var(--color-gold)", background: "rgba(255,176,0,0.06)" }}
          data-testid="rest-banner"
        >
          <div className="flex items-center gap-2 text-sm" style={{ color: "var(--color-goldhi)" }}>
            <Moon size={16} />
            Rest mode is ON — all decay is paused. Recover well.
          </div>
          <button
            onClick={() => setRest.mutate(false)}
            className="rounded-md border border-edge px-3 py-1.5 text-xs font-medium text-muted transition-colors hover:text-ink"
          >
            Wake up
          </button>
        </div>
      )}

      {data.decayed_today > 0 && (
        <div
          className="rounded-lg border px-4 py-3 text-sm"
          style={{ borderColor: "#ff6a3d88", background: "rgba(255,106,61,0.07)", color: "#ff8a65" }}
          data-testid="decay-alert"
        >
          SYSTEM DEGRADATION — {data.decayed_today} XP lost to decay since your last visit. Train the rusting
          attributes or ward them.
        </div>
      )}
```

with, near the other hooks at the top of the component:
```tsx
  const setRest = useSetRestMode();
```

Add a small rest toggle for turning rest ON (when off): in the dashboard header area next to the section title (or immediately above the attributes section), add:

```tsx
      {!data.rest_mode && (
        <button
          onClick={() => setRest.mutate(true)}
          className="flex items-center gap-1.5 text-[11px] uppercase tracking-wider text-faint transition-colors hover:text-muted"
          title="Pause all decay (vacation/sick)"
          data-testid="rest-toggle"
        >
          <Moon size={12} /> rest mode
        </button>
      )}
```

Placement judgment call: put it right of the "Attributes" `SectionTitle` in a flex row — adapt to the page's existing layout.

- [ ] **Step 3: XPFeed handles negative amounts**

In `client/src/components/XPFeed.tsx`, replace the amount div:

```tsx
              <div className="tabnum text-sm font-semibold" style={{ color: e.amount < 0 ? "#ff6a3d" : meta.color }}>
                {e.amount < 0 ? e.amount : `+${e.amount}`}
              </div>
```

and extend the source label line so decay events read clearly:

```tsx
              <div className="text-[10px] uppercase tracking-wide text-faint">
                {meta.label}
                {e.source === "seed" && " · seed"}
                {e.source === "decay" && " · decay"}
              </div>
```

- [ ] **Step 4: Build + commit**

Run: `cd client && npm run build` → clean.

```bash
git add client/src
git commit -m "feat(client): decay UI — rusting attribute cards, ward button, rest banner, negative XP feed"
```

---

### Task 11: End-to-end validation + docs

**Files:**
- Modify: `CLAUDE.md` (decay invariants)
- Modify: `README.md` (feature notes)

- [ ] **Step 1: Full automated pass**

```bash
cd server && go build ./... && go vet ./... && go test ./... -race && gofmt -l .
cd ../client && npm run build
```
Expected: green; `gofmt -l` prints nothing.

- [ ] **Step 2: Live decay simulation (curl + sqlite3)**

Start `make backend` (kill any :8080 squatter first). Simulate idleness by backdating one attribute's positive events in the dev DB, then watch the catch-up bill it:

```bash
sqlite3 server/edi.db "UPDATE xp_events SET created_at = strftime('%Y-%m-%dT%H:%M:%f','now','-9 days') || '000000Z' WHERE attribute_key='learning' AND amount > 0;"
curl -s localhost:8080/api/dashboard | python3 -c "import sys,json; d=json.load(sys.stdin); print('decayed_today:', d['decayed_today']); print([ (a['key'], a['decay']['state'], a['total_xp']) for a in d['attributes'] if a['key']=='learning' ])"
sqlite3 server/edi.db "SELECT amount, note FROM xp_events WHERE source='decay' ORDER BY id;"
sqlite3 server/edi.db "SELECT a.key, a.total_xp, (SELECT COALESCE(SUM(amount),0) FROM xp_events e WHERE e.attribute_key=a.key AND e.user_id=a.user_id) FROM attributes a WHERE a.key='learning';"
```
Expected: 6 decay events (idle days 4–9), `decayed_today` > 0, state `decaying`, and the audit columns equal. Then ward it and confirm tomorrow's bill would be excluded (state flips to `warded`):

```bash
curl -s -X POST localhost:8080/api/attributes/learning/ward
curl -s localhost:8080/api/dashboard | python3 -c "import sys,json; print([a['decay'] for a in json.load(sys.stdin)['attributes'] if a['key']=='learning'])"
```

- [ ] **Step 3: Browser validation (agent-browser skill)**

With `make dev` running: load the dashboard — the decay alert box and the rusting card (badge, rust progress bar) render; console clean. Click the ward shield on a decaying attribute → toast, badge flips to "warded", gold drops in the header. Toggle rest mode on → banner appears, all cards show no decay pressure; toggle off. Complete a quest → celebration overlay unaffected. Re-check console.

- [ ] **Step 4: Docs**

In `CLAUDE.md` "Invariants you must not break", after the gold bullet, add:

```markdown
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
```

In `README.md`: add a "Decay & stakes" concept bullet (mirroring the gold bullet's voice), the three new routes to the API table, and bump the agent-tool count by 2 (`ward_attribute`, `set_rest_mode`) in the MCP tool list.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: decay invariants + feature notes"
```

---

## Self-review notes

- Spec coverage: decay rules incl. running-total 1%/min-5/floor (T1/T5), idle clock with rest anchor (T5), ward windows excluded without clock reset (T3/T5), lazy idempotent catch-up wired into dashboard/attributes/completions (T5), peak_xp same-tx maintenance + backfill (T1/T2), ward purchase tx + stacking + gold source `ward` (T3), rest mode settings + banner (T4/T6/T10), dashboard decay statuses + rest fields + decayed_today (T5/T6), routes/tools (T7), apiclient/CLI (T8), client types/UI incl. negative feed styling (T9/T10), tests incl. `-race` concurrency (T5), live + browser validation and docs (T11).
- Type consistency checked: `ApplyDecay(userID, restEndedAt, now)`; `CreateWard(userID, attrKey, attrName, cost, days)`; `DecayInputs` map keyed by attribute key; `ListGoldEvents(limit, source)` (two-arg, post-Phase-1 signature) used in ward tests; frontend hook names match between Tasks 9 and 10.
- Known judgment calls for the executor: exact insertion point of the rest toggle in Dashboard.tsx; `cmdDashboard`'s attribute-print loop format in the CLI.
