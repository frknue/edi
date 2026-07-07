import { useState } from "react";
import { motion } from "framer-motion";
import { BrainCircuit, ChevronRight, History } from "lucide-react";
import { useTools, useToolEntries } from "../lib/queries";
import { DailyMoodLog } from "../components/DailyMoodLog";
import { EmptyState, RewardChips, SectionTitle, Spinner } from "../components/ui";
import { relativeTime } from "../lib/format";
import type { ToolDefinition } from "../lib/types";

export function ToolsPage() {
  const { data: tools, isLoading } = useTools();
  const [active, setActive] = useState<string | null>(null);

  if (active === "daily_mood_log") {
    return <DailyMoodLog onClose={() => setActive(null)} />;
  }

  return (
    <div className="space-y-5">
      <div>
        <h1 className="font-display text-xl font-bold tracking-tight text-ink">Tools</h1>
        <p className="text-sm text-faint">Guided exercises for real change — each one earns XP.</p>
      </div>

      {isLoading ? (
        <Spinner />
      ) : !tools || tools.length === 0 ? (
        <EmptyState title="No tools yet" />
      ) : (
        <div className="grid grid-cols-1 gap-4">
          {tools.map((t, i) => (
            <ToolCard key={t.key} tool={t} index={i} onOpen={() => setActive(t.key)} />
          ))}
        </div>
      )}
    </div>
  );
}

function ToolCard({ tool, index, onOpen }: { tool: ToolDefinition; index: number; onOpen: () => void }) {
  const { data: entries } = useToolEntries(tool.key);
  const recent = entries?.slice(0, 3) ?? [];

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, delay: index * 0.05, ease: [0.16, 1, 0.3, 1] }}
      className="hud-panel clip-corner overflow-hidden"
    >
      <button onClick={onOpen} data-testid={`tool-${tool.key}`} className="group w-full p-5 text-left">
        <div className="flex items-start gap-4">
          <div
            className="grid h-11 w-11 shrink-0 place-items-center rounded-xl"
            style={{ background: "rgba(69,224,208,0.14)", color: "var(--color-spirituality)" }}
          >
            <BrainCircuit size={22} />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <h2 className="text-lg font-semibold text-ink">{tool.name}</h2>
              <ChevronRight size={16} className="text-faint transition-transform group-hover:translate-x-0.5" />
            </div>
            <div className="text-[11px] uppercase tracking-wider text-faint">{tool.tagline}</div>
            <p className="mt-2 text-sm text-muted">{tool.description}</p>
            <div className="mt-3 flex items-center gap-2">
              <span className="text-[11px] text-faint">Rewards</span>
              <RewardChips rewards={tool.attribute_rewards} />
            </div>
          </div>
        </div>
      </button>

      {recent.length > 0 && (
        <div className="border-t border-edge px-5 py-3">
          <SectionTitle>
            <span className="flex items-center gap-1.5 text-[11px]">
              <History size={12} /> Recent
            </span>
          </SectionTitle>
          <ul className="space-y-1.5">
            {recent.map((e) => (
              <li key={e.id} className="flex items-center justify-between text-xs">
                <span className="truncate text-muted">{e.summary || "Completed"}</span>
                <span className="tabnum shrink-0 text-faint">{relativeTime(e.created_at)}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </motion.div>
  );
}
