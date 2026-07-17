# Collapsible Sidebar (Icon Rail) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the desktop sidebar collapse to a ~56px icon-only rail (cmux/VS Code style), toggled by a button and Cmd/Ctrl+B, persisted in localStorage.

**Architecture:** Client-only change. Extract the desktop sidebar out of `App.tsx` into `components/Sidebar.tsx`; `App` owns a `collapsed` boolean (localStorage-backed) and a global keyboard listener, and passes `collapsed`/`onToggle` down. One nav-item list rendered two ways (expanded vs rail).

**Tech Stack:** React 18 + TypeScript (strict), Tailwind v4 classes + existing CSS variables, lucide-react icons. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-07-18-collapsible-sidebar-design.md`

## Global Constraints

- No new npm dependencies. Tooltips are native `title` attributes.
- localStorage key: `edi.sidebarCollapsed`, values `"1"` / `"0"`; reads/writes wrapped in `try/catch` (private mode).
- Collapse is desktop-only (`lg:` and up). Mobile top bar + bottom tab nav unchanged.
- Main content keeps the centered `max-w-[1240px]` container in both states.
- Preserve existing `data-testid`s: `nav-dashboard|quests|shop|agent`, `nav-tools`, `nav-tool-moodlog|journal`. New: `sidebar-toggle`.
- Strict TS: `noUnusedLocals`/`noUnusedParameters` — remove imports that become unused or `npm run build` fails.
- Validation for every task: `cd client && npm run build` must pass; final task does the full browser pass.
- All work happens in `client/`; the Go backend is untouched.

---

### Task 1: Extract `Sidebar.tsx` (pure refactor, zero behavior change)

**Files:**
- Create: `client/src/components/Sidebar.tsx`
- Modify: `client/src/App.tsx` (remove inline desktop sidebar; keep mobile header/bottom-nav/main)

**Interfaces:**
- Produces: `export type View = "dashboard" | "quests" | "shop" | "moodlog" | "journal" | "agent"`;
  `export const TOOL_CHILDREN: { id: View; label: string; Icon: LucideIcon }[]`;
  `export function Logo(): JSX.Element`;
  `export function Sidebar(props: { view: View; setView: (v: View) => void }): JSX.Element`.
  Task 2 extends `Logo` and `Sidebar` props — signatures here are Task 1's parity checkpoint only.

- [ ] **Step 1: Create `client/src/components/Sidebar.tsx`**

```tsx
import { useState } from "react";
import {
  Bot,
  BookHeart,
  BrainCircuit,
  ChevronDown,
  LayoutDashboard,
  ScrollText,
  Store,
  Wrench,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

export type View = "dashboard" | "quests" | "shop" | "moodlog" | "journal" | "agent";

type NavItem = { id: View; label: string; Icon: LucideIcon };

// "Tools" is not a page — in the sidebar it's a collapsible group whose children
// (Daily Mood Log, Journal) are the actual destinations.
export const TOOL_CHILDREN: NavItem[] = [
  { id: "moodlog", label: "Daily Mood Log", Icon: BrainCircuit },
  { id: "journal", label: "Journal", Icon: BookHeart },
];

const TOP_ITEMS: NavItem[] = [
  { id: "dashboard", label: "Dashboard", Icon: LayoutDashboard },
  { id: "quests", label: "Quests", Icon: ScrollText },
  { id: "shop", label: "Shop", Icon: Store },
];

const AGENT_ITEM: NavItem = { id: "agent", label: "Agent", Icon: Bot };

export function Logo() {
  return (
    <div className="flex items-center gap-2.5">
      <div
        className="grid h-8 w-8 shrink-0 place-items-center rounded-sm border font-display text-lg leading-none"
        style={{
          borderColor: "var(--color-phos)",
          color: "var(--color-phos)",
          boxShadow: "0 0 14px -4px rgba(75,255,126,0.8), inset 0 0 10px rgba(75,255,126,0.12)",
        }}
      >
        &gt;_
      </div>
      <div>
        <div
          className="cursor-blink font-display text-xl leading-none text-ink"
          style={{ color: "var(--color-phos)" }}
        >
          edi
        </div>
        <div className="text-[9px] uppercase tracking-[0.3em] text-faint">life-rpg terminal</div>
      </div>
    </div>
  );
}

export function Sidebar({ view, setView }: { view: View; setView: (v: View) => void }) {
  const inToolsGroup = view === "moodlog" || view === "journal";
  const [toolsOpen, setToolsOpen] = useState(false);
  const showChildren = toolsOpen || inToolsGroup;

  const navBtnStyle = (active: boolean) => ({
    background: active ? "rgba(255,176,0,0.08)" : "transparent",
    color: active ? "var(--color-goldhi)" : "var(--color-muted)",
  });

  const topItem = ({ id, label, Icon }: NavItem) => (
    <button
      key={id}
      onClick={() => setView(id)}
      data-testid={`nav-${id}`}
      className="group relative flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors"
      style={navBtnStyle(view === id)}
    >
      {view === id && (
        <span
          className="absolute left-0 top-1/2 h-5 w-1 -translate-y-1/2 rounded-r-full"
          style={{ background: "var(--color-gold)" }}
        />
      )}
      <Icon size={18} className="transition-transform group-hover:scale-110" />
      {label}
    </button>
  );

  return (
    <aside className="sticky top-0 hidden h-screen w-60 shrink-0 flex-col border-r border-edge px-5 py-6 lg:flex">
      <Logo />
      <nav className="mt-10 flex flex-1 flex-col gap-1">
        {TOP_ITEMS.map(topItem)}

        {/* Tools: collapsible group, no page of its own */}
        <button
          onClick={() => setToolsOpen((o) => !o)}
          data-testid="nav-tools"
          aria-expanded={showChildren}
          className="group relative flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors"
          style={navBtnStyle(inToolsGroup)}
        >
          {inToolsGroup && (
            <span
              className="absolute left-0 top-1/2 h-5 w-1 -translate-y-1/2 rounded-r-full"
              style={{ background: "var(--color-gold)" }}
            />
          )}
          <Wrench size={18} className="transition-transform group-hover:scale-110" />
          Tools
          <ChevronDown
            size={14}
            className="ml-auto transition-transform"
            style={{ transform: showChildren ? "rotate(180deg)" : "none" }}
          />
        </button>
        {showChildren && (
          <div className="ml-4 flex flex-col gap-0.5 border-l border-edge pl-3">
            {TOOL_CHILDREN.map(({ id, label, Icon }) => {
              const active = view === id;
              return (
                <button
                  key={id}
                  onClick={() => setView(id)}
                  data-testid={`nav-tool-${id}`}
                  className="flex items-center gap-2.5 rounded-md px-2.5 py-2 text-[13px] font-medium transition-colors"
                  style={{
                    background: active ? "rgba(46,230,200,0.10)" : "transparent",
                    color: active ? "var(--color-spirituality)" : "var(--color-faint)",
                  }}
                >
                  <Icon size={15} />
                  {label}
                </button>
              );
            })}
          </div>
        )}

        {topItem(AGENT_ITEM)}
      </nav>
      <div className="rounded-xl border border-edge bg-white/[0.02] p-3">
        <div className="text-[11px] font-medium text-muted">Single-user mode</div>
        <div className="mt-0.5 text-[10px] text-faint">Self-hosted · SQLite</div>
      </div>
    </aside>
  );
}
```

- [ ] **Step 2: Rewrite `client/src/App.tsx` to use it**

Replace the entire file with:

```tsx
import { useState } from "react";
import { Bot, BookHeart, BrainCircuit, LayoutDashboard, ScrollText, Store } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { DashboardPage } from "./pages/Dashboard";
import { QuestsPage } from "./pages/Quests";
import { JournalPage } from "./pages/Journal";
import { SuggestionsPage } from "./pages/Suggestions";
import { ShopPage } from "./pages/Shop";
import { DailyMoodLog } from "./components/DailyMoodLog";
import { Logo, Sidebar } from "./components/Sidebar";
import type { View } from "./components/Sidebar";

export default function App() {
  const [view, setView] = useState<View>("dashboard");

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-[1240px] flex-col lg:flex-row">
      <Sidebar view={view} setView={setView} />

      {/* Mobile top bar */}
      <header className="flex items-center justify-between border-b border-edge px-4 py-3 lg:hidden">
        <Logo />
      </header>

      {/* Main content */}
      <main className="flex-1 px-4 pb-28 pt-5 sm:px-6 lg:px-8 lg:pb-10 lg:pt-8">
        {view === "dashboard" && (
          <DashboardPage onGoToQuests={() => setView("quests")} onGoToAgent={() => setView("agent")} />
        )}
        {view === "quests" && <QuestsPage />}
        {view === "shop" && <ShopPage />}
        {view === "moodlog" && <DailyMoodLog onClose={() => setView("dashboard")} />}
        {view === "journal" && <JournalPage />}
        {view === "agent" && <SuggestionsPage />}
      </main>

      {/* Mobile bottom nav: flat, so the tool children are direct tabs */}
      <nav className="fixed inset-x-0 bottom-0 z-30 flex items-center justify-around border-t border-edge bg-[var(--color-abyss)]/95 px-2 py-2 backdrop-blur lg:hidden">
        {(
          [
            { id: "dashboard", label: "Dashboard", Icon: LayoutDashboard },
            { id: "quests", label: "Quests", Icon: ScrollText },
            { id: "shop", label: "Shop", Icon: Store },
            { id: "moodlog", label: "Mood Log", Icon: BrainCircuit },
            { id: "journal", label: "Journal", Icon: BookHeart },
            { id: "agent", label: "Agent", Icon: Bot },
          ] as { id: View; label: string; Icon: LucideIcon }[]
        ).map(({ id, label, Icon }) => (
          <button
            key={id}
            onClick={() => setView(id)}
            className="flex flex-1 flex-col items-center gap-0.5 rounded-lg py-1.5 text-[10px] font-medium"
            style={{ color: view === id ? "var(--color-goldhi)" : "var(--color-faint)" }}
          >
            <Icon size={20} />
            {label}
          </button>
        ))}
      </nav>
    </div>
  );
}
```

(Note the only intended diffs vs today: sidebar markup moved to the component, `shrink-0` added to the logo glyph box, and `View`/`TOOL_CHILDREN` now live in `Sidebar.tsx`. Everything else is verbatim.)

- [ ] **Step 3: Build**

Run: `cd client && npm run build`
Expected: PASS (tsc emits nothing, Vite build succeeds). A failure here is almost certainly a leftover unused import in `App.tsx`.

- [ ] **Step 4: Quick parity check in browser**

With `make dev` running (start it in the background from the repo root if it isn't), load `http://localhost:5173` in a desktop-width browser (agent-browser skill). Verify:
- Sidebar looks identical to before (logo, Dashboard/Quests/Shop, Tools group expands/collapses with its children, Agent, footer card).
- Clicking `nav-quests` shows the Quests page; clicking `nav-tools` then `nav-tool-journal` shows the Journal with the teal active tint.
- Browser console has no errors/warnings.

