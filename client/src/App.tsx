import { useState } from "react";
import { Bot, BookHeart, BrainCircuit, ChevronDown, LayoutDashboard, ScrollText, Store, Wrench } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { DashboardPage } from "./pages/Dashboard";
import { QuestsPage } from "./pages/Quests";
import { JournalPage } from "./pages/Journal";
import { SuggestionsPage } from "./pages/Suggestions";
import { ShopPage } from "./pages/Shop";
import { DailyMoodLog } from "./components/DailyMoodLog";

type View = "dashboard" | "quests" | "shop" | "moodlog" | "journal" | "agent";

// "Tools" is not a page — in the sidebar it's a collapsible group whose children
// (Daily Mood Log, Journal) are the actual destinations.
const TOOL_CHILDREN: { id: View; label: string; Icon: LucideIcon }[] = [
  { id: "moodlog", label: "Daily Mood Log", Icon: BrainCircuit },
  { id: "journal", label: "Journal", Icon: BookHeart },
];

function Logo() {
  return (
    <div className="flex items-center gap-2.5">
      <div
        className="grid h-8 w-8 place-items-center rounded-sm border font-display text-lg leading-none"
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

export default function App() {
  const [view, setView] = useState<View>("dashboard");
  const inToolsGroup = view === "moodlog" || view === "journal";
  const [toolsOpen, setToolsOpen] = useState(false);
  const showChildren = toolsOpen || inToolsGroup;

  const navBtnStyle = (active: boolean) => ({
    background: active ? "rgba(255,176,0,0.08)" : "transparent",
    color: active ? "var(--color-goldhi)" : "var(--color-muted)",
  });

  const topItem = (id: View, label: string, Icon: LucideIcon) => (
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
    <div className="mx-auto flex min-h-screen w-full max-w-[1240px] flex-col lg:flex-row">
      {/* Sidebar (desktop) */}
      <aside className="sticky top-0 hidden h-screen w-60 shrink-0 flex-col border-r border-edge px-5 py-6 lg:flex">
        <Logo />
        <nav className="mt-10 flex flex-1 flex-col gap-1">
          {topItem("dashboard", "Dashboard", LayoutDashboard)}
          {topItem("quests", "Quests", ScrollText)}
          {topItem("shop", "Shop", Store)}

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

          {topItem("agent", "Agent", Bot)}
        </nav>
        <div className="rounded-xl border border-edge bg-white/[0.02] p-3">
          <div className="text-[11px] font-medium text-muted">Single-user mode</div>
          <div className="mt-0.5 text-[10px] text-faint">Self-hosted · SQLite</div>
        </div>
      </aside>

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
