# Collapsible sidebar (icon rail) — design

Date: 2026-07-18
Status: approved

## Goal

Let the desktop sidebar collapse to a narrow icon-only rail (cmux/VS Code style)
so the UI can present a quieter, more focused frame. Client-only change; no
backend involvement.

## Decisions (from brainstorming)

- **Collapse mode:** icon-only rail (~56px), not fully hidden. Navigation stays
  one click away.
- **Tools group when collapsed:** flatten — Daily Mood Log and Journal render as
  direct rail icons; the Tools wrapper button and chevron do not render in
  collapsed mode. The group returns when expanded.
- **Content width:** unchanged. The app keeps its centered `max-w-[1240px]`
  container in both states; collapsing is about focus, not reflow.

## Behavior

### State & persistence

- `collapsed: boolean` React state in `App`, initialized from
  `localStorage["edi.sidebarCollapsed"]` ("1"/"0"), written back on every
  toggle. Defaults to expanded. Survives reloads.

### Toggles

- A chevron toggle button pinned at the bottom of the sidebar: `◀` when
  expanded ("Collapse sidebar"), `▶` when collapsed ("Expand sidebar").
  `data-testid="sidebar-toggle"`, native `title` tooltip.
- Keyboard shortcut **Cmd/Ctrl+B** (global `keydown` listener). Ignored when
  the event target is an `input`, `textarea`, `select`, or `contenteditable`
  element so it never fires mid-typing (e.g. journal entries).

### Collapsed rendering (rail, `w-14` ≈ 56px)

- Sidebar width animates between `w-60` and `w-14` via a CSS width transition;
  labels hide (no text in the rail).
- Logo reduces to the `>_` glyph box only; wordmark and tagline hide.
- Nav items become centered icon buttons keeping the existing active treatment
  (gold left accent bar + amber tint). Each has a native `title` tooltip with
  its label. Existing `data-testid="nav-*"` attributes are preserved.
- Flattened tool items (Mood Log, Journal) keep their teal active tint so the
  "tool" identity survives; `data-testid="nav-tool-*"` preserved.
- The "Single-user mode" footer card hides entirely in collapsed mode.

### Unchanged

- Mobile layout (top bar + bottom tab nav) is untouched; collapse is
  desktop-only (`lg:` breakpoint and up).
- No new npm dependencies. Tooltips are native `title` attributes — no popover
  library.

## Code shape

- Extract the desktop sidebar from `App.tsx` into
  `client/src/components/Sidebar.tsx` with props:
  `view`, `setView`, `collapsed`, `onToggle`. `App.tsx` keeps view switching,
  the mobile header/bottom nav, and owns the collapsed state (with the
  localStorage read/write and the Cmd/Ctrl+B listener).
- The `View` type and `TOOL_CHILDREN` list move to `Sidebar.tsx` and are
  exported from there; `App.tsx` imports `View`, and the mobile bottom nav
  keeps its own inline list (shorter "Mood Log" label). `TOOL_CHILDREN` is
  exported for the sidebar's two render modes (expanded group, collapsed
  flattened rail) — no duplication of the nav item list between those two
  modes, one list rendered two ways.

## Error handling

- `localStorage` access wrapped defensively (private-mode/quota errors fall
  back to in-memory default). No other failure surface — no network, no
  backend.

## Validation

1. `cd client && npm run build` — strict TS (`noUnusedLocals`), Vite build.
2. Real browser pass (agent-browser/Playwright against `make dev`):
   - Toggle via button: rail shrinks/expands, labels disappear/reappear.
   - Toggle via Cmd/Ctrl+B; confirm it does NOT fire while focus is in a
     journal textarea.
   - Active nav states correct in both modes, including a tool child
     (Journal) active in collapsed mode.
   - Reload with the rail collapsed → stays collapsed (persistence).
   - Browser console clean (no errors/warnings).