- [ ] **Step 5: Commit**

```bash
git add client/src/components/Sidebar.tsx client/src/App.tsx
git commit -m "refactor(client): extract desktop sidebar into components/Sidebar.tsx"
```

---

### Task 2: Collapse state, icon rail, toggle button, persistence

**Files:**
- Modify: `client/src/components/Sidebar.tsx` (full rewrite below)
- Modify: `client/src/App.tsx` (add collapsed state + persistence)

**Interfaces:**
- Consumes: Task 1's `Sidebar`/`Logo`/`View`/`TOOL_CHILDREN`.
- Produces: `Sidebar(props: { view: View; setView: (v: View) => void; collapsed: boolean; onToggle: () => void })`;
  `Logo(props: { collapsed?: boolean })` (defaults to `false`; mobile header keeps calling `<Logo />`).
  Task 3 relies on `App`'s `toggleSidebar: () => void` callback defined in Step 2.

- [ ] **Step 1: Rewrite `client/src/components/Sidebar.tsx`**

Replace the `Logo` and `Sidebar` functions (imports change too — full file):

```tsx
import { useState } from "react";
import {
  Bot,
  BookHeart,
  BrainCircuit,
  ChevronDown,
  ChevronsLeft,
  ChevronsRight,
  LayoutDashboard,
  ScrollText,
  Store,
  Wrench,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { CSSProperties } from "react";

export type View = "dashboard" | "quests" | "shop" | "moodlog" | "journal" | "agent";

type NavItem = { id: View; label: string; Icon: LucideIcon };

// "Tools" is not a page — in the expanded sidebar it's a collapsible group whose
// children (Daily Mood Log, Journal) are the actual destinations. In the
// collapsed rail the children render as direct icons and the group disappears.
export const TOOL_CHILDREN: NavItem[] = [
  { id: "moodlog", label: "Daily Mood Log", Icon: BrainCircuit },
  { id: "journal", label: "Journal", Icon: BookHeart },
];

const TOP_ITEMS: NavItem[] = [
  { id: "dashboard", label: "Dashboard", Icon: LayoutDashboard },
  { id: "quests", label: "Quests", Icon: ScrollText },
  { id: "shop", label: "Shop", Icon: Store },
];

const AGENT_ITEM: NavItem = { id: "agent", label: "Agent", Icon: Bot };

export function Logo({ collapsed = false }: { collapsed?: boolean }) {
  return (
    <div className="flex items-center gap-2.5">
      <div
        className="grid h-8 w-8 shrink-0 place-items-center rounded-sm border font-display text-lg leading-none"
        style={{
          borderColor: "var(--color-phos)",
          color: "var(--color-phos)",
          boxShadow: "0 0 14px -4px rgba(75,255,126,0.8), inset 0 0 10px rgba(75,255,126,0.12)",
        }}
      >
        &gt;_
      </div>
      {!collapsed && (
        <div>
          <div
            className="cursor-blink font-display text-xl leading-none text-ink"
            style={{ color: "var(--color-phos)" }}
          >
            edi
          </div>
          <div className="text-[9px] uppercase tracking-[0.3em] text-faint">life-rpg terminal</div>
        </div>
      )}
    </div>
  );
}

const goldStyle = (active: boolean): CSSProperties => ({
  background: active ? "rgba(255,176,0,0.08)" : "transparent",
  color: active ? "var(--color-goldhi)" : "var(--color-muted)",
});

const tealStyle = (active: boolean): CSSProperties => ({
  background: active ? "rgba(46,230,200,0.10)" : "transparent",
  color: active ? "var(--color-spirituality)" : "var(--color-faint)",
});

export function Sidebar({
  view,
  setView,
  collapsed,
  onToggle,
}: {
  view: View;
  setView: (v: View) => void;
  collapsed: boolean;
  onToggle: () => void;
}) {
  const inToolsGroup = view === "moodlog" || view === "journal";
  const [toolsOpen, setToolsOpen] = useState(false);
  const showChildren = toolsOpen || inToolsGroup;

  const navBtn = (
    { id, label, Icon }: NavItem,
    styleFor: (active: boolean) => CSSProperties,
    testId: string,
  ) => {
    const active = view === id;
    return (
      <button
        key={id}
        onClick={() => setView(id)}
        data-testid={testId}
        title={collapsed ? label : undefined}
        className={`group relative flex w-full items-center rounded-lg py-2.5 text-sm font-medium transition-colors ${
          collapsed ? "justify-center px-0" : "gap-3 px-3"
        }`}
        style={styleFor(active)}
      >
        {active && (
          <span
            className="absolute left-0 top-1/2 h-5 w-1 -translate-y-1/2 rounded-r-full"
            style={{ background: "var(--color-gold)" }}
          />
        )}
        <Icon size={18} className="transition-transform group-hover:scale-110" />
        {!collapsed && label}
      </button>
    );
  };

  return (
    <aside
      className={`sticky top-0 hidden h-screen shrink-0 flex-col border-r border-edge py-6 transition-[width] duration-200 lg:flex ${
        collapsed ? "w-14 px-2" : "w-60 px-5"
      }`}
    >
      <div className={collapsed ? "flex justify-center" : undefined}>
        <Logo collapsed={collapsed} />
      </div>
      <nav className="mt-10 flex flex-1 flex-col gap-1">
        {TOP_ITEMS.map((item) => navBtn(item, goldStyle, `nav-${item.id}`))}

        {collapsed ? (
          // Rail: the Tools group flattens — its children become direct icons.
          TOOL_CHILDREN.map((item) => navBtn(item, tealStyle, `nav-tool-${item.id}`))
        ) : (
          <>
            {/* Tools: collapsible group, no page of its own */}
            <button
              onClick={() => setToolsOpen((o) => !o)}
              data-testid="nav-tools"
              aria-expanded={showChildren}
              className="group relative flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors"
              style={goldStyle(inToolsGroup)}
            >
              {inToolsGroup && (
                <span
                  className="absolute left-0 top-1/2 h-5 w-1 -translate-y-1/2 rounded-r-full"
                  style={{ background: "var(--color-gold)" }}
                />
              )}
              <Wrench size={18} className="transition-transform group-hover:scale-110" />
              Tools
              <ChevronDown
                size={14}
                className="ml-auto transition-transform"
                style={{ transform: showChildren ? "rotate(180deg)" : "none" }}
              />
            </button>
            {showChildren && (
              <div className="ml-4 flex flex-col gap-0.5 border-l border-edge pl-3">
                {TOOL_CHILDREN.map(({ id, label, Icon }) => {
                  const active = view === id;
                  return (
                    <button
                      key={id}
                      onClick={() => setView(id)}
                      data-testid={`nav-tool-${id}`}
                      className="flex items-center gap-2.5 rounded-md px-2.5 py-2 text-[13px] font-medium transition-colors"
                      style={tealStyle(active)}
                    >
                      <Icon size={15} />
                      {label}
                    </button>
                  );
                })}
              </div>
            )}
          </>
        )}

        {navBtn(AGENT_ITEM, goldStyle, "nav-agent")}
      </nav>

      {!collapsed && (
        <div className="rounded-xl border border-edge bg-white/[0.02] p-3">
          <div className="text-[11px] font-medium text-muted">Single-user mode</div>
          <div className="mt-0.5 text-[10px] text-faint">Self-hosted · SQLite</div>
        </div>
      )}
      <button
        onClick={onToggle}
        data-testid="sidebar-toggle"
        title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
        className="mt-3 flex w-full items-center justify-center rounded-lg border border-edge py-2 text-faint transition-colors hover:text-muted"
      >
        {collapsed ? <ChevronsRight size={16} /> : <ChevronsLeft size={16} />}
      </button>
    </aside>
  );
}
```

