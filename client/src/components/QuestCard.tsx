import { useEffect, useRef, useState } from "react";
import { motion } from "framer-motion";
import { AlarmClock, Archive, Check, CheckCircle2, Circle, CornerDownRight, Pencil, Play, RotateCcw, Scissors, SkipForward, Sparkles, Square, SquareCheckBig, Users } from "lucide-react";
import type { Quest } from "../lib/types";
import { getType } from "../lib/theme";
import { useBreakDownQuest, useOpenAIStatus, useShrinkQuest, useToggleSubtask } from "../lib/queries";
import { pushToast } from "../lib/toast";
import { Btn, DifficultyPips, RewardChips, TypeBadge } from "./ui";
import { useI18n } from "../lib/i18n";
import type { MessageKey } from "../lib/locales/en";

interface QuestCardProps {
  quest: Quest;
  index?: number;
  onComplete?: (id: number) => void;
  onStart?: (id: number) => void; // active quest mode: open a timed session
  running?: boolean; // this quest is the running session
  onSkip?: (id: number) => void;
  onArchive?: (id: number) => void;
  onRestore?: (id: number) => void;
  onEdit?: (quest: Quest) => void;
  busy?: boolean;
}

export function QuestCard({
  quest,
  index = 0,
  onComplete,
  onStart,
  running = false,
  onSkip,
  onArchive,
  onRestore,
  onEdit,
  busy,
}: QuestCardProps) {
  const { t } = useI18n();
  const meta = getType(quest.type);
  const isBoss = quest.type === "boss";
  const isRecovery = quest.type === "recovery";
  const isActive = quest.status === "active";
  const isDone = quest.status === "completed";
  const isShared = quest.shared_quest_id !== undefined;
  const myActive = isShared ? quest.assigned_to_me && quest.my_status === "active" : isActive;
  const myDone = isShared && quest.assigned_to_me && quest.my_status === "completed";
  const canEdit = !isShared || !quest.assignees.some((a) => a.status === "completed");
  const canRestore = isShared
    ? quest.assigned_to_me && (quest.my_status === "skipped" || quest.my_status === "archived")
    : quest.status === "skipped" || quest.status === "archived";

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
        running
          ? { borderColor: "var(--color-phos)", boxShadow: "0 0 18px -6px rgba(var(--phos-rgb),0.7)" }
          : isRecovery
            ? { background: "linear-gradient(180deg, rgba(46,230,200,0.06), rgba(255,255,255,0)), var(--color-panel)" }
            : undefined
      }
      data-running={running ? "1" : "0"}
    >
      {/* left accent rail */}
      <div className="absolute inset-y-0 left-0 w-1" style={{ background: accent }} />

      <div className="p-4 pl-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 flex-1">
            <div className="mb-1.5 flex items-center gap-2">
              <TypeBadge type={quest.type} />
              {isShared && (
                <span className="inline-flex items-center gap-1 text-[10px] text-[var(--color-relationships)]">
                  <Users size={11} /> {t("quest.shared")}
                </span>
              )}
              {isRecovery && (
                <span className="text-[10px] italic text-[var(--color-spirituality)]">
                  {t("quest.restCounts")}
                </span>
              )}
              {isDone && (
                <span className="inline-flex items-center gap-1 text-[10px] text-[var(--color-health)]">
                  <CheckCircle2 size={11} /> {t("quest.done")}
                </span>
              )}
              {running && (
                <span className="inline-flex items-center gap-1 font-display text-[10px] uppercase tracking-wider text-[var(--color-phos)]" data-testid={`running-${quest.id}`}>
                  <Play size={10} /> {t("quest.running")}
                </span>
              )}
              {myDone && !quest.all_completed && (
                <span className="inline-flex items-center gap-1 text-[10px] text-[var(--color-focus)]">
                  <CheckCircle2 size={11} /> {t("quest.waitingForOthers")}
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
          {/* Secondary actions live up here, where a card always has room;
              the footer keeps the two big verbs (Start / Complete). */}
          {isActive && ((onEdit && canEdit) || onSkip || onArchive) && (
            <div className="flex shrink-0 items-center gap-1" data-testid={`quest-actions-${quest.id}`}>
              {onEdit && canEdit && (
                <button type="button" className="icon-action" onClick={() => onEdit(quest)} aria-label={t("quest.edit")} title={t("quest.edit")}>
                  <Pencil size={14} />
                </button>
              )}
              {onSkip && (
                <button type="button" className="icon-action" disabled={busy} onClick={() => onSkip(quest.id)} aria-label={t("quest.skip")} title={t("quest.skip")}>
                  <SkipForward size={14} />
                </button>
              )}
              {onArchive && (
                <button type="button" className="icon-action" disabled={busy} onClick={() => onArchive(quest.id)} aria-label={t("quest.archive")} title={t("quest.archive")}>
                  <Archive size={14} />
                </button>
              )}
            </div>
          )}
        </div>

        <div className="mt-3 flex items-center justify-between gap-2">
          <DifficultyPips difficulty={quest.difficulty} />
          <div className="flex items-center gap-2">
            {typeof quest.projected_xp === "number" && quest.projected_xp > 0 && isActive && (
              <span
                className="tabnum whitespace-nowrap rounded-full border px-2 py-0.5 text-[10px] font-semibold"
                style={{ borderColor: "rgba(var(--gold-rgb),0.45)", color: "var(--color-goldhi)" }}
                title={t("dash.paysNowTitle")}
                data-testid={`pays-${quest.id}`}
              >
                {t("dash.paysNow", { xp: quest.projected_xp })}
              </span>
            )}
            <RewardChips rewards={quest.attribute_rewards} />
          </div>
        </div>

        {isBoss && quest.subtasks.length > 0 && <BossHP quest={quest} />}

        {isShared && (
          <div className="mt-2 flex flex-wrap gap-1" data-testid={`assignees-${quest.id}`}>
            {quest.assignees.map((assignee) => {
              const AssigneeIcon = assignee.status === "completed" ? CheckCircle2 : Circle;
              return (
                <span
                  key={assignee.user_id}
                  className="inline-flex items-center gap-1 rounded-full border border-edge bg-white/[0.02] px-2 py-0.5 text-[10px] text-muted"
                  title={t(`status.${assignee.status}` as MessageKey)}
                >
                  <AssigneeIcon
                    size={10}
                    style={{ color: assignee.status === "completed" ? "var(--color-health)" : "var(--color-faint)" }}
                  />
                  {assignee.name}
                </span>
              );
            })}
          </div>
        )}

        {(quest.trigger || quest.trigger_at) && isActive && (
          <div className="mt-2 flex items-center gap-1.5 text-[11px] text-muted" data-testid={`trigger-${quest.id}`}>
            <AlarmClock size={12} style={{ color: "var(--color-focus)" }} />
            <span>
              {quest.trigger_at && <span className="tabnum font-semibold text-ink">{quest.trigger_at}</span>}
              {quest.trigger_at && quest.trigger && " · "}
              {quest.trigger}
            </span>
          </div>
        )}

        {quest.resume_note && isActive && (
          <div className="mt-2 flex items-start gap-1.5 text-xs text-muted" data-testid={`resume-${quest.id}`}>
            <CornerDownRight size={12} className="mt-0.5 shrink-0" style={{ color: "var(--color-phos)" }} />
            <span>
              <span className="font-display text-[9px] uppercase tracking-[0.18em] text-faint">{t("quest.resume")} </span>
              {quest.resume_note}
            </span>
          </div>
        )}

        {quest.subtasks.length > 0 && <SubtaskList quest={quest} interactive={myActive} boss={isBoss} />}

        {quest.skip_count >= 2 && myActive && !isBoss && (
          <AvoidanceKit quest={quest} onRetire={onArchive} busy={busy} />
        )}

        {/* min-w-0 lets the two verbs share the row on a narrow card (the
            flex default of min-width:auto would push one out of the card). */}
        {(onComplete || onStart) && isActive && (
          <div className="mt-4 flex items-center gap-2">
            {onStart && myActive && !running && (
              <Btn
                variant="ghost"
                className="min-w-0 flex-1"
                disabled={busy}
                onClick={() => onStart(quest.id)}
                data-testid={`start-${quest.id}`}
              >
                <Play size={15} />
                {t("quest.start")}
              </Btn>
            )}
            {onComplete && (
              <Btn
                variant="primary"
                className="min-w-0 flex-1"
                disabled={busy}
                onClick={() => onComplete(quest.id)}
                data-testid={`complete-${quest.id}`}
              >
                <Check size={16} />
                {t("common.complete")}
              </Btn>
            )}
          </div>
        )}

        {/* Skipped/archived quests aren't gone — bring them back to active. */}
        {onRestore && canRestore && (
          <div className="mt-4">
            <Btn
              variant="ghost"
              className="w-full"
              disabled={busy}
              onClick={() => onRestore(quest.id)}
              data-testid={`restore-${quest.id}`}
            >
              <RotateCcw size={15} />
              {t("quest.restore")}
            </Btn>
          </div>
        )}
      </div>
    </motion.div>
  );
}

