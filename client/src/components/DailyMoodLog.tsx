import { useMemo, useState } from "react";
import { motion } from "framer-motion";
import {
  ArrowLeft,
  ArrowRight,
  BrainCircuit,
  Check,
  ChevronDown,
  ChevronUp,
  HeartHandshake,
  Loader2,
  Plus,
  Sparkles,
  Trash2,
  X,
} from "lucide-react";
import { useCompleteTool, useMoodAssist, useOpenAIStatus, useToolEntries } from "../lib/queries";
import { useReward } from "../lib/reward";
import { useAiConsent } from "../lib/aiConsent";
import { pushToast } from "../lib/toast";
import { relativeTime } from "../lib/format";
import { EMOTIONS, DISTORTIONS, emotionLabel, distortionName } from "../lib/cbt";
import type { MoodDistortionHit, MoodLog, MoodResponseIdea, MoodThought, ToolEntry } from "../lib/types";
import { Btn, EmptyState, SectionTitle, Spinner } from "./ui";
import { useI18n } from "../lib/i18n";
import type { MessageKey } from "../lib/locales/en";

interface EmotionState {
  before: number;
  after: number;
}
type EmotionMap = Record<string, EmotionState>;

const emptyThought = (): MoodThought => ({
  thought: "",
  belief_before: 70,
  distortions: [],
  positive_thought: "",
  positive_belief: 50,
  belief_after: 30,
});

const STEP_KEYS: MessageKey[] = ["mood.step1", "mood.step2", "mood.step3"];

// DailyMoodLog lands on the HISTORY of past logs (reviewing them is part of the
// method — you see your shifts and recurring distortions). [ NEW LOG ] enters
// the guided 3-step flow; finishing or backing out returns here.
export function DailyMoodLog({ onClose }: { onClose: () => void }) {
  const [writing, setWriting] = useState(false);
  if (writing) {
    return <MoodLogFlow onClose={() => setWriting(false)} />;
  }
  return <MoodLogHistory onClose={onClose} onNew={() => setWriting(true)} />;
}

// --- history landing ----------------------------------------------------------

function avgShift(data: MoodLog): { before: number; after: number } {
  const n = data.emotions.length || 1;
  const before = Math.round(data.emotions.reduce((s, e) => s + e.before, 0) / n);
  const after = Math.round(data.emotions.reduce((s, e) => s + e.after, 0) / n);
  return { before, after };
}

