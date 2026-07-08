import { useState } from "react";
import { Bot, BookHeart, BrainCircuit, ChevronDown, LayoutDashboard, ScrollText, Wrench } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { DashboardPage } from "./pages/Dashboard";
import { QuestsPage } from "./pages/Quests";
import { JournalPage } from "./pages/Journal";
import { SuggestionsPage } from "./pages/Suggestions";
import { ToolsPage } from "./pages/Tools";

type View = "dashboard" | "quests" | "tools" | "journal" | "agent";

// Top-level nav. "Tools" is a group whose children (Daily Mood Log, Journal) show
// as a dropdown in the sidebar; on mobile the Tools overview page covers them.
const NAV: { id: View; label: string; Icon: LucideIcon }[] = [
  { id: "dashboard", label: "Dashboard", Icon: LayoutDashboard },
  { id: "quests", label: "Quests", Icon: ScrollText },
  { id: "tools", label: "Tools", Icon: Wrench },
  { id: "agent", label: "Agent", Icon: Bot },
];

const TOOL_CHILDREN: { id: string; label: string; Icon: LucideIcon }[] = [
  { id: "daily_mood_log", label: "Daily Mood Log", Icon: BrainCircuit },
  { id: "journal", label: "Journal", Icon: BookHeart },
];

function Logo() {
  return (
    <div className="flex items-center gap-2.5">
      <div
        className="grid h-8 w-8 place-items-center"
        style={{
          background: "linear-gradient(160deg, var(--color-goldhi), var(--color-gold))",
          clipPath: "polygon(50% 0, 100% 50%, 50% 100%, 0 50%)",
          boxShadow: "0 0 18px -4px rgba(244,183,64,0.8)",
        }}
      >
        <div className="h-1.5 w-1.5 rounded-full bg-[#1a1305]" />
      </div>
      <div>
        <div className="font-display text-base font-bold leading-none tracking-[0.16em] text-ink">
          edi
        </div>
        <div className="text-[9px] uppercase tracking-[0.3em] text-faint">Life RPG</div>
      </div>
    </div>
  );
}

export default function App() {
  const [view, setView] = useState<View>("dashboard");
  // Which tool is open on the Tools page (null = overview).
  const [activeTool, setActiveTool] = useState<string | null>(null);

  const inToolsGroup = view === "tools" || view === "journal";
  const openToolChild = (id: string) => {
    if (id === "journal") {
      setView("journal");
    } else {
      setActiveTool(id);
      setView("tools");
    }
  };

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-[1240px] flex-col lg:flex-row">
      {/* Sidebar (desktop) */}
      <aside className="sticky top-0 hidden h-screen w-60 shrink-0 flex-col border-r border-edge px-5 py-6 lg:flex">
        <Logo />
        <nav className="mt-10 flex flex-1 flex-col gap-1">
          {NAV.map(({ id, label, Icon }) => {
            const isTools = id === "tools";
            const active = isTools ? inToolsGroup : view === id;
            return (
              <div key={id}>
                <button
                  onClick={() => {
                    if (isTools) setActiveTool(null); // land on the overview
                    setView(id);
                  }}
                  data-testid={`nav-${id}`}
                  className="group relative flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors"
                  style={{
                    background: active ? "rgba(244,183,64,0.08)" : "transparent",
                    color: active ? "var(--color-goldhi)" : "var(--color-muted)",
                  }}
                >
                  {active && (
                    <span
                      className="absolute left-0 top-1/2 h-5 w-1 -translate-y-1/2 rounded-r-full"
                      style={{ background: "var(--color-gold)" }}
                    />
                  )}
                  <Icon size={18} className="transition-transform group-hover:scale-110" />
                  {label}
                  {isTools && (
                    <ChevronDown
                      size={14}
                      className="ml-auto transition-transform"
                      style={{ transform: inToolsGroup ? "rotate(180deg)" : "none" }}
                    />
                  )}
                </button>

                {/* Tools dropdown */}
                {isTools && inToolsGroup && (
                  <div className="ml-4 mt-0.5 flex flex-col gap-0.5 border-l border-edge pl-3">
                    {TOOL_CHILDREN.map((child) => {
                      const childActive =
                        child.id === "journal"
                          ? view === "journal"
                          : view === "tools" && activeTool === child.id;
                      const CIcon = child.Icon;
                      return (
                        <button
                          key={child.id}
                          onClick={() => openToolChild(child.id)}
                          data-testid={`nav-tool-${child.id}`}
                          className="flex items-center gap-2.5 rounded-md px-2.5 py-2 text-[13px] font-medium transition-colors"
                          style={{
                            background: childActive ? "rgba(69,224,208,0.10)" : "transparent",
                            color: childActive ? "var(--color-spirituality)" : "var(--color-faint)",
                          }}
                        >
                          <CIcon size={15} />
                          {child.label}
                        </button>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
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
        {view === "tools" && (
          <ToolsPage
            activeTool={activeTool}
            onOpenTool={setActiveTool}
            onClose={() => setActiveTool(null)}
            onOpenJournal={() => setView("journal")}
          />
        )}
        {view === "journal" && <JournalPage />}
        {view === "agent" && <SuggestionsPage />}
      </main>

      {/* Mobile bottom nav (Journal lives inside Tools here) */}
      <nav className="fixed inset-x-0 bottom-0 z-30 flex items-center justify-around border-t border-edge bg-[var(--color-abyss)]/95 px-2 py-2 backdrop-blur lg:hidden">
        {NAV.map(({ id, label, Icon }) => {
          const active = id === "tools" ? inToolsGroup : view === id;
          return (
            <button
              key={id}
              onClick={() => {
                if (id === "tools") setActiveTool(null);
                setView(id);
              }}
              className="flex flex-1 flex-col items-center gap-0.5 rounded-lg py-1.5 text-[10px] font-medium"
              style={{ color: active ? "var(--color-goldhi)" : "var(--color-faint)" }}
            >
              <Icon size={20} />
              {label}
            </button>
          );
        })}
      </nav>
    </div>
  );
}
