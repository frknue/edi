import { motion } from "framer-motion";
import { ArrowRight, Moon, Sparkles, Swords, Zap } from "lucide-react";
import {
  useDashboard,
  useCompleteQuest,
  useSkipQuest,
  useAcceptSuggestion,
  useOpenAIStatus,
  useSetRestMode,
} from "../lib/queries";
import { useReward } from "../lib/reward";
import { getAttr } from "../lib/theme";
import { CharacterHeader } from "../components/CharacterHeader";
import { TrophyCase } from "../components/TrophyCase";
import { AttributeCard } from "../components/AttributeCard";
import { QuestCard } from "../components/QuestCard";
import { XPFeed } from "../components/XPFeed";
import { SuggestionCard } from "../components/SuggestionCard";
import { Btn, EmptyState, SectionTitle, Spinner, RewardChips } from "../components/ui";
import { pushToast } from "../lib/toast";
import { formatTime } from "../lib/format";
import { useI18n } from "../lib/i18n";

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

  const handleComplete = (id: number) =>
    complete.mutate(id, {
      onSuccess: (res) =>
        celebrate({
          title: res.completed_quest.title,
          xp_events: res.xp_events,
          level_ups: res.level_ups,
          label: t("reward.questComplete"),
          gold: res.gold,
        }),
    });

  const rec = data.recommended_quest;

  return (
    <div className="space-y-7">
      <CharacterHeader character={data.character} streak={data.streak} daily={data.daily_progress} gold={data.gold_balance} />

      <TrophyCase />

      <NearGoal attributes={data.attributes} />

      {/* Running loot buffs — a reason to complete MORE today */}
      {data.active_buffs.length > 0 && (
        <div className="flex flex-wrap gap-2" data-testid="active-buffs">
          {data.active_buffs.map((b, i) => (
            <span
              key={i}
              className="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium"
              style={{ borderColor: "#b98aff66", background: "#b98aff14", color: "#cbaaff" }}
              title={t("dash.buffUntil", { time: formatTime(b.expires_at) })}
            >
              {t("dash.buff", { percent: b.percent, attr: b.attribute === "" ? t("dash.buffAll") : getAttr(b.attribute).label })}
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

      {data.decayed_today > 0 && (
        <div
          className="rounded-lg border px-4 py-3 text-sm"
          style={{ borderColor: "#ff6a3d88", background: "rgba(255,106,61,0.07)", color: "#ff8a65" }}
          data-testid="decay-alert"
        >
          {t("dash.degradation", { xp: data.decayed_today })}
        </div>
      )}

      {/* Attributes */}
      <section>
        <SectionTitle
          hint={t("dash.attributesHint")}
          action={
            !data.rest_mode && (
              <button
                onClick={() => setRest.mutate(true)}
                className="flex items-center gap-1.5 text-[11px] uppercase tracking-wider text-faint transition-colors hover:text-muted"
                title={t("dash.restTitle")}
                data-testid="rest-toggle"
              >
                <Moon size={12} /> {t("dash.restMode")}
              </button>
            )
          }
        >
          {t("dash.attributes")}
        </SectionTitle>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {data.attributes.map((a, i) => (
            <AttributeCard key={a.key} attribute={a} index={i} goldBalance={data.gold_balance} />
          ))}
        </div>
      </section>

      <div className="grid grid-cols-1 gap-7 lg:grid-cols-3">
        {/* Quests column */}
        <div className="space-y-6 lg:col-span-2">
          {/* Recommended */}
          {rec && (
            <motion.div
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              className="hud-panel clip-corner relative overflow-hidden p-4"
              style={{ background: "linear-gradient(120deg, rgba(255,176,0,0.10), rgba(53,224,255,0.06)), var(--color-panel)" }}
            >
              <div className="mb-2 flex items-center gap-2">
                <Sparkles size={14} style={{ color: "var(--color-gold)" }} />
                <span className="font-display text-[11px] font-semibold uppercase tracking-[0.2em] text-[var(--color-gold)]">
                  {t("dash.recommended")}
                </span>
              </div>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <h3 className="truncate text-lg font-semibold text-ink">{rec.title}</h3>
                  {rec.description && <p className="mt-0.5 line-clamp-1 text-xs text-muted">{rec.description}</p>}
                  <div className="mt-2">
                    <RewardChips rewards={rec.attribute_rewards} />
                  </div>
                </div>
                <Btn
                  variant="primary"
                  className="shrink-0"
                  disabled={complete.isPending}
                  onClick={() => handleComplete(rec.id)}
                >
                  <Zap size={16} /> {t("common.complete")}
                </Btn>
              </div>
            </motion.div>
          )}

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
              <EmptyState
                icon={<Swords size={20} />}
                title={t("dash.noActiveQuests")}
                hint={t("dash.noActiveQuestsHint")}
              />
            ) : (
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
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
        </div>

        {/* Side column */}
        <div className="space-y-6">
          <section className="hud-panel p-4">
            <SectionTitle hint={t("dash.recentXpHint")}>{t("dash.recentXp")}</SectionTitle>
            <XPFeed events={data.recent_xp_events} />
          </section>

          <section>
            <SectionTitle hint={t("dash.aiHint")}>{t("dash.aiSuggestions")}</SectionTitle>
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
          </section>
        </div>
      </div>
    </div>
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