function MoodLogHistory({ onClose, onNew }: { onClose: () => void; onNew: () => void }) {
  const { t, tp } = useI18n();
  const { data: entries, isLoading } = useToolEntries("daily_mood_log");

  // Aggregate insight across all logs: average distress drop + top distortions.
  const stats = useMemo(() => {
    if (!entries || entries.length === 0) return null;
    let dropSum = 0;
    const distortionCount: Record<string, number> = {};
    for (const e of entries) {
      const { before, after } = avgShift(e.data);
      dropSum += before - after;
      for (const t of e.data.thoughts ?? []) {
        for (const d of t.distortions ?? []) distortionCount[d] = (distortionCount[d] ?? 0) + 1;
      }
    }
    const top = Object.entries(distortionCount).sort((a, b) => b[1] - a[1]).slice(0, 2);
    return { count: entries.length, avgDrop: Math.round(dropSum / entries.length), top };
  }, [entries]);

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-5 flex items-center justify-between">
        <button onClick={onClose} className="flex items-center gap-1.5 text-sm text-muted hover:text-ink">
          <ArrowLeft size={16} /> {t("common.back")}
        </button>
        <Btn variant="primary" onClick={onNew} data-testid="new-mood-log">
          <Plus size={15} /> {t("mood.newLog")}
        </Btn>
      </div>

      <div className="mb-1 font-display text-[11px] uppercase tracking-[0.24em] text-[var(--color-spirituality)]">
        {t("mood.overline")}
      </div>
      <h1 className="mb-4 font-display text-3xl leading-tight text-ink">{t("mood.yourLogs")}</h1>

      {stats && (
        <div className="hud-panel clip-corner mb-5 flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
          <Stat label={tp("mood.logs", stats.count)} value={String(stats.count)} />
          <Stat
            label={t("mood.avgDrop")}
            value={t("mood.pts", { n: `${stats.avgDrop > 0 ? "−" : ""}${Math.abs(stats.avgDrop)}` })}
            color={stats.avgDrop > 0 ? "var(--color-health)" : "var(--color-muted)"}
          />
          {stats.top.length > 0 && (
            <div className="flex items-center gap-1.5">
              <span className="text-[10px] uppercase tracking-wider text-faint">{t("mood.recurring")}</span>
              {stats.top.map(([code, n]) => (
                <span
                  key={code}
                  className="rounded-sm px-1.5 py-0.5 text-[11px]"
                  style={{ background: "rgba(185,138,255,0.14)", color: "#b98aff" }}
                  title={tp("mood.appearedIn", n, { name: distortionName(code) })}
                >
                  {distortionName(code)} ×{n}
                </span>
              ))}
            </div>
          )}
        </div>
      )}

      {isLoading ? (
        <Spinner />
      ) : !entries || entries.length === 0 ? (
        <EmptyState
          icon={<BrainCircuit size={22} />}
          title={t("mood.noLogs")}
          hint={t("mood.noLogsHint")}
        />
      ) : (
        <div className="space-y-2.5">
          <SectionTitle hint={t("mood.historyHint")}>{t("mood.history")}</SectionTitle>
          {entries.map((e, i) => (
            <HistoryEntry key={e.id} entry={e} index={i} />
          ))}
        </div>
      )}
    </div>
  );
}

function Stat({ label, value, color }: { label: string; value: string; color?: string }) {
  return (
    <div className="flex items-baseline gap-1.5">
      <span className="font-display text-2xl" style={{ color: color ?? "var(--color-ink)" }}>
        {value}
      </span>
      <span className="text-[10px] uppercase tracking-wider text-faint">{label}</span>
    </div>
  );
}