// AvoidanceKit: a quest skipped or missed twice is a step that is too big,
// not a lazy person. Three exits, no counter shown: tiny first steps (AI),
// a smaller version (AI), or retiring it with dignity.
function AvoidanceKit({ quest, onRetire, busy }: { quest: Quest; onRetire?: (id: number) => void; busy?: boolean }) {
  const { t } = useI18n();
  const { data: openai } = useOpenAIStatus();
  const breakDown = useBreakDownQuest();
  const shrink = useShrinkQuest();
  const ai = !!openai?.connected;
  const pending = breakDown.isPending || shrink.isPending;
  return (
    <div className="mt-3 rounded-lg border border-dashed border-edge px-2.5 py-2" data-testid={`avoidance-${quest.id}`}>
      <div className="font-display text-[9px] uppercase tracking-[0.18em] text-faint">{t("quest.tooBig")}</div>
      <div className="mt-1.5 flex flex-wrap gap-1.5">
        <button
          disabled={!ai || pending || busy}
          title={ai ? t("quest.breakDownTitle") : t("quest.needsAi")}
          onClick={(e) => {
            e.stopPropagation();
            breakDown.mutate(quest.id, { onSuccess: () => pushToast(t("quest.brokenDown"), "success") });
          }}
          className="inline-flex items-center gap-1 rounded-md border border-edge px-2 py-1 text-[11px] text-muted transition-colors hover:text-ink disabled:opacity-40"
          data-testid={`breakdown-${quest.id}`}
        >
          <Sparkles size={11} /> {t("quest.breakDown")}
        </button>
        {!quest.shared_quest_id && (
          <button
            disabled={!ai || pending || busy}
            title={ai ? t("quest.shrinkTitle") : t("quest.needsAi")}
            onClick={(e) => {
              e.stopPropagation();
              shrink.mutate(quest.id, { onSuccess: (q) => pushToast(t("quest.shrunk", { title: q.title }), "success") });
            }}
            className="inline-flex items-center gap-1 rounded-md border border-edge px-2 py-1 text-[11px] text-muted transition-colors hover:text-ink disabled:opacity-40"
            data-testid={`shrink-${quest.id}`}
          >
            <Scissors size={11} /> {t("quest.shrink")}
          </button>
        )}
        {onRetire && (
          <button
            disabled={busy}
            title={t("quest.retireTitle")}
            onClick={(e) => {
              e.stopPropagation();
              onRetire(quest.id);
            }}
            className="inline-flex items-center gap-1 rounded-md border border-edge px-2 py-1 text-[11px] text-muted transition-colors hover:text-ink"
            data-testid={`retire-${quest.id}`}
          >
            <Archive size={11} /> {t("quest.retire")}
          </button>
        )}
      </div>
    </div>
  );
}

