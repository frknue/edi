import { useMemo, useState } from "react";
import { motion } from "framer-motion";
import { ArrowLeft, ArrowRight, Check, HeartHandshake, Loader2, Plus, Sparkles, Trash2, X } from "lucide-react";
import { useCompleteTool, useMoodAssist, useOpenAIStatus } from "../lib/queries";
import { useReward } from "../lib/reward";
import { useAiConsent } from "../lib/aiConsent";
import { pushToast } from "../lib/toast";
import { EMOTIONS, DISTORTIONS } from "../lib/cbt";
import type { MoodDistortionHit, MoodResponseIdea, MoodThought } from "../lib/types";
import { Btn } from "./ui";

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

const STEPS = ["The moment", "The thoughts", "Re-rate & finish"];

export function DailyMoodLog({ onClose }: { onClose: () => void }) {
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
          title: "Daily Mood Log",
          xp_events: res.xp_events,
          level_ups: res.level_ups,
          label: "Tool Complete",
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
          <ArrowLeft size={16} /> Tools
        </button>
        <div className="flex items-center gap-1.5">
          {STEPS.map((_, i) => (
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
        Daily Mood Log · Step {step + 1}/3
      </div>
      <h1 className="mb-5 font-display text-2xl font-bold tracking-tight text-ink">{STEPS[step]}</h1>

      <motion.div key={step} initial={{ opacity: 0, x: 12 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: 0.25 }}>
        {step === 0 && (
          <div className="space-y-6">
            <Field label="What happened?" hint="One specific upsetting moment — the who, what, when.">
              <textarea
                autoFocus
                value={event}
                onChange={(e) => setEvent(e.target.value)}
                rows={3}
                placeholder="e.g. My manager gave critical feedback on the project in front of the team."
                className="w-full resize-none rounded-lg border border-edge bg-white/[0.03] px-3 py-2 text-sm text-ink placeholder:text-faint focus:border-[var(--color-spirituality)] focus:outline-none"
              />
            </Field>

            <Field label="How did you feel?" hint="Pick the feelings, then rate how strong each was (0–100%).">
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
                        background: active ? "rgba(69,224,208,0.14)" : "transparent",
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
              Write the automatic negative thoughts running through your mind, tag the distortions in each, then
              answer with a response that is <em>100% true</em> and takes the sting out.
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
              <Plus size={14} /> Add another thought
            </button>
          </div>
        )}

        {step === 2 && (
          <div className="space-y-6">
            <Field label="Re-rate your feelings" hint="Now that you've answered the thoughts, how strong is each feeling?">
              <div className="space-y-3">
                {chosenEmotions.map((key) => {
                  const meta = EMOTIONS.find((x) => x.key === key)!;
                  const st = emotions[key];
                  return (
                    <div key={key} className="rounded-lg border border-edge bg-white/[0.02] px-3 py-2">
                      <div className="mb-1 flex items-baseline justify-between">
                        <span className="text-sm text-ink">{meta.label}</span>
                        <span className="tabnum text-[11px] text-faint">
                          was {st.before}% → now {st.after}%
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
            <ArrowLeft size={15} /> Back
          </Btn>
        ) : (
          <Btn variant="soft" onClick={onClose}>
            <X size={15} /> Cancel
          </Btn>
        )}
        {step < 2 ? (
          <Btn variant="primary" disabled={!canNext} onClick={() => setStep((s) => s + 1)} data-testid="mood-next">
            Continue <ArrowRight size={15} />
          </Btn>
        ) : (
          <Btn variant="primary" disabled={complete.isPending} onClick={finish} data-testid="mood-finish">
            <Check size={16} /> Finish & earn XP
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
      pushToast("Write the negative thought first", "info");
      return;
    }
    if (!(await requestConsent())) return;
    assist.mutate(
      { mode, event, thought: thought.thought, distortions: thought.distortions },
      {
        onSuccess: (res) => {
          if (res.crisis) {
            setCrisis(res.crisis_message ?? "Please reach out for support.");
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
        <span className="font-display text-[11px] uppercase tracking-wider text-faint">Thought {index + 1}</span>
        {canRemove && (
          <button onClick={onRemove} className="text-faint hover:text-[#ff7d9d]" aria-label="Remove thought">
            <Trash2 size={14} />
          </button>
        )}
      </div>

      <div>
        <textarea
          value={thought.thought}
          onChange={(e) => set({ thought: e.target.value })}
          rows={2}
          placeholder="Negative thought — e.g. “I'm not good enough.”"
          className="w-full resize-none rounded-lg border border-edge bg-white/[0.03] px-3 py-2 text-sm text-ink placeholder:text-faint focus:border-[var(--color-spirituality)] focus:outline-none"
        />
        <div className="mt-2">
          <LabeledSlider label="I believe this" value={thought.belief_before} onChange={(v) => set({ belief_before: v })} />
        </div>
      </div>

      {crisis && (
        <div className="flex items-start gap-2.5 rounded-lg border border-[#ff7d9d]/30 bg-[#ff3b6b]/[0.06] p-3">
          <HeartHandshake size={18} className="mt-0.5 shrink-0 text-[#ff7d9d]" />
          <p className="text-[13px] leading-relaxed text-ink">{crisis}</p>
        </div>
      )}

      <div>
        <div className="mb-1.5 flex items-center justify-between">
          <span className="text-[11px] text-muted">Distortions in this thought</span>
          {aiEnabled && <AssistButton label="Find distortions" loading={loadingMode === "distortions"} onClick={() => runAssist("distortions")} />}
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
                  borderColor: active ? "#b18bff" : "var(--color-edge)",
                  background: active ? "rgba(177,139,255,0.16)" : "transparent",
                  color: active ? "#c4a8ff" : "var(--color-faint)",
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
                  <span className="text-[#c4a8ff]">{meta?.name ?? h.code}</span> — {h.why}
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <div className="rounded-lg border border-[var(--color-health)]/25 bg-[var(--color-health)]/[0.05] p-2.5">
        <div className="mb-1.5 flex items-center justify-between">
          <span className="text-[11px] text-muted">A truer, kinder response</span>
          {aiEnabled && <AssistButton label="Suggest a response" loading={loadingMode === "responses"} onClick={() => runAssist("responses")} />}
        </div>
        <textarea
          value={thought.positive_thought}
          onChange={(e) => set({ positive_thought: e.target.value })}
          rows={2}
          placeholder="Must be 100% true — the kind of thing you'd tell a friend."
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
                  style={{ background: "rgba(62,229,148,0.14)", color: "var(--color-health)" }}>
                  {c.technique}
                </span>
                <p className="text-[13px] leading-snug text-ink">{c.text}</p>
              </button>
            ))}
            <p className="text-[10px] text-faint">Tap one to use it, then make it your own.</p>
          </div>
        )}
        <div className="mt-2 space-y-2">
          <LabeledSlider
            label="I believe the response"
            value={thought.positive_belief}
            color="var(--color-health)"
            onChange={(v) => set({ positive_belief: v })}
          />
          <LabeledSlider
            label="Now I believe the negative thought"
            value={thought.belief_after}
            color="var(--color-gold)"
            onChange={(v) => set({ belief_after: v })}
          />
        </div>
      </div>
    </div>
  );
}

function AssistButton({ label, loading, onClick }: { label: string; loading?: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      disabled={loading}
      data-testid={`assist-${label.toLowerCase().replace(/\s+/g, "-")}`}
      className="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-medium transition-colors disabled:opacity-60"
      style={{ color: "var(--color-spirituality)" }}
    >
      {loading ? <Loader2 size={12} className="animate-spin" /> : <Sparkles size={12} />}
      {label}
    </button>
  );
}

function Summary({ emotions, thoughts }: { emotions: EmotionMap; thoughts: MoodThought[] }) {
  const vals = Object.values(emotions);
  const avg = (fn: (e: EmotionState) => number) =>
    vals.length ? Math.round(vals.reduce((s, e) => s + fn(e), 0) / vals.length) : 0;
  const before = avg((e) => e.before);
  const after = avg((e) => e.after);
  const drop = before - after;
  return (
    <div className="hud-panel clip-corner p-4 text-center">
      <div className="mb-1 flex items-center justify-center gap-2 font-display text-[11px] uppercase tracking-wider text-faint">
        <Sparkles size={13} style={{ color: "var(--color-spirituality)" }} /> Your shift
      </div>
      <div className="tabnum text-2xl font-bold text-ink">
        {before}% <span className="text-faint">→</span>{" "}
        <span style={{ color: drop > 0 ? "var(--color-health)" : "var(--color-ink)" }}>{after}%</span>
      </div>
      <p className="mt-1 text-xs text-muted">
        Average distress{drop > 0 ? ` down ${drop} points` : ""} · {thoughts.length} thought
        {thoughts.length === 1 ? "" : "s"} reframed
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
