import { motion } from "framer-motion";
import { Check, Sparkles, X } from "lucide-react";
import type { AgentSuggestion } from "../lib/types";
import { Btn, RewardChips, TypeBadge } from "./ui";

const typeLabel: Record<string, string> = {
  low_attribute: "Rebalance",
  level_up_focus: "Level Up",
  make_easier: "Make Easier",
  recovery: "Recovery",
};

export function SuggestionCard({
  suggestion,
  index = 0,
  onAccept,
  onDismiss,
  busy,
}: {
  suggestion: AgentSuggestion;
  index?: number;
  onAccept?: (id: number) => void;
  onDismiss?: (id: number) => void;
  busy?: boolean;
}) {
  const q = suggestion.suggested_quest;
  const resolved = suggestion.status !== "pending";
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, delay: index * 0.05, ease: [0.16, 1, 0.3, 1] }}
      className={`hud-panel relative overflow-hidden p-4 ${resolved ? "opacity-60" : "hud-panel-hover"}`}
    >
      <div
        className="pointer-events-none absolute -left-8 -top-8 h-24 w-24 rounded-full"
        style={{ background: "radial-gradient(circle, rgba(185,138,255,0.22), transparent 70%)" }}
      />
      <div className="relative">
        <div className="mb-2 flex items-center gap-2">
          <span
            className="inline-flex items-center gap-1 rounded-md px-2 py-0.5 font-display text-[10px] font-semibold uppercase tracking-wider"
            style={{ background: "rgba(185,138,255,0.14)", color: "#b98aff" }}
          >
            <Sparkles size={11} />
            {typeLabel[suggestion.type] ?? "Agent"}
          </span>
          {resolved && (
            <span className="text-[10px] uppercase tracking-wide text-faint">{suggestion.status}</span>
          )}
        </div>

        <h3 className="text-[15px] font-semibold text-ink">{suggestion.title}</h3>
        <p className="mt-1 text-xs leading-relaxed text-muted">{suggestion.reason}</p>

        <div className="mt-3 rounded-lg border border-edge bg-white/[0.02] p-2.5">
          <div className="mb-1.5 flex items-center justify-between gap-2">
            <span className="truncate text-[13px] font-medium text-ink">{q.title}</span>
            <TypeBadge type={q.type} />
          </div>
          <RewardChips rewards={q.attribute_rewards} />
        </div>

        {!resolved && (onAccept || onDismiss) && (
          <div className="mt-3.5 flex items-center gap-2">
            {onAccept && (
              <Btn
                variant="primary"
                className="flex-1"
                disabled={busy}
                onClick={() => onAccept(suggestion.id)}
                data-testid={`accept-suggestion-${suggestion.id}`}
              >
                <Check size={15} /> Accept &amp; add quest
              </Btn>
            )}
            {onDismiss && (
              <Btn variant="soft" disabled={busy} onClick={() => onDismiss(suggestion.id)}>
                <X size={15} /> Dismiss
              </Btn>
            )}
          </div>
        )}
      </div>
    </motion.div>
  );
}
