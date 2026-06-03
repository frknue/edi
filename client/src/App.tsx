import { useState } from "react";
import { Bot, BookHeart, LayoutDashboard, ScrollText } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { DashboardPage } from "./pages/Dashboard";
import { QuestsPage } from "./pages/Quests";
import { JournalPage } from "./pages/Journal";
import { SuggestionsPage } from "./pages/Suggestions";

type View = "dashboard" | "quests" | "journal" | "agent";

const NAV: { id: View; label: string; Icon: LucideIcon }[] = [
  { id: "dashboard", label: "Dashboard", Icon: LayoutDashboard },
  { id: "quests", label: "Quests", Icon: ScrollText },
  { id: "journal", label: "Journal", Icon: BookHeart },
  { id: "agent", label: "Agent", Icon: Bot },
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
          ASCEND
        </div>
        <div className="text-[9px] uppercase tracking-[0.3em] text-faint">Life RPG</div>
      </div>
    </div>
  );
}

export default function App() {
  const [view, setView] = useState<View>("dashboard");

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-[1240px] flex-col lg:flex-row">
      {/* Sidebar (desktop) */}
      <aside className="sticky top-0 hidden h-screen w-60 shrink-0 flex-col border-r border-edge px-5 py-6 lg:flex">
        <Logo />
        <nav className="mt-10 flex flex-1 flex-col gap-1">
          {NAV.map(({ id, label, Icon }) => {
            const active = view === id;
            return (
              <button
                key={id}
                onClick={() => setView(id)}
                data-testid={`nav-${id}`}
                className="group relative flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors"
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
              </button>
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
        {view === "dashboard" && <DashboardPage onGoToQuests={() => setView("quests")} />}
        {view === "quests" && <QuestsPage />}
        {view === "journal" && <JournalPage />}
        {view === "agent" && <SuggestionsPage />}
      </main>

      {/* Mobile bottom nav */}
      <nav className="fixed inset-x-0 bottom-0 z-30 flex items-center justify-around border-t border-edge bg-[var(--color-abyss)]/95 px-2 py-2 backdrop-blur lg:hidden">
        {NAV.map(({ id, label, Icon }) => {
          const active = view === id;
          return (
            <button
              key={id}
              onClick={() => setView(id)}
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
