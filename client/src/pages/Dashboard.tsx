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
import { CharacterHeader } from "../components/CharacterHeader";
import { AttributeCard } from "../components/AttributeCard";
import { QuestCard } from "../components/QuestCard";
import { XPFeed } from "../components/XPFeed";
import { SuggestionCard } from "../components/SuggestionCard";
import { Btn, EmptyState, SectionTitle, Spinner, RewardChips } from "../components/ui";
import { pushToast } from "../lib/toast";

export function DashboardPage({
  onGoToQuests,
  onGoToAgent,
}: {
  onGoToQuests: () => void;
  onGoToAgent: () => void;
}) {
  const { data, isLoading, isError, error } = useDashboard();
  const { data: openai } = useOpenAIStatus();
  const complete = useCompleteQuest();
  const skip = useSkipQuest();
  const accept = useAcceptSuggestion();
  const setRest = useSetRestMode();
  const { celebrate } = useReward();

  if (isLoading) return <Spinner label="Loading your character…" />;
  if (isError || !data) {
    return (
      <EmptyState
        title="Couldn't reach the backend"
        hint={(error as Error)?.message ?? "Is the Go server running on :8080?"}
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
          label: "Quest Complete",
          gold: res.gold,
        }),
    });

  const rec = data.recommended_quest;

  return (
    <div className="space-y-7">
      <CharacterHeader character={data.character} streak={data.streak} daily={data.daily_progress} gold={data.gold_balance} />

      {data.rest_mode && (
        <div
          className="flex items-center justify-between rounded-lg border px-4 py-3"
          style={{ borderColor: "var(--color-gold)", background: "rgba(255,176,0,0.06)" }}
          data-testid="rest-banner"
        >
          <div className="flex items-center gap-2 text-sm" style={{ color: "var(--color-goldhi)" }}>
            <Moon size={16} />
            Rest mode is ON — all decay is paused. Recover well.
          </div>
          <button
            onClick={() => setRest.mutate(false)}
            className="rounded-md border border-edge px-3 py-1.5 text-xs font-medium text-muted transition-colors hover:text-ink"
          >
            Wake up
          </button>
        </div>
      )}

      {data.decayed_today > 0 && (
        <div
          className="rounded-lg border px-4 py-3 text-sm"
          style={{ borderColor: "#ff6a3d88", background: "rgba(255,106,61,0.07)", color: "#ff8a65" }}
          data-testid="decay-alert"
        >
          SYSTEM DEGRADATION — {data.decayed_today} XP lost to decay since your last visit. Train the rusting
          attributes or ward them.
        </div>
      )}

      {/* Attributes */}
      <section>
        <SectionTitle
          hint="Every action trains a real-life stat."
          action={
            !data.rest_mode && (
              <button
                onClick={() => setRest.mutate(true)}
                className="flex items-center gap-1.5 text-[11px] uppercase tracking-wider text-faint transition-colors hover:text-muted"
                title="Pause all decay (vacation/sick)"
                data-testid="rest-toggle"
              >
                <Moon size={12} /> rest mode
              </button>
            )
          }
        >
          Attributes
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
                  Recommended next
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
                  <Zap size={16} /> Complete
                </Btn>
              </div>
            </motion.div>
          )}

          <section>
            <SectionTitle
              hint="Complete actions to earn XP."
              action={
                <Btn variant="ghost" onClick={onGoToQuests}>
                  Manage <ArrowRight size={14} />
                </Btn>
              }
            >
              Today's Quests
            </SectionTitle>
            {data.today_quests.length === 0 ? (
              <EmptyState
                icon={<Swords size={20} />}
                title="No active quests"
                hint="Create one or accept a suggestion to get going."
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
            <SectionTitle hint="Your latest gains.">Recent XP</SectionTitle>
            <XPFeed events={data.recent_xp_events} />
          </section>

          <section>
            <SectionTitle hint="From your ChatGPT model.">AI Suggestions</SectionTitle>
            {openai && !openai.connected ? (
              <button
                onClick={onGoToAgent}
                className="flex w-full items-center gap-2 rounded-xl border border-dashed border-edge px-4 py-3 text-left text-xs text-muted transition-colors hover:border-edge2 hover:text-ink"
              >
                <Sparkles size={15} style={{ color: "#b98aff" }} />
                Connect your ChatGPT account on the Agent tab to unlock AI suggestions.
              </button>
            ) : data.pending_suggestions.length === 0 ? (
              <EmptyState title="No suggestions" hint="Generate some on the Agent tab." />
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
                        onSuccess: (q) => pushToast(`Added quest: ${q.title}`, "success"),
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