- [ ] **Step 2: Add collapsed state + persistence to `client/src/App.tsx`**

Change the react import and the top of the component; pass the new props:

```tsx
import { useCallback, useState } from "react";
```

Inside `App`, replace `const [view, setView] = useState<View>("dashboard");` block with:

```tsx
const [view, setView] = useState<View>("dashboard");
const [collapsed, setCollapsed] = useState<boolean>(() => {
  try {
    return localStorage.getItem("edi.sidebarCollapsed") === "1";
  } catch {
    return false;
  }
});
const toggleSidebar = useCallback(() => {
  setCollapsed((c) => {
    const next = !c;
    try {
      localStorage.setItem("edi.sidebarCollapsed", next ? "1" : "0");
    } catch {
      // private mode / quota — keep in-memory state only
    }
    return next;
  });
}, []);
```

And change the `<Sidebar ... />` element to:

```tsx
<Sidebar view={view} setView={setView} collapsed={collapsed} onToggle={toggleSidebar} />
```

- [ ] **Step 3: Build**

Run: `cd client && npm run build`
Expected: PASS.

- [ ] **Step 4: Browser check**

On `http://localhost:5173` (desktop width):
- Click `sidebar-toggle`: sidebar animates to the narrow rail; labels, wordmark, Tools group, and footer card disappear; Mood Log + Journal icons appear directly in the rail.
- Hover a rail icon: native tooltip shows the label.
- Click the Journal rail icon: Journal page loads, icon shows teal tint + gold left bar.
- Click `sidebar-toggle` again: full sidebar returns, Tools group auto-open (Journal active).
- Reload while collapsed: rail is still collapsed (persistence). `localStorage.getItem("edi.sidebarCollapsed")` is `"1"`.
- Console clean.

