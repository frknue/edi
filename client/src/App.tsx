import { useCallback, useEffect, useState } from "react";
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

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-[1240px] flex-col lg:flex-row">
      <Sidebar view={view} setView={setView} collapsed={collapsed} onToggle={toggleSidebar} />

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