function HistoryEntry({ entry, index }: { entry: ToolEntry; index: number }) {
  const { t, tp } = useI18n();
  const [open, setOpen] = useState(false);
  const data = entry.data;
  const { before, after } = avgShift(data);
  const drop = before - after;

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay: Math.min(index * 0.04, 0.3) }}
      className="hud-panel overflow-hidden"
    >
      <button
        onClick={() => setOpen((o) => !o)}
        data-testid={`mood-entry-${entry.id}`}
        className="flex w-full items-center gap-3 p-3.5 text-left"
      >
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm text-ink">{data.event}</div>
          <div className="mt-0.5 text-[11px] text-faint">
            {relativeTime(entry.created_at)} · {tp("mood.thoughts", data.thoughts?.length ?? 0)} · +{entry.xp_awarded} XP
          </div>
        </div>
        <span className="tabnum shrink-0 text-sm">
          <span className="text-muted">{before}%</span>
          <span className="text-faint"> → </span>
          <span style={{ color: drop > 0 ? "var(--color-health)" : "var(--color-ink)" }}>{after}%</span>
        </span>
        {open ? <ChevronUp size={15} className="shrink-0 text-faint" /> : <ChevronDown size={15} className="shrink-0 text-faint" />}
      </button>

      {open && (
        <div className="space-y-3 border-t border-edge px-4 py-3">
          {/* emotions */}
          <div className="flex flex-wrap gap-1.5">
            {data.emotions.map((em) => (
              <span
                key={em.category}
                className="rounded-sm border border-edge px-2 py-0.5 text-[11px] text-muted"
              >
                {emotionLabel(em.category)}{" "}
                <span className="tabnum">
                  {em.before}%<span className="text-faint">→</span>
                  <span style={{ color: em.after < em.before ? "var(--color-health)" : "var(--color-ink)" }}>
                    {em.after}%
                  </span>
                </span>
              </span>
            ))}
          </div>

          {/* thoughts worked through */}
          {(data.thoughts ?? []).map((th, i) => (
            <div key={i} className="rounded-sm border border-edge bg-white/[0.02] p-2.5">
              <div className="text-[13px] text-ink">“{th.thought}”</div>
              <div className="mt-1 flex flex-wrap items-center gap-1.5">
                <span className="tabnum text-[11px] text-faint">
                  {t("mood.belief", { before: th.belief_before, after: th.belief_after })}
                </span>
                {th.distortions.map((d) => (
                  <span
                    key={d}
                    className="rounded-sm px-1.5 py-0.5 text-[10px]"
                    style={{ background: "rgba(185,138,255,0.14)", color: "#b98aff" }}
                  >
                    {distortionName(d)}
                  </span>
                ))}
              </div>
              {th.positive_thought && (
                <div
                  className="mt-2 rounded-sm border-l-2 py-1 pl-2 text-[13px]"
                  style={{ borderColor: "var(--color-health)", color: "var(--color-muted)" }}
                >
                  {th.positive_thought}
                  <span className="tabnum ml-1.5 text-[10px] text-faint">{t("mood.believed", { n: th.positive_belief })}</span>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </motion.div>
  );
}

// --- guided 3-step flow ---------------------------------------------------------

function MoodLogFlow({ onClose }: { onClose: () => void }) {
  const { t } = useI18n();
  const [step, setStep] = useState(0);
  const [event, setEvent] = useState("");
  const [emotions, setEmotions] = useState<EmotionMap>({});
  const [thoughts, setThoughts] = useState<MoodThought[]>([emptyThought()]);

  const complete = useCompleteTool("daily_mood_log");
  const { celebrate } = useReward();
  const { data: openai } = useOpenAIStatus();
  const aiEnabled = !!openai?.connected;

  const chosenEmotions = useMemo(() => Object.keys(emotions), [emotions]);
  const validThoughts = thoughts.filter((t) => t.thought.trim() !== "");

  const canNext =
    step === 0 ? event.trim() !== "" && chosenEmotions.length > 0 : step === 1 ? validThoughts.length > 0 : true;

  const toggleEmotion = (key: string) =>
    setEmotions((m) => {
      const copy = { ...m };
      if (copy[key]) delete copy[key];
      else copy[key] = { before: 60, after: 60 };
      return copy;
    });

  const finish = () => {
    const data = {
      event: event.trim(),
      emotions: chosenEmotions.map((category) => ({
        category,
        before: emotions[category].before,
        after: emotions[category].after,
      })),
      thoughts: validThoughts,
    };
    complete.mutate(data, {
      onSuccess: (res) => {
        celebrate({
          title: t("mood.toolTitle"),
          xp_events: res.xp_events,
          level_ups: res.level_ups,
          label: t("reward.toolComplete"),
          gold: res.gold,
        });
        onClose();
      },
    });
  };

  return (
    <div className="mx-auto max-w-2xl">
      {/* Header */}
      <div className="mb-5 flex items-center justify-between">
        <button onClick={onClose} className="flex items-center gap-1.5 text-sm text-muted hover:text-ink">
          <ArrowLeft size={16} /> {t("common.back")}
        </button>
        <div className="flex items-center gap-1.5">
          {STEP_KEYS.map((_, i) => (
            <span
              key={i}
              className="h-1.5 rounded-full transition-all"
              style={{
                width: i === step ? 24 : 8,
                background: i <= step ? "var(--color-spirituality)" : "var(--color-edge)",
              }}
            />
          ))}
        </div>
      </div>

      <div className="mb-1 font-display text-[11px] uppercase tracking-[0.24em] text-[var(--color-spirituality)]">
        {t("mood.stepOf", { step: step + 1 })}
      </div>
      <h1 className="mb-5 font-display text-2xl font-bold tracking-tight text-ink">{t(STEP_KEYS[step])}</h1>

      <motion.div key={step} initial={{ opacity: 0, x: 12 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: 0.25 }}>
        {step === 0 && (
          <div className="space-y-6">
            <Field label={t("mood.whatHappened")} hint={t("mood.whatHappenedHint")}>
              <textarea
                autoFocus
                value={event}
                onChange={(e) => setEvent(e.target.value)}
                rows={3}
                placeholder={t("mood.eventPlaceholder")}
                className="w-full resize-none rounded-lg border border-edge bg-white/[0.03] px-3 py-2 text-sm text-ink placeholder:text-faint focus:border-[var(--color-spirituality)] focus:outline-none"
              />
            </Field>

            <Field label={t("mood.howFeel")} hint={t("mood.howFeelHint")}>
              <div className="flex flex-wrap gap-2">
                {EMOTIONS.map((e) => {
                  const active = !!emotions[e.key];
                  return (
                    <button
                      key={e.key}
                      onClick={() => toggleEmotion(e.key)}
                      title={e.also}
                      data-testid={`emotion-${e.key}`}
                      className="rounded-full border px-3 py-1.5 text-xs font-medium transition-all"
                      style={{
                        borderColor: active ? "var(--color-spirituality)" : "var(--color-edge)",
                        background: active ? "rgba(46,230,200,0.14)" : "transparent",
                        color: active ? "var(--color-spirituality)" : "var(--color-muted)",
                      }}
                    >
                      {e.label}
                    </button>
                  );
                })}
              </div>
              {chosenEmotions.length > 0 && (
                <div className="mt-4 space-y-3">
                  {chosenEmotions.map((key) => {
                    const meta = EMOTIONS.find((x) => x.key === key)!;
                    return (
                      <div key={key} className="rounded-lg border border-edge bg-white/[0.02] px-3 py-2">
                        <div className="mb-1 flex items-baseline justify-between">
                          <span className="text-sm text-ink">{meta.label}</span>
                          <span className="text-[10px] text-faint">{meta.also}</span>
                        </div>
                        <PercentSlider
                          value={emotions[key].before}
                          onChange={(v) =>
                            setEmotions((m) => ({ ...m, [key]: { before: v, after: v } }))
                          }
                        />
                      </div>
                    );
                  })}
                </div>
              )}
            </Field>
          </div>
        )}

        {step === 1 && (
          <div className="space-y-4">
            <p className="text-sm text-muted">
              {t("mood.thoughtsIntro1")}
              <em>{t("mood.hundredTrue")}</em>
              {t("mood.thoughtsIntro2")}
            </p>
            {thoughts.map((t, i) => (
              <ThoughtEditor
                key={i}
                index={i}
                thought={t}
                event={event}
                aiEnabled={aiEnabled}
                canRemove={thoughts.length > 1}
                onChange={(next) => setThoughts((ts) => ts.map((x, j) => (j === i ? next : x)))}
                onRemove={() => setThoughts((ts) => ts.filter((_, j) => j !== i))}
              />
            ))}
            <button
              onClick={() => setThoughts((ts) => [...ts, emptyThought()])}
              className="flex w-full items-center justify-center gap-1.5 rounded-lg border border-dashed border-edge py-2.5 text-xs text-muted hover:border-edge2 hover:text-ink"
            >
              <Plus size={14} /> {t("mood.addThought")}
            </button>
          </div>
        )}

        {step === 2 && (
          <div className="space-y-6">
            <Field label={t("mood.rerate")} hint={t("mood.rerateHint")}>
              <div className="space-y-3">
                {chosenEmotions.map((key) => {
                  const meta = EMOTIONS.find((x) => x.key === key)!;
                  const st = emotions[key];
                  return (
                    <div key={key} className="rounded-lg border border-edge bg-white/[0.02] px-3 py-2">
                      <div className="mb-1 flex items-baseline justify-between">
                        <span className="text-sm text-ink">{meta.label}</span>
                        <span className="tabnum text-[11px] text-faint">
                          {t("mood.wasNow", { before: st.before, after: st.after })}
                        </span>
                      </div>
                      <PercentSlider
                        value={st.after}
                        color="var(--color-health)"
                        onChange={(v) => setEmotions((m) => ({ ...m, [key]: { ...m[key], after: v } }))}
                      />
                    </div>
                  );
                })}
              </div>
            </Field>
            <Summary emotions={emotions} thoughts={validThoughts} />
          </div>
        )}
      </motion.div>

      {/* Footer nav */}
      <div className="mt-7 flex items-center justify-between">
        {step > 0 ? (
          <Btn variant="ghost" onClick={() => setStep((s) => s - 1)}>
            <ArrowLeft size={15} /> {t("common.back")}
          </Btn>
        ) : (
          <Btn variant="soft" onClick={onClose}>
            <X size={15} /> {t("common.cancel")}
          </Btn>
        )}
        {step < 2 ? (
          <Btn variant="primary" disabled={!canNext} onClick={() => setStep((s) => s + 1)} data-testid="mood-next">
            {t("mood.continue")} <ArrowRight size={15} />
          </Btn>
        ) : (
          <Btn variant="primary" disabled={complete.isPending} onClick={finish} data-testid="mood-finish">
            <Check size={16} /> {t("mood.finish")}
          </Btn>
        )}
      </div>
    </div>
  );
}

function ThoughtEditor({
  index,
  thought,
  event,
  aiEnabled,
  canRemove,
  onChange,
  onRemove,
}: {
  index: number;
  thought: MoodThought;
  event: string;
  aiEnabled: boolean;
  canRemove: boolean;
  onChange: (t: MoodThought) => void;
  onRemove: () => void;
}) {
  const { t } = useI18n();
  const set = (patch: Partial<MoodThought>) => onChange({ ...thought, ...patch });
  const toggleDistortion = (code: string) =>
    set({
      distortions: thought.distortions.includes(code)
        ? thought.distortions.filter((c) => c !== code)
        : [...thought.distortions, code],
    });

  const assist = useMoodAssist();
  const { requestConsent } = useAiConsent();
  const [hits, setHits] = useState<MoodDistortionHit[]>([]);
  const [candidates, setCandidates] = useState<MoodResponseIdea[]>([]);
  const [crisis, setCrisis] = useState<string | null>(null);
  const loadingMode = assist.isPending ? assist.variables?.mode : undefined;

  const runAssist = async (mode: "distortions" | "responses") => {
    if (thought.thought.trim() === "") {
      pushToast(t("mood.writeThoughtFirst"), "info");
      return;
    }
    if (!(await requestConsent())) return;
    assist.mutate(
      { mode, event, thought: thought.thought, distortions: thought.distortions },
      {
        onSuccess: (res) => {
          if (res.crisis) {
            setCrisis(res.crisis_message ?? t("mood.reachOut"));
            setHits([]);
            setCandidates([]);
            return;
          }
          setCrisis(null);
          if (mode === "distortions") {
            const codes = (res.distortions ?? []).map((d) => d.code);
            set({ distortions: Array.from(new Set([...thought.distortions, ...codes])) });
            setHits(res.distortions ?? []);
          } else {
            setCandidates(res.responses ?? []);
          }
        },
      },
    );
  };

  return (
    <div className="hud-panel space-y-3 p-4">
      <div className="flex items-center justify-between">
        <span className="font-display text-[11px] uppercase tracking-wider text-faint">{t("mood.thoughtN", { n: index + 1 })}</span>
        {canRemove && (
          <button onClick={onRemove} className="text-faint hover:text-[#ff8a80]" aria-label={t("mood.removeThought")}>
            <Trash2 size={14} />
          </button>
        )}
      </div>

      <div>
        <textarea
          value={thought.thought}
          onChange={(e) => set({ thought: e.target.value })}
          rows={2}
          placeholder={t("mood.thoughtPlaceholder")}
          className="w-full resize-none rounded-lg border border-edge bg-white/[0.03] px-3 py-2 text-sm text-ink placeholder:text-faint focus:border-[var(--color-spirituality)] focus:outline-none"
        />
        <div className="mt-2">
          <LabeledSlider label={t("mood.believeThis")} value={thought.belief_before} onChange={(v) => set({ belief_before: v })} />
        </div>
      </div>

      {crisis && (
        <div className="flex items-start gap-2.5 rounded-lg border border-[#ff8a80]/30 bg-[#ff4747]/[0.06] p-3">
          <HeartHandshake size={18} className="mt-0.5 shrink-0 text-[#ff8a80]" />
          <p className="text-[13px] leading-relaxed text-ink">{crisis}</p>
        </div>
      )}

      <div>
        <div className="mb-1.5 flex items-center justify-between">
          <span className="text-[11px] text-muted">{t("mood.distortionsIn")}</span>
          {aiEnabled && (
            <AssistButton
              id="find-distortions"
              label={t("mood.findDistortions")}
              loading={loadingMode === "distortions"}
              onClick={() => runAssist("distortions")}
            />
          )}
        </div>
        <div className="flex flex-wrap gap-1.5">
          {DISTORTIONS.map((d) => {
            const active = thought.distortions.includes(d.code);
            return (
              <button
                key={d.code}
                title={`${d.name} — ${d.blurb}`}
                onClick={() => toggleDistortion(d.code)}
                className="rounded-md border px-2 py-1 text-[11px] font-medium transition-all"
                style={{
                  borderColor: active ? "#b98aff" : "var(--color-edge)",
                  background: active ? "rgba(185,138,255,0.16)" : "transparent",
                  color: active ? "#cbaaff" : "var(--color-faint)",
                }}
              >
                {d.name}
              </button>
            );
          })}
        </div>
        {hits.length > 0 && (
          <ul className="mt-2 space-y-1">
            {hits.map((h) => {
              const meta = DISTORTIONS.find((d) => d.code === h.code);
              return (
                <li key={h.code} className="text-[11px] text-muted">
                  <span className="text-[#cbaaff]">{meta?.name ?? h.code}</span> — {h.why}
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <div className="rounded-lg border border-[var(--color-health)]/25 bg-[var(--color-health)]/[0.05] p-2.5">
        <div className="mb-1.5 flex items-center justify-between">
          <span className="text-[11px] text-muted">{t("mood.truerResponse")}</span>
          {aiEnabled && (
            <AssistButton
              id="suggest-a-response"
              label={t("mood.suggestResponse")}
              loading={loadingMode === "responses"}
              onClick={() => runAssist("responses")}
            />
          )}
        </div>
        <textarea
          value={thought.positive_thought}
          onChange={(e) => set({ positive_thought: e.target.value })}
          rows={2}
          placeholder={t("mood.responsePlaceholder")}
          className="w-full resize-none rounded-lg border border-edge bg-white/[0.03] px-3 py-2 text-sm text-ink placeholder:text-faint focus:border-[var(--color-health)] focus:outline-none"
        />
        {candidates.length > 0 && (
          <div className="mt-2 space-y-1.5">
            {candidates.map((c, i) => (
              <button
                key={i}
                onClick={() => {
                  set({ positive_thought: c.text });
                  setCandidates([]);
                }}
                data-testid={`use-response-${index}-${i}`}
                className="block w-full rounded-lg border border-edge bg-white/[0.02] p-2 text-left transition-colors hover:border-[var(--color-health)]/50"
              >
                <span className="mb-0.5 inline-block rounded px-1.5 py-0.5 text-[9px] uppercase tracking-wide"
                  style={{ background: "rgba(75,255,126,0.14)", color: "var(--color-health)" }}>
                  {c.technique}
                </span>
                <p className="text-[13px] leading-snug text-ink">{c.text}</p>
              </button>
            ))}
            <p className="text-[10px] text-faint">{t("mood.tapToUse")}</p>
          </div>
        )}
        <div className="mt-2 space-y-2">
          <LabeledSlider
            label={t("mood.believeResponse")}
            value={thought.positive_belief}
            color="var(--color-health)"
            onChange={(v) => set({ positive_belief: v })}
          />
          <LabeledSlider
            label={t("mood.nowBelieve")}
            value={thought.belief_after}
            color="var(--color-gold)"
            onChange={(v) => set({ belief_after: v })}
          />
        </div>
      </div>
    </div>
  );
}

// `id` keeps the data-testid stable across languages.
function AssistButton({
  id,
  label,
  loading,
  onClick,
}: {
  id: string;
  label: string;
  loading?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      disabled={loading}
      data-testid={`assist-${id}`}
      className="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-medium transition-colors disabled:opacity-60"
      style={{ color: "var(--color-spirituality)" }}
    >
      {loading ? <Loader2 size={12} className="animate-spin" /> : <Sparkles size={12} />}
      {label}
    </button>
  );
}

function Summary({ emotions, thoughts }: { emotions: EmotionMap; thoughts: MoodThought[] }) {
  const { t, tp } = useI18n();
  const vals = Object.values(emotions);
  const avg = (fn: (e: EmotionState) => number) =>
    vals.length ? Math.round(vals.reduce((s, e) => s + fn(e), 0) / vals.length) : 0;
  const before = avg((e) => e.before);
  const after = avg((e) => e.after);
  const drop = before - after;
  return (
    <div className="hud-panel clip-corner p-4 text-center">
      <div className="mb-1 flex items-center justify-center gap-2 font-display text-[11px] uppercase tracking-wider text-faint">
        <Sparkles size={13} style={{ color: "var(--color-spirituality)" }} /> {t("mood.yourShift")}
      </div>
      <div className="tabnum text-2xl font-bold text-ink">
        {before}% <span className="text-faint">→</span>{" "}
        <span style={{ color: drop > 0 ? "var(--color-health)" : "var(--color-ink)" }}>{after}%</span>
      </div>
      <p className="mt-1 text-xs text-muted">
        {t("mood.avgDistress")}
        {drop > 0 ? t("mood.downPoints", { n: drop }) : ""} · {tp("mood.reframed", thoughts.length)}
      </p>
    </div>
  );
}

// --- small controls ---------------------------------------------------------

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="mb-2">
        <div className="text-sm font-semibold text-ink">{label}</div>
        {hint && <div className="text-xs text-faint">{hint}</div>}
      </div>
      {children}
    </div>
  );
}

function PercentSlider({
  value,
  onChange,
  color = "var(--color-spirituality)",
}: {
  value: number;
  onChange: (v: number) => void;
  color?: string;
}) {
  return (
    <div className="flex items-center gap-3">
      <input
        type="range"
        min={0}
        max={100}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full"
        style={{ background: `linear-gradient(90deg, ${color} ${value}%, rgba(255,255,255,0.08) ${value}%)` }}
      />
      <span className="tabnum w-10 shrink-0 text-right text-sm" style={{ color }}>
        {value}%
      </span>
    </div>
  );
}

function LabeledSlider({
  label,
  value,
  onChange,
  color,
}: {
  label: string;
  value: number;
  onChange: (v: number) => void;
  color?: string;
}) {
  return (
    <div>
      <div className="mb-0.5 text-[11px] text-muted">{label}</div>
      <PercentSlider value={value} onChange={onChange} color={color} />
    </div>
  );
}