- [ ] **Step 5: Commit**

```bash
git add client/src/components/Sidebar.tsx client/src/App.tsx
git commit -m "feat(client): collapsible sidebar — icon rail with toggle + persistence"
```

---

### Task 3: Cmd/Ctrl+B keyboard shortcut

**Files:**
- Modify: `client/src/App.tsx`

**Interfaces:**
- Consumes: `toggleSidebar` from Task 2.
- Produces: nothing new for later tasks.

- [ ] **Step 1: Add the global keydown listener**

Change the react import to include `useEffect`:

```tsx
import { useCallback, useEffect, useState } from "react";
```

Below `toggleSidebar` in `App`, add:

```tsx
useEffect(() => {
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key.toLowerCase() !== "b" || !(e.metaKey || e.ctrlKey) || e.altKey || e.shiftKey) return;
    const t = e.target;
    if (
      t instanceof HTMLElement &&
      (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable)
    ) {
      return; // never toggle mid-typing (e.g. journal entry)
    }
    e.preventDefault();
    toggleSidebar();
  };
  window.addEventListener("keydown", onKeyDown);
  return () => window.removeEventListener("keydown", onKeyDown);
}, [toggleSidebar]);
```

- [ ] **Step 2: Build**

Run: `cd client && npm run build`
Expected: PASS.

