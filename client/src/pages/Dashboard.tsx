import { useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import { ArrowRight, Moon, Skull, Sparkles, SkipForward, Swords, Zap } from "lucide-react";
import {
  useDashboard,
  useCompleteQuest,
  useSkipQuest,
  useAcceptSuggestion,
  useOpenAIStatus,
  useSetRestMode,
  useSetHardcoreMode,
} from "../lib/queries";
import { useReward } from "../lib/reward";
import { getAttr, getType } from "../lib/theme";
import { CharacterHeader } from "../components/CharacterHeader";
import { TrophyCase } from "../components/TrophyCase";
import { AttributeCard } from "../components/AttributeCard";
import { QuestCard } from "../components/QuestCard";
import { XPFeed } from "../components/XPFeed";
import { SuggestionCard } from "../components/SuggestionCard";
import { Btn, DifficultyPips, EmptyState, Fold, SectionTitle, Spinner, RewardChips, TypeBadge } from "../components/ui";
import { pushToast } from "../lib/toast";
import { formatTime } from "../lib/format";
import { useI18n } from "../lib/i18n";
import type { CompletionResult, Quest } from "../lib/types";

export function DashboardPage({
  onGoToQuests,
  onGoToAgent,
}: {
  onGoToQuests: () => void;
  onGoToAgent: () => void;
}) {
  const { t } = useI18n();
  const { data, isLoading, isError, error } = useDashboard();
  const { data: openai } = useOpenAIStatus();
  const complete = useCompleteQuest();
  const skip = useSkipQuest();
  const accept = useAcceptSuggestion();
  const setRest = useSetRestMode();
  const setHardcore = useSetHardcoreMode();
  const { celebrate } = useReward();

  if (isLoading) return <Spinner label={t("dash.loading")} />;
  if (isError || !data) {
    return (
      <EmptyState
        title={t("common.backendUnreachable")}
        hint={(error as Error)?.message ?? t("common.backendHint")}
      />
    );
  }

  // Parity with the Quests page: crits, combos, drops and badges earned
  // from the home screen must land in the overlay too.
  const onCompleted = (res: CompletionResult) =>
    celebrate({
      title: res.completed_quest.title,
      xp_events: res.xp_events,
      level_ups: res.level_ups,
      label: t("reward.questComplete"),
      gold: res.gold,
      crit: res.crit,
      combo: res.combo_multiplier,
      drop: res.drop,
      achievements: res.achievements_unlocked,
      level: res.dashboard.character.level,
    });
  const handleComplete = (id: number) => complete.mutate(id, { onSuccess: onCompleted });

  return (
    <div className="space-y-6">
      <CharacterHeader
        character={data.character}
        streak={data.streak}
        daily={data.daily_progress}
        gold={data.gold_balance}
        activeDays={data.active_days}
      />

      {/* THE next move — one quest, one button, above everything else. */}
      <NextMove
        quests={data.today_quests}
        recommended={data.recommended_quest}
        busy={complete.isPending}
        onComplete={handleComplete}
        onGoToQuests={onGoToQuests}
      />

      <NearGoal attributes={data.attributes} />

      {/* Running loot buffs — a reason to complete MORE today */}
      {data.active_buffs.length > 0 && (
        <div className="flex flex-wrap gap-2" data-testid="active-buffs">
          {data.active_buffs.map((b) => (
            <span
              key={b.id}
              className="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium"
              style={{ borderColor: "#b98aff66", background: "#b98aff14", color: "#cbaaff" }}
              title={t("dash.buffUntil", { time: formatTime(b.expires_at) })}
            >
              {t("dash.buff", { percent: b.percent, attr: b.attribute === "" ? t("dash.buffAll") : getAttr(b.attribute).label })}
              {typeof b.uses_left === "number" && (
                <span className="tabnum opacity-80">· {t("dash.buffUses", { n: b.uses_left })}</span>
              )}
            </span>
          ))}
        </div>
      )}

      {data.rest_mode && (
        <div
          className="flex items-center justify-between rounded-lg border px-4 py-3"
          style={{ borderColor: "var(--color-gold)", background: "rgba(255,176,0,0.06)" }}
          data-testid="rest-banner"
        >
          <div className="flex items-center gap-2 text-sm" style={{ color: "var(--color-goldhi)" }}>
            <Moon size={16} />
            {t("dash.restOn")}
          </div>
          <button
            onClick={() => setRest.mutate(false)}
            className="rounded-md border border-edge px-3 py-1.5 text-xs font-medium text-muted transition-colors hover:text-ink"
          >
            {t("dash.wakeUp")}
          </button>
        </div>
      )}

      {/* Today's quests */}
      <section>
        <SectionTitle
          hint={t("dash.questsHint")}
          action={
            <Btn variant="ghost" onClick={onGoToQuests}>
              {t("dash.manage")} <ArrowRight size={14} />
            </Btn>
          }
        >
          {t("dash.todaysQuests")}
        </SectionTitle>
        {data.today_quests.length === 0 ? (
          <EmptyState icon={<Swords size={20} />} title={t("dash.noActiveQuests")} hint={t("dash.noActiveQuestsHint")} />
        ) : (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {data.today_quests.map((q, i) => (
              <QuestCard
                key={q.id}
                quest={q}
                index={i}
                busy={complete.isPending || skip.isPending}
                onComplete={handleComplete}
                onSkip={(id) => skip.mutate(id)}
              />
            ))}
          </div>
        )}
      </section>

      {/* Everything below is the stat sheet: folded by default. */}
      <Fold
        id="attributes"
        title={t("dash.attributes")}
        hint={t("dash.attributesHint")}
        action={
          <div className="flex items-center gap-4">
            <button
              onClick={() => setHardcore.mutate(!data.hardcore)}
              disabled={setHardcore.isPending}
              className="flex items-center gap-1.5 text-[11px] uppercase tracking-wider transition-colors hover:text-muted"
              style={{ color: data.hardcore ? "var(--color-boss)" : "var(--color-faint)" }}
              title={t("dash.hardcoreTitle")}
              data-testid="hardcore-toggle"
              data-on={data.hardcore ? "1" : "0"}
            >
              <Skull size={12} /> {data.hardcore ? t("dash.hardcoreOn") : t("dash.hardcoreOff")}
            </button>
            {!data.rest_mode && (
              <button
                onClick={() => setRest.mutate(true)}
                className="flex items-center gap-1.5 text-[11px] uppercase tracking-wider text-faint transition-colors hover:text-muted"
                title={t("dash.restTitle")}
                data-testid="rest-toggle"
              >
                <Moon size={12} /> {t("dash.restMode")}
              </button>
            )}
          </div>
        }
      >
        {data.hardcore && (data.decayed_today > 0 || data.daily_penalty_xp > 0) && (
          <div className="mb-3 rounded-lg border px-4 py-2.5 text-xs text-muted" style={{ borderColor: "#ff6a3d55" }} data-testid="hardcore-ledger">
            {data.decayed_today > 0 && <div>{t("dash.degradation", { xp: data.decayed_today })}</div>}
            {data.daily_penalty_xp > 0 && <div>{t("dash.dailyPenalty", { xp: data.daily_penalty_xp })}</div>}
          </div>
        )}
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {data.attributes.map((a, i) => (
            <AttributeCard key={a.key} attribute={a} index={i} goldBalance={data.gold_balance} />
          ))}
        </div>
      </Fold>

      <Fold id="trophies" title={t("dash.trophies")} hint={t("dash.trophiesHint")}>
        <TrophyCase />
      </Fold>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Fold id="recent-xp" title={t("dash.recentXp")} hint={t("dash.recentXpHint")}>
          <div className="hud-panel p-4">
            <XPFeed events={data.recent_xp_events} />
          </div>
        </Fold>

        <Fold id="suggestions" title={t("dash.aiSuggestions")} hint={t("dash.aiHint")} defaultOpen={data.pending_suggestions.length > 0}>
          {openai && !openai.connected ? (
            <button
              onClick={onGoToAgent}
              className="flex w-full items-center gap-2 rounded-xl border border-dashed border-edge px-4 py-3 text-left text-xs text-muted transition-colors hover:border-edge2 hover:text-ink"
            >
              <Sparkles size={15} style={{ color: "#b98aff" }} />
              {t("dash.connectHint")}
            </button>
          ) : data.pending_suggestions.length === 0 ? (
            <EmptyState title={t("dash.noSuggestions")} hint={t("dash.noSuggestionsHint")} />
          ) : (
            <div className="space-y-3">
              {data.pending_suggestions.slice(0, 2).map((s, i) => (
                <SuggestionCard
                  key={s.id}
                  suggestion={s}
                  index={i}
                  busy={accept.isPending}
                  onAccept={(id) =>
                    accept.mutate(id, {
                      onSuccess: (q) => pushToast(t("dash.addedQuest", { title: q.title }), "success"),
                    })
                  }
                />
              ))}
            </div>
          )}
        </Fold>
      </div>
    </div>
  );
}

// NextMove is the one-action screen: the recommended quest, huge, with one
// button. "Not this one" is a veto that rotates through the rest of today's
// board (client-side, nothing is skipped or logged) — choosing costs nothing.
function NextMove({
  quests,
  recommended,
  busy,
  onComplete,
  onGoToQuests,
}: {
  quests: Quest[];
  recommended: Quest | null;
  busy: boolean;
  onComplete: (id: number) => void;
  onGoToQuests: () => void;
}) {
  const { t } = useI18n();
  // Rotation order: the recommendation, then the rest of the board with
  // bosses last (they are deliberate, never a "pick something quick" answer).
  const ordered = useMemo(() => {
    const rest = quests.filter((q) => q.id !== recommended?.id);
    const normal = rest.filter((q) => q.type !== "boss");
    const bosses = rest.filter((q) => q.type === "boss");
    return [...(recommended ? [recommended] : []), ...normal, ...bosses];
  }, [quests, recommended]);
  const [vetoed, setVetoed] = useState<number[]>([]);
  // A new board (completion, rollover) resets the vetoes.
  useEffect(() => setVetoed([]), [recommended?.id, quests.length]);

  const candidates = ordered.filter((q) => !vetoed.includes(q.id));
  const current = candidates[0] ?? ordered[0];
  if (!current) {
    return (
      <div className="hud-panel clip-corner p-5 text-center" data-testid="next-move-empty">
        <div className="font-display text-[11px] uppercase tracking-[0.2em] text-muted">{t("dash.nextMove")}</div>
        <p className="mt-2 text-sm text-muted">{t("dash.noActiveQuestsHint")}</p>
        <Btn variant="primary" className="mt-3" onClick={onGoToQuests}>
          {t("dash.manage")} <ArrowRight size={14} />
        </Btn>
      </div>
    );
  }
  const meta = getType(current.type);
  const others = ordered.length - 1;
  return (
    <motion.div
      key={current.id}
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      className="hud-panel clip-corner relative overflow-hidden p-5 sm:p-6"
      style={{ background: "linear-gradient(120deg, rgba(255,176,0,0.12), rgba(53,224,255,0.06)), var(--color-panel)" }}
      data-testid="next-move"
      data-quest-id={current.id}
    >
      <div className="absolute inset-y-0 left-0 w-1" style={{ background: meta.color }} />
      <div className="mb-2 flex items-center gap-2">
        <Sparkles size={14} style={{ color: "var(--color-gold)" }} />
        <span className="font-display text-[11px] font-semibold uppercase tracking-[0.2em] text-[var(--color-gold)]">{t("dash.nextMove")}</span>
        <TypeBadge type={current.type} />
      </div>
      <h2 className="font-display text-2xl leading-tight text-ink sm:text-3xl" data-testid="next-move-title">
        {current.title}
      </h2>
      {current.description && <p className="mt-1 line-clamp-2 text-sm text-muted">{current.description}</p>}
      <div className="mt-3 flex flex-wrap items-center gap-3">
        <DifficultyPips difficulty={current.difficulty} />
        <RewardChips rewards={current.attribute_rewards} />
      </div>
      <div className="mt-5 flex flex-col gap-2 sm:flex-row sm:items-center">
        <Btn
          variant="primary"
          className="!py-3 !text-base sm:min-w-[220px]"
          disabled={busy}
          onClick={() => onComplete(current.id)}
          data-testid="next-move-complete"
        >
          <Zap size={18} /> {t("common.complete")}
        </Btn>
        {others > 0 && (
          <Btn variant="soft" onClick={() => setVetoed((v) => (candidates.length > 1 ? [...v, current.id] : []))} data-testid="next-move-veto">
            <SkipForward size={15} /> {t("dash.notThisOne")}
          </Btn>
        )}
      </div>
    </motion.div>
  );
}

// NearGoal names the attribute closest to leveling — something is always
// almost done, and the brain wants to close it.
function NearGoal({ attributes }: { attributes: import("../lib/types").Attribute[] }) {
  const { t } = useI18n();
  const candidates = attributes
    .map((a) => ({ a, left: a.xp_for_next_level - a.xp_into_level }))
    .filter((c) => c.left > 0)
    .sort((x, y) => x.left - y.left);
  const best = candidates[0];
  if (!best || best.left > 100) return null; // only when it's genuinely close
  const meta = getAttr(best.a.key);
  const Icon = meta.Icon;
  return (
    <div
      className="flex items-center gap-2 rounded-xl border px-3.5 py-2 text-sm"
      style={{ borderColor: `${meta.color}55`, background: `${meta.color}0f` }}
      data-testid="near-goal"
    >
      <Icon size={15} style={{ color: meta.color }} />
      <span className="text-muted">
        <span className="font-semibold" style={{ color: meta.color }}>{meta.label}</span>
        {t("dash.nearGoal1")}
        <span className="tabnum font-semibold text-ink">{t("dash.nearGoalXp", { xp: best.left })}</span>
        {t("dash.nearGoal2", { level: best.a.level + 1 })}
      </span>
    </div>
  );
}
