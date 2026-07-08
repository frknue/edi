import { motion } from "framer-motion";
import { Archive, Check, CheckCircle2, Pencil, SkipForward, Square, SquareCheckBig } from "lucide-react";
import type { Quest } from "../lib/types";
import { getType } from "../lib/theme";
import { useToggleSubtask } from "../lib/queries";
import { Btn, DifficultyPips, RewardChips, TypeBadge } from "./ui";

interface QuestCardProps {
  quest: Quest;
  index?: number;
  onComplete?: (id: number) => void;
  onSkip?: (id: number) => void;
  onArchive?: (id: number) => void;
  onEdit?: (quest: Quest) => void;
  busy?: boolean;
}

export function QuestCard({
  quest,
  index = 0,
  onComplete,
  onSkip,
  onArchive,
  onEdit,
  busy,
}: QuestCardProps) {
  const meta = getType(quest.type);
  const isBoss = quest.type === "boss";
  const isRecovery = quest.type === "recovery";
  const isActive = quest.status === "active";
  const isDone = quest.status === "completed";

  const accent = meta.color;

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, delay: index * 0.04, ease: [0.16, 1, 0.3, 1] }}
      className={`hud-panel relative overflow-hidden ${isBoss ? "boss-glow" : "hud-panel-hover"} ${
        !isActive ? "opacity-60" : ""
      }`}
      style={
        isRecovery
          ? { background: "linear-gradient(180deg, rgba(69,224,208,0.06), rgba(255,255,255,0)), var(--color-panel)" }
          : undefined
      }
    >
      {/* left accent rail */}
      <div className="absolute inset-y-0 left-0 w-1" style={{ background: accent }} />

      <div className="p-4 pl-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 flex-1">
            <div className="mb-1.5 flex items-center gap-2">
              <TypeBadge type={quest.type} />
              {isRecovery && (
                <span className="text-[10px] italic text-[var(--color-spirituality)]">
                  rest counts too
                </span>
              )}
              {isDone && (
                <span className="inline-flex items-center gap-1 text-[10px] text-[var(--color-health)]">
                  <CheckCircle2 size={11} /> done
                </span>
              )}
            </div>
            <h3
              className={`truncate text-[15px] font-semibold ${
                isBoss ? "font-display tracking-tight" : ""
              } text-ink`}
              style={isBoss ? { color: "var(--color-boss)" } : undefined}
            >
              {quest.title}
            </h3>
            {quest.description && (
              <p className="mt-0.5 line-clamp-2 text-xs text-muted">{quest.description}</p>
            )}
          </div>
        </div>

        <div className="mt-3 flex items-center justify-between gap-2">
          <DifficultyPips difficulty={quest.difficulty} />
          <RewardChips rewards={quest.attribute_rewards} />
        </div>

        {quest.subtasks.length > 0 && <SubtaskList quest={quest} interactive={isActive} />}

        {(onComplete || onSkip || onArchive || onEdit) && isActive && (
          <div className="mt-4 flex items-center gap-2">
            {onComplete && (
              <Btn
                variant="primary"
                className="flex-1"
                disabled={busy}
                onClick={() => onComplete(quest.id)}
                data-testid={`complete-${quest.id}`}
              >
                <Check size={16} />
                Complete
              </Btn>
            )}
            {onEdit && (
              <Btn variant="ghost" onClick={() => onEdit(quest)} aria-label="Edit quest">
                <Pencil size={15} />
              </Btn>
            )}
            {onSkip && (
              <Btn variant="soft" disabled={busy} onClick={() => onSkip(quest.id)} aria-label="Skip quest">
                <SkipForward size={15} />
              </Btn>
            )}
            {onArchive && (
              <Btn variant="soft" disabled={busy} onClick={() => onArchive(quest.id)} aria-label="Archive quest">
                <Archive size={15} />
              </Btn>
            )}
          </div>
        )}
      </div>
    </motion.div>
  );
}

// SubtaskList renders a quest's bonus objectives. While the quest is active the
// checkboxes toggle via the API; afterwards they show frozen state.
function SubtaskList({ quest, interactive }: { quest: Quest; interactive: boolean }) {
  const toggle = useToggleSubtask();
  return (
    <div className="mt-3 space-y-1 rounded-lg border border-edge/70 bg-white/[0.015] p-2">
      <div className="px-1 font-display text-[9px] uppercase tracking-[0.18em] text-faint">
        Bonus objectives
      </div>
      {quest.subtasks.map((st) => {
        const Icon = st.done ? SquareCheckBig : Square;
        return (
          <button
            key={st.id}
            disabled={!interactive || toggle.isPending}
            onClick={(e) => {
              e.stopPropagation();
              toggle.mutate({ questId: quest.id, subtaskId: st.id });
            }}
            data-testid={`subtask-${st.id}`}
            className={`flex w-full items-center gap-2 rounded-md px-1.5 py-1 text-left transition-colors ${
              interactive ? "hover:bg-white/[0.04]" : "cursor-default"
            }`}
          >
            <Icon
              size={14}
              className="shrink-0"
              style={{ color: st.done ? "var(--color-health)" : "var(--color-faint)" }}
            />
            <span
              className={`min-w-0 flex-1 truncate text-xs ${st.done ? "text-ink" : "text-muted"}`}
              style={st.done ? { textDecoration: "none" } : undefined}
            >
              {st.title}
            </span>
            <RewardChips rewards={st.attribute_rewards} />
          </button>
        );
      })}
    </div>
  );
}