// BossHP renders a boss's phases as an HP bar: every checked phase is a hit
// that takes a chunk off. A week-long boss becomes five visible hits.
function BossHP({ quest }: { quest: Quest }) {
  const { t } = useI18n();
  const total = quest.subtasks.length;
  const done = quest.subtasks.filter((s) => s.done).length;
  const left = total - done;
  const prev = useRef(done);
  const [hit, setHit] = useState(false);
  useEffect(() => {
    if (done > prev.current) {
      setHit(true);
      const id = window.setTimeout(() => setHit(false), 550);
      return () => window.clearTimeout(id);
    }
    prev.current = done;
  }, [done]);
  useEffect(() => {
    prev.current = done;
  }, [done]);
  return (
    <div className={`mt-3 ${hit ? "boss-hit" : ""}`} data-testid={`boss-hp-${quest.id}`} data-hp={`${left}/${total}`}>
      <div className="mb-1 flex items-center justify-between font-display text-[10px] uppercase tracking-[0.18em]" style={{ color: "var(--color-boss)" }}>
        <span>{t("quest.hp")}</span>
        <span className="tabnum">
          {left}/{total}
        </span>
      </div>
      <div className="flex gap-1">
        {quest.subtasks.map((st) => (
          <span
            key={st.id}
            className="h-2 flex-1 rounded-[2px] transition-all"
            style={{
              background: st.done ? "rgba(255,255,255,0.06)" : "var(--color-boss)",
              boxShadow: st.done ? undefined : "0 0 8px rgba(var(--boss-rgb),0.6)",
            }}
          />
        ))}
      </div>
    </div>
  );
}

// SubtaskList renders a quest's bonus objectives (or a boss's phases). While
// the quest is active the checkboxes toggle via the API; afterwards they show
// frozen state.
function SubtaskList({ quest, interactive, boss = false }: { quest: Quest; interactive: boolean; boss?: boolean }) {
  const { t } = useI18n();
  const toggle = useToggleSubtask();
  return (
    <div className="mt-3 space-y-1 rounded-lg border border-edge/70 bg-white/[0.015] p-2">
      <div className="px-1 font-display text-[9px] uppercase tracking-[0.18em] text-faint">
        {boss ? t("quest.phases") : t("quest.bonusObjectives")}
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
