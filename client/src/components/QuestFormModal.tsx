import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { ChevronDown, ChevronUp, Minus, Plus, Trash2, X } from "lucide-react";
import type { Difficulty, Quest, QuestInput, QuestType, SubtaskInput } from "../lib/types";
import { attributeMeta, difficultyMeta, getType, typeMeta } from "../lib/theme";
import { Btn, RewardChips } from "./ui";

const TYPES = Object.keys(typeMeta) as QuestType[];
const DIFFICULTIES = Object.keys(difficultyMeta) as Difficulty[];

interface Props {
  open: boolean;
  initial?: Quest | null;
  busy?: boolean;
  error?: string | null;
  onClose: () => void;
  onSubmit: (input: QuestInput, id?: number) => void;
}

export function QuestFormModal({ open, initial, busy, error, onClose, onSubmit }: Props) {
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [type, setType] = useState<QuestType>("daily");
  const [difficulty, setDifficulty] = useState<Difficulty>("easy");
  const [rewards, setRewards] = useState<Record<string, number>>({});
  const [subtasks, setSubtasks] = useState<SubtaskInput[]>([]);
  const [expandedSub, setExpandedSub] = useState<number | null>(null);

  // Reset form whenever the modal opens (for create or edit).
  useEffect(() => {
    if (!open) return;
    setTitle(initial?.title ?? "");
    setDescription(initial?.description ?? "");
    setType(initial?.type ?? "daily");
    setDifficulty(initial?.difficulty ?? "easy");
    setRewards(initial?.attribute_rewards ? { ...initial.attribute_rewards } : {});
    setSubtasks(
      initial?.subtasks?.map((st) => ({ title: st.title, attribute_rewards: { ...st.attribute_rewards } })) ?? [],
    );
    setExpandedSub(null);
  }, [open, initial]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  const bump = (key: string, delta: number) =>
    setRewards((r) => {
      const next = Math.max(0, (r[key] ?? 0) + delta);
      const copy = { ...r };
      if (next === 0) delete copy[key];
      else copy[key] = next;
      return copy;
    });

  const setSubtask = (i: number, patch: Partial<SubtaskInput>) =>
    setSubtasks((ss) => ss.map((s, j) => (j === i ? { ...s, ...patch } : s)));

  const bumpSubtaskReward = (i: number, key: string, delta: number) =>
    setSubtasks((ss) =>
      ss.map((s, j) => {
        if (j !== i) return s;
        const next = Math.max(0, (s.attribute_rewards[key] ?? 0) + delta);
        const copy = { ...s.attribute_rewards };
        if (next === 0) delete copy[key];
        else copy[key] = next;
        return { ...s, attribute_rewards: copy };
      }),
    );

  const submit = () => {
    const cleaned: Record<string, number> = {};
    for (const [k, v] of Object.entries(rewards)) if (v > 0) cleaned[k] = v;
    const cleanedSubs = subtasks
      .filter((s) => s.title.trim() !== "")
      .map((s) => ({ title: s.title.trim(), attribute_rewards: s.attribute_rewards }));
    onSubmit(
      {
        title: title.trim(),
        description: description.trim(),
        type,
        difficulty,
        attribute_rewards: cleaned,
        subtasks: cleanedSubs,
      },
      initial?.id,
    );
  };

  return (
    <AnimatePresence>
      {open && (
        <motion.div
          className="fixed inset-0 z-40 flex items-end justify-center p-0 sm:items-center sm:p-6"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          style={{ background: "rgba(4,5,9,0.7)", backdropFilter: "blur(4px)" }}
          onClick={onClose}
        >
          <motion.div
            className="hud-panel w-full max-w-lg overflow-hidden"
            initial={{ y: 30, opacity: 0, scale: 0.98 }}
            animate={{ y: 0, opacity: 1, scale: 1 }}
            exit={{ y: 20, opacity: 0 }}
            transition={{ type: "spring", stiffness: 300, damping: 28 }}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-edge px-5 py-3.5">
              <h2 className="font-display text-sm font-semibold uppercase tracking-[0.18em] text-ink">
                {initial ? "Edit quest" : "New quest"}
              </h2>
              <button onClick={onClose} className="text-faint hover:text-ink" aria-label="Close">
                <X size={18} />
              </button>
            </div>

            <div className="max-h-[70vh] space-y-4 overflow-y-auto px-5 py-4">
              <div>
                <label className="mb-1 block text-xs font-medium text-muted">Title</label>
                <input
                  autoFocus
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  placeholder="e.g. 30 minute workout"
                  className="w-full rounded-lg border border-edge bg-white/[0.03] px-3 py-2 text-sm text-ink placeholder:text-faint focus:border-[var(--color-gold)] focus:outline-none"
                />
              </div>

              <div>
                <label className="mb-1 block text-xs font-medium text-muted">Description</label>
                <textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  rows={2}
                  placeholder="Optional details…"
                  className="w-full resize-none rounded-lg border border-edge bg-white/[0.03] px-3 py-2 text-sm text-ink placeholder:text-faint focus:border-[var(--color-gold)] focus:outline-none"
                />
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-medium text-muted">Type</label>
                <div className="flex flex-wrap gap-1.5">
                  {TYPES.map((t) => {
                    const m = getType(t);
                    const active = type === t;
                    const Icon = m.Icon;
                    return (
                      <button
                        key={t}
                        onClick={() => setType(t)}
                        className="inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs font-medium transition-all"
                        style={{
                          borderColor: active ? m.color : "var(--color-edge)",
                          background: active ? `${m.color}1f` : "transparent",
                          color: active ? m.color : "var(--color-muted)",
                        }}
                      >
                        <Icon size={13} />
                        {m.label}
                      </button>
                    );
                  })}
                </div>
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-medium text-muted">Difficulty</label>
                <div className="flex flex-wrap gap-1.5">
                  {DIFFICULTIES.map((d) => {
                    const m = difficultyMeta[d];
                    const active = difficulty === d;
                    return (
                      <button
                        key={d}
                        onClick={() => setDifficulty(d)}
                        className="rounded-lg border px-2.5 py-1.5 text-xs font-medium transition-all"
                        style={{
                          borderColor: active ? m.color : "var(--color-edge)",
                          background: active ? `${m.color}1f` : "transparent",
                          color: active ? m.color : "var(--color-muted)",
                        }}
                      >
                        {m.label}
                      </button>
                    );
                  })}
                </div>
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-medium text-muted">
                  Attribute rewards (XP)
                </label>
                <div className="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
                  {Object.entries(attributeMeta).map(([key, meta]) => {
                    const val = rewards[key] ?? 0;
                    const Icon = meta.Icon;
                    return (
                      <div
                        key={key}
                        className="flex items-center justify-between rounded-lg border border-edge bg-white/[0.02] px-2.5 py-1.5"
                        style={val > 0 ? { borderColor: `${meta.color}55` } : undefined}
                      >
                        <span className="flex items-center gap-1.5 text-xs" style={{ color: val > 0 ? meta.color : "var(--color-muted)" }}>
                          <Icon size={13} />
                          {meta.label}
                        </span>
                        <span className="flex items-center gap-1">
                          <button
                            onClick={() => bump(key, -5)}
                            className="grid h-6 w-6 place-items-center rounded text-faint hover:bg-white/5 hover:text-ink"
                          >
                            <Minus size={12} />
                          </button>
                          <span className="tabnum w-7 text-center text-xs font-semibold text-ink">{val}</span>
                          <button
                            onClick={() => bump(key, 5)}
                            className="grid h-6 w-6 place-items-center rounded text-faint hover:bg-white/5 hover:text-ink"
                          >
                            <Plus size={12} />
                          </button>
                        </span>
                      </div>
                    );
                  })}
                </div>
              </div>

              <div>
                <div className="mb-1.5 flex items-center justify-between">
                  <label className="block text-xs font-medium text-muted">
                    Bonus objectives <span className="text-faint">(optional subtasks — extra XP if checked)</span>
                  </label>
                  <button
                    onClick={() => {
                      setSubtasks((ss) => [...ss, { title: "", attribute_rewards: {} }]);
                      setExpandedSub(subtasks.length);
                    }}
                    data-testid="add-subtask"
                    className="inline-flex items-center gap-1 text-[11px] font-medium text-[var(--color-spirituality)]"
                  >
                    <Plus size={12} /> Add
                  </button>
                </div>
                {subtasks.length > 0 && (
                  <div className="space-y-1.5">
                    {subtasks.map((st, i) => {
                      const expanded = expandedSub === i;
                      return (
                        <div key={i} className="rounded-lg border border-edge bg-white/[0.02]">
                          <div className="flex items-center gap-1.5 px-2 py-1.5">
                            <input
                              value={st.title}
                              onChange={(e) => setSubtask(i, { title: e.target.value })}
                              placeholder="e.g. Bike there instead of driving"
                              data-testid={`subtask-title-${i}`}
                              className="min-w-0 flex-1 bg-transparent text-xs text-ink placeholder:text-faint focus:outline-none"
                            />
                            <RewardChips rewards={st.attribute_rewards} />
                            <button
                              onClick={() => setExpandedSub(expanded ? null : i)}
                              className="text-faint hover:text-ink"
                              aria-label="Edit bonus rewards"
                              data-testid={`subtask-expand-${i}`}
                            >
                              {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
                            </button>
                            <button
                              onClick={() => {
                                setSubtasks((ss) => ss.filter((_, j) => j !== i));
                                setExpandedSub(null);
                              }}
                              className="text-faint hover:text-[#ff7d9d]"
                              aria-label="Remove subtask"
                            >
                              <Trash2 size={13} />
                            </button>
                          </div>
                          {expanded && (
                            <div className="grid grid-cols-1 gap-1 border-t border-edge p-2 sm:grid-cols-2">
                              {Object.entries(attributeMeta).map(([key, meta]) => {
                                const val = st.attribute_rewards[key] ?? 0;
                                const Icon = meta.Icon;
                                return (
                                  <div key={key} className="flex items-center justify-between rounded-md px-1.5 py-0.5">
                                    <span
                                      className="flex items-center gap-1 text-[11px]"
                                      style={{ color: val > 0 ? meta.color : "var(--color-faint)" }}
                                    >
                                      <Icon size={11} />
                                      {meta.label}
                                    </span>
                                    <span className="flex items-center gap-0.5">
                                      <button
                                        onClick={() => bumpSubtaskReward(i, key, -5)}
                                        className="grid h-5 w-5 place-items-center rounded text-faint hover:bg-white/5 hover:text-ink"
                                      >
                                        <Minus size={10} />
                                      </button>
                                      <span className="tabnum w-6 text-center text-[11px] font-semibold text-ink">{val}</span>
                                      <button
                                        onClick={() => bumpSubtaskReward(i, key, 5)}
                                        data-testid={`subtask-${i}-plus-${key}`}
                                        className="grid h-5 w-5 place-items-center rounded text-faint hover:bg-white/5 hover:text-ink"
                                      >
                                        <Plus size={10} />
                                      </button>
                                    </span>
                                  </div>
                                );
                              })}
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>

              {error && <p className="text-xs text-[#ff7d9d]">{error}</p>}
            </div>

            <div className="flex items-center justify-end gap-2 border-t border-edge px-5 py-3.5">
              <Btn variant="ghost" onClick={onClose}>
                Cancel
              </Btn>
              <Btn variant="primary" disabled={busy || title.trim() === ""} onClick={submit}>
                {initial ? "Save changes" : "Create quest"}
              </Btn>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
