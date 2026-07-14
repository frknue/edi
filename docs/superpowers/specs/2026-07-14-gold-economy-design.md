# Gold Economy + Reward Shop — Design

Date: 2026-07-14
Status: Approved (Phase 1 of the engagement roadmap)

## Why

Engagement in edi leaks in three places: rewards feel hollow, there is no
forward pull, and the app is easy to forget. The roadmap fixes these in
phases:

1. **Gold economy + reward shop** (this spec) — make the payoff real.
2. Decay / stakes — make neglect visible.
3. Telegram bot + `edi status` shell presence — pull the user back in.
4. Achievements, titles, AI weekly chronicle — collection and narrative.

Each phase gets its own spec → plan → build cycle.

## Concept

**XP is permanent progress; gold is spendable permission.** Every
XP-awarding action also mints gold. The user defines a shop of real-life
rewards ("gaming evening: 50g", "order takeout: 30g") and spends gold on
them guilt-free. Gold couples indulgence to effort.

## Minting rule

- Every `xp_events` row minted also writes one `gold_events` row **in the
  same transaction**, with `amount = max(1, xp/10)` (integer division).
- The rule is global — quests, subtask bonuses, tools, journal daily XP all
  mint automatically. No per-quest gold field, no new creation-flow config.
- The mint function is a pure function in `services/xp.go` next to the
  level math, unit-tested.

## Data model (new migration `NNN_gold.sql`)

### `gold_events`

Mirrors `xp_events`:

| column        | type    | notes                                              |
|---------------|---------|----------------------------------------------------|
| id            | integer | PK                                                 |
| user_id       | integer |                                                    |
| amount        | integer | positive = mint, negative = purchase               |
| source        | text    | `quest` \| `subtask` \| `tool` \| `journal` \| `purchase` \| `grant` |
| label         | text    | human-readable ("quest · subtask" style, or item name) |
| shop_item_id  | integer | nullable, set for purchases                        |
| created_at    | text    | UTC fixed-width (`timeLayout`)                     |

Index on `(user_id, created_at)` and `(user_id, source)`.

### `shop_items`

| column      | type    | notes                                   |
|-------------|---------|------------------------------------------|
| id          | integer | PK                                       |
| user_id     | integer |                                          |
| name        | text    | required, non-empty                      |
| price       | integer | required, > 0                            |
| created_at  | text    | UTC fixed-width                          |
| archived_at | text    | nullable — archive instead of delete so purchase history keeps labels |

### Invariants

- **Gold is auditable**: balance is `SUM(gold_events.amount)` computed on
  read. No stored balance column anywhere (matches the "don't store derived
  values" rule and the XP audit invariant).
- **Balance never goes negative**: `PurchaseItem` checks
  `balance >= price` *inside the purchase transaction* (single-writer
  SQLite, `SetMaxOpenConns(1)`, serializes this safely).
- All items are repeatable. One-time big-ticket items are handled by the
  user archiving them after purchase.

## Backfill

On migration (one-time), mint a single `grant` gold_event per user with
`amount = max(1, total_historical_xp/10)` computed from existing
`xp_events`, labeled "retroactive grant". This makes the shop usable on
day one.

## Service layer (single source of truth)

New methods on `services.Service`, each exposed via handler **and** agent
tool, then `internal/apiclient` + CLI, then `client/src/lib/types.ts`:

- `ListShopItems` (active items; `[]` never `null`)
- `CreateShopItem(name, price)` — validation: name non-empty, price > 0
- `UpdateShopItem(id, name, price)`
- `ArchiveShopItem(id)`
- `PurchaseItem(id)` — tx: load item (active), check balance, write
  negative gold_event. Insufficient gold → `services.ErrValidation` → 400.
  Unknown/archived item → `services.ErrNotFound` → 404.
- `GoldLedger` (recent gold_events; `[]` never `null`)
- Dashboard payload gains `gold_balance`.

Completion paths (`CompleteQuest`, `CompleteTool`, `InsertJournal`) mint
gold inside their existing transactions; their result payloads gain a
`gold` field so clients can show it.

## Frontend

- Gold balance in the CRT header (dashboard payload).
- New **Shop** page in the nav: item list with prices, buy button with
  confirm, add/edit/archive items, recent purchases. Loading/empty/error
  states as usual; mutations invalidate the relevant query keys.
- `RewardPayload` in `lib/reward.tsx` gains a `gold` field; the celebrate
  overlay shows `+Ng` under the XP lines.
- Purchase success → toast + balance update. Errors surface via the global
  MutationCache toast (free).

## Testing

- Unit: mint math (`max(1, xp/10)` edges: 0, 9, 10, 11 XP).
- Purchase with insufficient balance → `ErrValidation` (HTTP 400).
- Concurrent double-purchase does not double-spend (`-race`), sibling of
  `TestCompleteQuestConcurrentNoDoubleAward`.
- Audit: balance == `SUM(gold_events.amount)` after a mixed
  mint/purchase sequence; every completion writes gold in the same tx.
- JSON contract: shop list + ledger serialize as `[]` when empty
  (extend `TestEmptyListsSerializeAsArrays`).
- Live validation: `make backend`, curl the endpoints, verify rows in
  `server/edi.db` via `sqlite3`; browser pass on the Shop page with a
  clean console.

## Out of scope (later phases)

Decay/stakes, achievements/titles, Telegram bot, `edi status` shell
presence, AI chronicle, item cooldowns, one-time item flags.
