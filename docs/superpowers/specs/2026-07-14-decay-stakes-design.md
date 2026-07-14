# Decay & Stakes — Design

Date: 2026-07-14
Status: Approved (Phase 2 of the engagement roadmap)

## Why

Phase 1 (gold economy + reward shop, shipped) made rewards real. Phase 2 adds
the other half of habit glue: something to lose. Neglected attributes bleed
XP — visibly, honestly, and with guardrails — and earned gold buys
protection, giving the economy its first real sink.

Roadmap context: Phase 3 = Telegram bot + `edi status` shell presence;
Phase 4 = achievements/titles + AI weekly chronicle.

## Concept

**Attributes you stop training decay.** Real XP loss (levels can drop), not
a cosmetic dimming — but proportional, floored, and escapable through two
deliberate mechanisms: a gold-bought ward and an explicit rest mode.

## Decay rules (exact)

- **Idle clock** per attribute: local days since the LATEST of
  (a) the attribute's last positive `xp_events` row, or
  (b) the end of the most recent rest period.
- **Grace:** idle days 1–3 cost nothing.
- **Bleed:** from idle day 4 onward, each idle day costs
  `max(5, floor(total_xp / 100))` XP (i.e. 1% of the attribute's total XP at
  the time each day's decay is applied, minimum 5).
- **Floor:** an attribute never decays below `XPForLevel(peakLevel - 2)`,
  where `peakLevel = LevelForXP(peak_xp)`. If the floor is above the current
  total (already low), no decay applies. Level-1..2 peaks floor at 0 XP but
  the min-5 bleed still stops at the floor (partial last bleed allowed, never
  below the floor, never below 0).
- **Exclusions:** no decay for a warded attribute (see Wards); no decay for
  anyone while rest mode is on; days covered by rest do not count as idle.
- **No gold clawback:** decay events are negative XP; `GoldForXP(<=0) = 0`,
  so no gold is ever minted or removed by decay.

## Application mechanism: lazy catch-up on touch

No daemon, no cron. A single service entry point `ApplyDecay()` runs at the
top of attribute-touching reads (at minimum: `GetDashboard`,
`ListAttributes`) and inside the completion paths' service methods BEFORE
completion (so a comeback quest lands on post-decay numbers).

- It computes, per attribute, the owed decay days since the last applied
  decay (or since activity), and writes ONE negative `xp_events` row per owed
  idle day (source `decay`, note `decay · idle day N`) plus the matching
  `attributes.total_xp` decrement — all in ONE transaction.
- **Idempotent per local day:** inside the tx, the latest decay event date
  per attribute is re-read; days already billed are skipped. With the
  single-writer connection this makes concurrent first-touches safe
  (regression test with `-race`, sibling of the completion/purchase tests).
- Server offline for a week → the next touch catches up all owed days
  honestly, respecting the floor at each step.

## peak_xp

New column `attributes.peak_xp INTEGER NOT NULL DEFAULT 0`, maintained in
the SAME tx as every XP award (`peak_xp = MAX(peak_xp, new total_xp)`),
backfilled by migration to the current `total_xp` (decay hasn't existed, so
current == peak). Same stored-but-auditable treatment as `total_xp`: the
audit check is `peak_xp == running max of cumulative event sums`.

## Maintenance Ward (gold sink)

- `POST /api/attributes/{key}/ward` spends a flat **30 gold** and shields
  that one attribute from decay for **7 days**.
- Storage: `wards` table — id, user_id, attribute_key, expires_at,
  created_at. Active ward = `expires_at > now`. Re-warding extends 7 days
  from the CURRENT expiry (stacking allowed).
- Payment: negative `gold_events` row, source `ward`, label
  `Maintenance Ward · <attribute name>` — inside the same tx as the ward
  insert; balance checked inside the tx (never negative), insufficient gold
  → `ErrValidation` (400), unknown attribute → `ErrNotFound` (404).
- The ward does NOT stop the idle clock — an attribute idle 10 days whose
  ward lapses owes only the uncovered days beyond grace (days covered by an
  active ward are excluded from billing, like rest days).

## Rest mode

- `POST /api/rest` with `{"on": true|false}`; state in `app_settings`
  (keys: `rest_mode` = "1"/"", `rest_ended_at` = timestamp written when
  turned off).
- While on: `ApplyDecay` is a no-op. When turned off: every attribute's idle
  clock restarts from `rest_ended_at` (per the idle-clock definition above).
- Free, but loud: the dashboard shows a prominent banner while active
  (`rest_mode` + `rest_since` in the dashboard payload).

## Surfaces (service → handler → agent tools → apiclient/CLI → types.ts)

- **Dashboard payload:** each attribute gains a `decay` object:
  `state` (`fresh` | `grace` | `decaying` | `warded` | `rest`), `idle_days`,
  `warded_until` (nullable), `projected_daily_loss` (0 when not decaying),
  and the floor level. Top-level: `rest_mode` bool, `rest_since` nullable,
  and `decayed_today` (total XP lost by the most recent catch-up, so a
  returning user is confronted with the bleed).
- **Routes:** `POST /api/attributes/{key}/ward`, `POST /api/rest`.
- **Agent tools:** `ward_attribute` (`{attribute_key}`), `set_rest_mode`
  (`{on: bool}`). Decay itself needs no tool — it applies on any read.
- **apiclient + CLI:** typed methods for both; CLI commands
  `edi-cli ward <attribute>` and `edi-cli rest on|off`; `edi-cli dashboard`
  shows per-attribute decay state.
- **Web UI:** attribute cards show the decay state (CRT rust treatment on
  decaying attributes, dim + static per state), a ward button with the 30g
  price (disabled when unaffordable — global error toast covers failures),
  rest-mode toggle + banner on the dashboard, and negative amounts styled
  red/rust in the XP feed. Lists stay `[]`-never-`null`.

## Invariants (additions)

- XP audit invariant unchanged: decay writes negative `xp_events` in the
  same tx as the `total_xp` decrement.
- Decay is idempotent per attribute per local day, enforced inside the tx.
- Decay never takes an attribute below its floor, never below 0.
- `peak_xp` never decreases.
- Ward purchases follow the gold audit + never-negative-balance invariants.

## Testing

- Pure math: grace boundary (day 3 vs 4), 1%-vs-min-5 crossover, floor clamp
  (partial final bleed), idle-day counting across rest windows and ward
  windows.
- Store: catch-up writes N events for N owed days; concurrent double-touch
  applies once (`-race`); completion after long idleness decays first, then
  awards; peak_xp maintained on award and used for the floor.
- Ward: purchase tx (insufficient gold 400, unknown attribute 404, balance
  audit), extension stacking, decay exclusion.
- Rest: no decay while on; clocks restart at off; `rest_ended_at`
  respected.
- JSON contract: new nullable/array fields serialize correctly.
- Live validation: curl + sqlite3 (simulate idleness by backdating an
  xp_event's `created_at` in the dev DB), browser pass with clean console.

## Out of scope (later phases)

Streak stakes beyond the existing reset, gold decay, HP systems,
achievements, Telegram/shell surfaces, AI chronicle, variable ward pricing.