- [ ] **Step 3: Browser check**

- Press Cmd+B (Mac) on the dashboard: sidebar toggles. Press again: toggles back.
- Open Journal → focus the entry textarea → press Cmd+B: sidebar does NOT toggle, no character issues.
- Console clean.

- [ ] **Step 4: Commit**

```bash
git add client/src/App.tsx
git commit -m "feat(client): Cmd/Ctrl+B toggles the sidebar (ignored while typing)"
```

---

### Task 4: Full validation pass (spec checklist)

**Files:** none (verification only; fix-forward if anything fails, then re-run).

- [ ] **Step 1: Build**

Run: `cd client && npm run build`
Expected: PASS.

- [ ] **Step 2: Full browser pass against `make dev`**

Desktop viewport (≥1024px wide), using the agent-browser skill; walk the spec's validation list end to end:

1. Toggle via button: rail shrinks to ~56px (`w-14`), labels disappear; expand restores 240px.
2. Toggle via Cmd/Ctrl+B; then confirm it does NOT fire while focus is in the journal textarea.
3. Active states correct in both modes for a top item (Quests) and a tool child (Journal), including the gold/teal tints and left accent bar.
4. Tools group behavior in expanded mode unchanged (chevron, children indent, auto-open when a child is active).
5. Reload while collapsed → stays collapsed.
6. Narrow the viewport below `lg`: mobile top bar + bottom tabs render exactly as before; no rail artifacts.
7. Browser console clean throughout (no errors/warnings).

- [ ] **Step 3: Capture evidence**

Take a screenshot of the collapsed rail and one of the expanded sidebar; report them with the validation summary.

- [ ] **Step 4: Commit (only if fixes were needed)**

```bash
git add -A client/src
git commit -m "fix(client): sidebar collapse polish from validation pass"
```
