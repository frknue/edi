import { useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import { ArrowRight, Check, CornerDownRight, Moon, Play, Skull, Sparkles, SkipForward, Square, Star, Swords, X, Zap } from "lucide-react";
import {
  useDashboard,
  useCompleteQuest,
  useSkipQuest,
  useAcceptSuggestion,
  useOpenAIStatus,
  useSetRestMode,
  useSetHardcoreMode,
  useStartQuest,
  useStopQuest,
  useSetFirstMove,
  useClearFirstMove,
  useQuests,
  useArchiveQuest,
  useStoryChapters,
} from "../lib/queries";
import { useReward } from "../lib/reward";
import { getAttr, getType } from "../lib/theme";
import { CharacterHeader } from "../components/CharacterHeader";
import { Hero } from "../components/Hero";
import { TrophyCase } from "../components/TrophyCase";
import { AttributeCard } from "../components/AttributeCard";
import { QuestCard } from "../components/QuestCard";
import { XPFeed } from "../components/XPFeed";
import { SuggestionCard } from "../components/SuggestionCard";
import { Btn, DifficultyPips, EmptyState, Fold, SectionTitle, Spinner, RewardChips, TypeBadge } from "../components/ui";
import { pushToast } from "../lib/toast";
import { formatTime } from "../lib/format";
import { useI18n } from "../lib/i18n";
import type { CompletionResult, Dashboard, EquippedCosmetic, Quest, QuestSession } from "../lib/types";
import type { MessageKey } from "../lib/locales/en";

export function DashboardPage({
  onGoToQuests,
  onGoToAgent,
  onGoToWardrobe,
}: {
  onGoToQuests: () => void;
  onGoToAgent: () => void;
  onGoToWardrobe: () => void;
}) {
  const { t } = useI18n();
  const { data, isLoading, isError, error } = useDashboard();
  const { data: openai } = useOpenAIStatus();
  const complete = useCompleteQuest();
  const skip = useSkipQuest();
  const accept = useAcceptSuggestion();
  const setRest = useSetRestMode();
  const setHardcore = useSetHardcoreMode();
  const start = useStartQuest();
  const stop = useStopQuest();
  const archive = useArchiveQuest();
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
      label: res.board_clear ? t("reward.boardClear") : t("reward.questComplete"),
      gold: res.gold,
      crit: res.crit,
      combo: res.combo_multiplier,
      drop: res.drop,
      achievements: res.achievements_unlocked,
      level: res.dashboard.character.level,
      loadout: res.dashboard.loadout,
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
        loot={data.loot_pity}
        loadout={data.loadout}
        gearGoal={data.gear_goal}
        mood={data.active_session ? "focus" : data.day_state === "camp" ? "camp" : "idle"}
        onOpenWardrobe={onGoToWardrobe}
      />

      {/* THE next move — one quest, one button, above everything else.
          While a quest runs, the panel IS the running quest: timer, finisher.
          Once today's set is cleared, it is the campfire: you're done. */}
      {data.active_session ? (
        <RunningQuest
          session={data.active_session}
          level={data.character.level}
          loadout={data.loadout}
          busy={complete.isPending || stop.isPending}
          onComplete={handleComplete}
          onStop={(note) => stop.mutate(note)}
        />
      ) : data.day_state === "camp" ? (
        <Campfire data={data} />
      ) : (
        <NextMove
          quests={data.today_quests}
          recommended={data.recommended_quest}
          busy={complete.isPending || start.isPending}
          onComplete={handleComplete}
          onStart={(id) => start.mutate(id)}
          onGoToQuests={onGoToQuests}
        />
      )}

      {data.partner_session && (
        <div
          className="flex items-center gap-2 rounded-xl border px-3.5 py-2 text-sm"
          style={{ borderColor: "rgba(255,140,200,0.4)", background: "rgba(255,140,200,0.06)" }}
          data-testid="partner-session"
        >
          <span aria-hidden>🤝</span>
          <span className="text-muted">
            {t("dash.alongside", { name: data.partner_session.name, title: data.partner_session.title, min: Math.floor(data.partner_session.elapsed_seconds / 60) })}
          </span>
        </div>
      )}

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
          style={{ borderColor: "var(--color-gold)", background: "rgba(var(--gold-rgb),0.06)" }}
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
          {data.day_state === "camp" ? t("dash.extraCredit") : t("dash.todaysQuests")}
        </SectionTitle>
        {data.today_quests.length === 0 ? (
          <EmptyState icon={<Swords size={20} />} title={t("dash.noActiveQuests")} hint={t("dash.noActiveQuestsHint")} />
        ) : (
          <div className={`grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 ${data.day_state === "camp" ? "opacity-70" : ""}`}>
            {data.today_quests.map((q, i) => (
              <QuestCard
                key={q.id}
                quest={q}
                index={i}
                busy={complete.isPending || skip.isPending || start.isPending}
                running={data.active_session?.quest_id === q.id}
                onStart={data.active_session ? undefined : (id) => start.mutate(id)}
                onComplete={handleComplete}
                onSkip={(id) => skip.mutate(id)}
                onArchive={q.skip_count >= 2 ? (id) => archive.mutate(id, { onSuccess: () => pushToast(t("quest.retired"), "success") }) : undefined}
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

      <Fold id="saga" title={t("dash.saga")} hint={t("dash.sagaHint")} defaultOpen={!!data.latest_chapter}>
        <Saga latest={data.latest_chapter} />
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
  onStart,
  onGoToQuests,
}: {
  quests: Quest[];
  recommended: Quest | null;
  busy: boolean;
  onComplete: (id: number) => void;
  onStart: (id: number) => void;
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
      style={{ background: "linear-gradient(120deg, rgba(var(--gold-rgb),0.12), rgba(53,224,255,0.06)), var(--color-panel)" }}
      data-testid="next-move"
      data-quest-id={current.id}
    >
      <div className="absolute inset-y-0 left-0 w-1" style={{ background: meta.color }} />
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <Sparkles size={14} style={{ color: "var(--color-gold)" }} />
        <span className="font-display text-[11px] font-semibold uppercase tracking-[0.2em] text-[var(--color-gold)]">{t("dash.nextMove")}</span>
        <TypeBadge type={current.type} />
        {current.recommend_reason && current.id === recommended?.id && (
          <span className="text-[11px] text-muted" data-testid="next-move-reason">
            · {t(`dash.reason.${current.recommend_reason}` as MessageKey)}
          </span>
        )}
      </div>
      <h2 className="font-display text-2xl leading-tight text-ink sm:text-3xl" data-testid="next-move-title">
        {current.title}
      </h2>
      {current.description && <p className="mt-1 line-clamp-2 text-sm text-muted">{current.description}</p>}
      {current.resume_note && (
        <p className="mt-2 flex items-start gap-1.5 text-sm text-muted" data-testid="next-move-resume">
          <CornerDownRight size={14} className="mt-0.5 shrink-0" style={{ color: "var(--color-phos)" }} />
          <span>
            <span className="font-display text-[10px] uppercase tracking-[0.18em] text-faint">{t("quest.resume")} </span>
            {current.resume_note}
          </span>
        </p>
      )}
      <div className="mt-3 flex flex-wrap items-center gap-3">
        <DifficultyPips difficulty={current.difficulty} />
        <RewardChips rewards={current.attribute_rewards} />
        {typeof current.projected_xp === "number" && current.projected_xp > 0 && (
          <span className="tabnum font-display text-sm" style={{ color: "var(--color-goldhi)" }} title={t("dash.paysNowTitle")} data-testid="next-move-pays">
            {t("dash.paysNow", { xp: current.projected_xp })}
          </span>
        )}
      </div>
      <div className="mt-5 flex flex-col gap-2 sm:flex-row sm:items-center">
        <Btn
          variant="primary"
          className="!py-3 !text-base sm:min-w-[220px]"
          disabled={busy}
          onClick={() => onStart(current.id)}
          data-testid="next-move-start"
        >
          <Play size={18} /> {t("dash.start")}
        </Btn>
        <Btn variant="ghost" disabled={busy} onClick={() => onComplete(current.id)} data-testid="next-move-complete" title={t("dash.alreadyDoneTitle")}>
          <Check size={15} /> {t("dash.alreadyDone")}
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

// Saga: the narrator's chapters, newest first — the hero returns with a story.
function Saga({ latest }: { latest: Dashboard["latest_chapter"] }) {
  const { t } = useI18n();
  const { data: chapters } = useStoryChapters(5);
  const list = chapters ?? (latest ? [latest] : []);
  if (list.length === 0) return <EmptyState title={t("dash.noChapters")} hint={t("dash.noChaptersHint")} />;
  return (
    <div className="space-y-2" data-testid="saga">
      {list.map((ch) => (
        <div key={ch.id} className="hud-panel p-3.5">
          <div className="font-display text-[10px] uppercase tracking-[0.2em]" style={{ color: "var(--color-goldhi)" }}>
            {t("dash.chapter", { n: ch.number })}
          </div>
          <p className="mt-1 text-sm italic leading-relaxed text-ink">{ch.text}</p>
        </div>
      ))}
    </div>
  );
}

// Campfire is the home screen once today's set is cleared: you are done.
// The hero rests by the fire, the day is summarised once, the extras below
// are muted, the nudge stands down. One tap picks tomorrow's first move
// (the shutdown ritual) — dismissing it later costs nothing.
function Campfire({ data }: { data: Dashboard }) {
  const { t } = useI18n();
  const setFirst = useSetFirstMove();
  const clearFirst = useClearFirstMove();
  const { data: dailies } = useQuests({ type: "daily" });
  const tomorrow = data.first_move && !data.active_days.some((d) => d.today && d.day === data.first_move?.day) ? data.first_move : null;
  const today = data.first_move && !tomorrow ? data.first_move : null;
  const candidates = useMemo(() => {
    const seen = new Set<number>();
    const out: Quest[] = [];
    for (const q of [...(dailies ?? []).filter((q) => q.status === "active" || q.status === "completed"), ...data.today_quests]) {
      if (seen.has(q.id) || q.type === "boss" || (q.shared_quest_id !== undefined && !q.assigned_to_me)) continue;
      seen.add(q.id);
      out.push(q);
      if (out.length >= 4) break;
    }
    return out;
  }, [dailies, data.today_quests]);
  const active = data.active_days.filter((d) => d.active).length;
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      className="hud-panel clip-corner relative overflow-hidden p-5 sm:p-6"
      style={{ background: "linear-gradient(120deg, rgba(255,140,40,0.12), rgba(var(--gold-rgb),0.05)), var(--color-panel)", borderColor: "rgba(255,160,60,0.45)" }}
      data-testid="campfire"
    >
      <div className="flex flex-col gap-5 sm:flex-row sm:items-center">
        <div className="flex items-center gap-4">
          <Hero level={data.character.level} titled={!!data.character.title} mood="camp" size={96} loadout={data.loadout} className="-my-4" />
          <span className="campfire text-3xl" aria-hidden>
            🔥
          </span>
          <div className="min-w-0">
            <div className="font-display text-[11px] font-semibold uppercase tracking-[0.2em]" style={{ color: "#ffa23e" }}>
              {t("dash.camp")}
            </div>
            <h2 className="font-display text-2xl leading-tight text-ink sm:text-3xl">{t("dash.campTitle")}</h2>
            <p className="mt-1 text-sm text-muted">
              {t("dash.campSummary", { xp: data.xp_today, done: data.daily_progress.dailies_done, goal: data.daily_progress.goal, days: active })}
              {data.board_clear_today && <span style={{ color: "var(--color-goldhi)" }}> {t("dash.campBonus")}</span>}
            </p>
          </div>
        </div>
      </div>

      <div className="mt-5 border-t border-edge/60 pt-4" data-testid="tomorrow-first">
        <div className="font-display text-[11px] uppercase tracking-[0.18em] text-muted">{t("dash.tomorrowFirst")}</div>
        {tomorrow ? (
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <span className="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm" style={{ borderColor: "var(--color-gold)", color: "var(--color-goldhi)" }} data-testid="tomorrow-picked">
              <Star size={13} /> {tomorrow.quest.title}
            </span>
            <button onClick={() => clearFirst.mutate()} className="text-faint hover:text-ink" aria-label={t("dash.unpin")} title={t("dash.unpin")} data-testid="tomorrow-unpin">
              <X size={14} />
            </button>
          </div>
        ) : candidates.length === 0 ? (
          <p className="mt-2 text-xs text-faint">{t("dash.noActiveQuestsHint")}</p>
        ) : (
          <div className="mt-2 flex flex-wrap gap-2">
            {candidates.map((q) => (
              <button
                key={q.id}
                disabled={setFirst.isPending}
                onClick={() => setFirst.mutate({ questId: q.id, tomorrow: true })}
                className="rounded-full border border-edge px-3 py-1 text-sm text-muted transition-colors hover:border-[var(--color-gold)] hover:text-ink"
                data-testid={`pick-first-${q.id}`}
              >
                {q.title}
              </button>
            ))}
          </div>
        )}
        {today && <p className="mt-2 text-[11px] text-faint">{t("dash.firstMoveDone")}</p>}
      </div>
    </motion.div>
  );
}

// RunningQuest is the home screen while a quest is in progress: the timer,
// the reward waiting at the end, the hero at work. Complete is the finisher;
// Stop asks the landing question ("what is the next physical action?") and
// stores the answer on the quest as its resume note. The ticks are cosmetic
// — no XP moves until Complete.
function RunningQuest({
  session,
  level,
  loadout,
  busy,
  onComplete,
  onStop,
}: {
  session: QuestSession;
  level: number;
  loadout: EquippedCosmetic[];
  busy: boolean;
  onComplete: (id: number) => void;
  onStop: (note: string) => void;
}) {
  const { t } = useI18n();
  const elapsed = useElapsed(session.started_at);
  const [landing, setLanding] = useState(false);
  const [note, setNote] = useState("");
  const meta = getType(session.quest_type);
  const focusBlock = 25 * 60; // one cosmetic "block" — the bar refills every 25 min
  const block = (elapsed % focusBlock) / focusBlock;
  const blocks = Math.floor(elapsed / focusBlock);
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      className="hud-panel clip-corner relative overflow-hidden p-5 sm:p-6"
      style={{ background: "linear-gradient(120deg, rgba(var(--phos-rgb),0.10), rgba(53,224,255,0.05)), var(--color-panel)", borderColor: "rgba(var(--phos-rgb),0.45)" }}
      data-testid="running-quest"
      data-quest-id={session.quest_id}
    >
      <div className="absolute inset-y-0 left-0 w-1" style={{ background: meta.color }} />
      <div className="flex flex-col gap-5 sm:flex-row sm:items-center">
        <div className="flex items-center gap-4">
          <Hero level={level} mood="focus" size={96} loadout={loadout} className="-my-4" />
          <div className="min-w-0">
            <div className="mb-1 flex items-center gap-2">
              <Play size={13} style={{ color: "var(--color-phos)" }} />
              <span className="font-display text-[11px] font-semibold uppercase tracking-[0.2em]" style={{ color: "var(--color-phos)" }}>
                {t("dash.running")}
              </span>
              <TypeBadge type={session.quest_type} />
            </div>
            <h2 className="font-display text-2xl leading-tight text-ink sm:text-3xl" data-testid="running-title">
              {session.title}
            </h2>
            {session.resume_note && !landing && (
              <p className="mt-1 flex items-start gap-1.5 text-sm text-muted">
                <CornerDownRight size={14} className="mt-0.5 shrink-0" style={{ color: "var(--color-phos)" }} />
                {session.resume_note}
              </p>
            )}
          </div>
        </div>
        <div className="sm:ml-auto sm:text-right">
          <div className="timer-pulse tabnum font-display text-5xl leading-none" style={{ color: "var(--color-phos)" }} data-testid="running-timer">
            {formatElapsed(elapsed)}
          </div>
          <div className="mt-1 font-display text-[10px] uppercase tracking-wider text-faint">
            {blocks > 0 ? t("dash.blocks", { n: blocks }) : t("dash.elapsed")}
          </div>
        </div>
      </div>

      {/* cosmetic momentum bar — refills every 25 minutes; pays nothing */}
      <div className="mt-4 h-1.5 overflow-hidden rounded-full bg-white/[0.06]">
        <div
          className="h-full rounded-full"
          style={{ width: `${Math.max(2, block * 100)}%`, background: "linear-gradient(90deg, rgba(var(--phos-rgb),0.5), var(--color-phos))", transition: "width 1s linear" }}
        />
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-muted">
        <span className="font-display text-[10px] uppercase tracking-[0.18em] text-faint">{t("dash.rewardWaiting")}</span>
        <RewardChips rewards={session.attribute_rewards} />
      </div>

      {landing ? (
        <form
          className="mt-5 space-y-2"
          data-testid="landing-form"
          onSubmit={(e) => {
            e.preventDefault();
            onStop(note.trim());
          }}
        >
          <label className="block font-display text-[11px] uppercase tracking-[0.18em] text-muted" htmlFor="landing-note">
            {t("dash.landingQuestion")}
          </label>
          <input
            id="landing-note"
            autoFocus
            maxLength={280}
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder={t("dash.landingPlaceholder")}
            className="w-full rounded-sm border border-edge bg-black/30 px-3 py-2 text-sm text-ink outline-none focus:border-[var(--color-phos)]"
            data-testid="landing-input"
          />
          <div className="flex flex-wrap gap-2">
            <Btn variant="primary" type="submit" disabled={busy} data-testid="landing-save">
              <Square size={14} /> {note.trim() ? t("dash.stopAndSave") : t("dash.justStop")}
            </Btn>
            <Btn variant="soft" type="button" onClick={() => setLanding(false)}>
              {t("common.cancel")}
            </Btn>
          </div>
        </form>
      ) : (
        <div className="mt-5 flex flex-col gap-2 sm:flex-row sm:items-center">
          <Btn
            variant="primary"
            className="!py-3 !text-base sm:min-w-[220px]"
            disabled={busy}
            onClick={() => onComplete(session.quest_id)}
            data-testid="running-complete"
          >
            <Zap size={18} /> {t("common.complete")}
          </Btn>
          <Btn variant="soft" disabled={busy} onClick={() => setLanding(true)} data-testid="running-stop">
            <Square size={14} /> {t("dash.stop")}
          </Btn>
        </div>
      )}
    </motion.div>
  );
}

// useElapsed ticks once a second from a server timestamp — the client owns
// the display, the server owns the truth (started_at).
function useElapsed(startedAt: string): number {
  const calc = () => Math.max(0, Math.floor((Date.now() - new Date(startedAt).getTime()) / 1000));
  const [elapsed, setElapsed] = useState(calc);
  useEffect(() => {
    setElapsed(calc());
    const id = window.setInterval(() => setElapsed(calc()), 1000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [startedAt]);
  return elapsed;
}

function formatElapsed(sec: number): string {
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const mm = String(m).padStart(2, "0");
  const ss = String(s).padStart(2, "0");
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
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
