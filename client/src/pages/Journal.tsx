import { useMemo, useState } from "react";
import { motion } from "framer-motion";
import { BatteryCharging, BookHeart, Pencil, Search, Smile, Trash2, X } from "lucide-react";
import { useJournal, useCreateJournal, useUpdateJournal, useDeleteJournal } from "../lib/queries";
import { useReward } from "../lib/reward";
import { pushToast } from "../lib/toast";
import { Btn, EmptyState, SectionTitle, Spinner } from "../components/ui";
import { relativeTime } from "../lib/format";
import type { JournalEntry } from "../lib/types";

function scoreColor(v: number): string {
  if (v <= 3) return "#ff5f56";
  if (v <= 6) return "#ffb000";
  return "#4bff7e";
}

function ScoreSlider({
  label,
  value,
  onChange,
  icon,
}: {
  label: string;
  value: number;
  onChange: (v: number) => void;
  icon: React.ReactNode;
}) {
  const color = scoreColor(value);
  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between">
        <span className="flex items-center gap-1.5 text-xs font-medium text-muted">
          {icon} {label}
        </span>
        <span className="tabnum text-lg font-bold" style={{ color }}>
          {value}
          <span className="text-xs text-faint">/10</span>
        </span>
      </div>
      <input
        type="range"
        min={1}
        max={10}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        style={{ background: `linear-gradient(90deg, ${color} ${(value - 1) * 11.1}%, rgba(255,255,255,0.08) ${(value - 1) * 11.1}%)` }}
        className="w-full"
      />
    </div>
  );
}

// --- trends -------------------------------------------------------------------

interface DayPoint {
  date: string; // YYYY-MM-DD (local)
  mood: number;
  energy: number;
  count: number;
}

function localDay(iso: string): string {
  const d = new Date(iso);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

// Per-day averages from raw entries.
function toDays(entries: JournalEntry[]): Map<string, DayPoint> {
  const acc = new Map<string, { m: number; e: number; n: number }>();
  for (const en of entries) {
    const key = localDay(en.created_at);
    const cur = acc.get(key) ?? { m: 0, e: 0, n: 0 };
    acc.set(key, { m: cur.m + en.mood, e: cur.e + en.energy, n: cur.n + 1 });
  }
  const out = new Map<string, DayPoint>();
  for (const [date, { m, e, n }] of acc) {
    out.set(date, { date, mood: m / n, energy: e / n, count: n });
  }
  return out;
}

// Single-series sparkline: one hue, 2px line, latest value direct-labeled,
// per-point tooltips. (Two measures = two charts — never a dual axis.)
function Sparkline({ label, color, points }: { label: string; color: string; points: DayPoint[]; }) {
  const vals = points.map((p) => ({ ...p, v: label === "mood" ? p.mood : p.energy }));
  const W = 220;
  const H = 40;
  const n = vals.length;
  const x = (i: number) => (n === 1 ? W / 2 : (i / (n - 1)) * (W - 8) + 4);
  const y = (v: number) => H - 4 - ((v - 1) / 9) * (H - 8); // scale 1..10
  const path = vals.map((p, i) => `${i === 0 ? "M" : "L"}${x(i).toFixed(1)},${y(p.v).toFixed(1)}`).join(" ");
  const last = vals[vals.length - 1];

  return (
    <div className="flex items-center gap-3">
      <span className="w-14 shrink-0 text-[10px] uppercase tracking-wider text-faint">{label}</span>
      {n === 0 ? (
        <span className="text-[11px] text-faint">no data yet</span>
      ) : (
        <>
          <svg width={W} height={H} className="shrink-0" role="img" aria-label={`${label} per day`}>
            <path d={path} fill="none" stroke={color} strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />
            {vals.map((p, i) => (
              <circle key={p.date} cx={x(i)} cy={y(p.v)} r="6" fill="transparent">
                <title>{`${p.date} · ${label} ${p.v.toFixed(1)}/10`}</title>
              </circle>
            ))}
            <circle cx={x(n - 1)} cy={y(last.v)} r="2.5" fill={color} />
          </svg>
          <span className="tabnum text-sm font-semibold" style={{ color }}>
            {last.v.toFixed(1)}
          </span>
        </>
      )}
    </div>
  );
}

// Consistency heatmap: last 10 weeks, sequential single-hue phosphor ramp —
// brighter green = better mood that day; empty cell = no entry.
function Heatmap({ days }: { days: Map<string, DayPoint> }) {
  const weeks = 10;
  const today = new Date();
  // Start on the Monday `weeks` back.
  const start = new Date(today);
  start.setDate(today.getDate() - ((today.getDay() + 6) % 7) - (weeks - 1) * 7);

  const cols: { date: Date; key: string }[][] = [];
  for (let w = 0; w < weeks; w++) {
    const col: { date: Date; key: string }[] = [];
    for (let d = 0; d < 7; d++) {
      const date = new Date(start);
      date.setDate(start.getDate() + w * 7 + d);
      const key = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
      col.push({ date, key });
    }
    cols.push(col);
  }

  return (
    <div className="flex items-center gap-3">
      <span className="w-14 shrink-0 text-[10px] uppercase tracking-wider text-faint">{weeks} weeks</span>
      <div className="flex gap-[3px]">
        {cols.map((col, w) => (
          <div key={w} className="flex flex-col gap-[3px]">
            {col.map(({ date, key }) => {
              const dp = days.get(key);
              const future = date > today;
              const bg = future
                ? "transparent"
                : dp
                  ? `rgba(75,255,126,${(0.18 + (dp.mood / 10) * 0.72).toFixed(2)})`
                  : "rgba(255,255,255,0.05)";
              return (
                <div
                  key={key}
                  className="h-[10px] w-[10px] rounded-[2px]"
                  style={{ background: bg }}
                  title={dp ? `${key} · mood ${dp.mood.toFixed(1)} · ${dp.count} entr${dp.count === 1 ? "y" : "ies"}` : future ? "" : `${key} · no entry`}
                />
              );
            })}
          </div>
        ))}
      </div>
      <div className="flex items-center gap-1 text-[9px] text-faint">
        low
        {[0.25, 0.5, 0.75, 0.95].map((a) => (
          <span key={a} className="h-[8px] w-[8px] rounded-[2px]" style={{ background: `rgba(75,255,126,${a})` }} />
        ))}
        high
      </div>
    </div>
  );
}

function TrendsPanel({ entries }: { entries: JournalEntry[] }) {
  const days = useMemo(() => toDays(entries), [entries]);
  const recent = useMemo(() => {
    const cutoff = new Date();
    cutoff.setDate(cutoff.getDate() - 30);
    return [...days.values()]
      .filter((d) => new Date(d.date) >= cutoff)
      .sort((a, b) => a.date.localeCompare(b.date));
  }, [days]);

  if (entries.length === 0) return null;
  return (
    <section className="hud-panel clip-corner space-y-2.5 px-4 py-3">
      <div className="font-display text-sm uppercase tracking-[0.14em] text-ink">
        <span className="mr-1.5 text-[var(--color-phos)]">▸</span>Trends
        <span className="ml-2 text-[10px] normal-case tracking-normal text-faint">last 30 days · per-day averages</span>
      </div>
      <Sparkline label="mood" color="#4bff7e" points={recent} />
      <Sparkline label="energy" color="#ffb000" points={recent} />
      <Heatmap days={days} />
    </section>
  );
}

// --- page ---------------------------------------------------------------------

export function JournalPage() {
  const [query, setQuery] = useState("");
  const { data: entries, isLoading } = useJournal(query);
  // Unfiltered set for trends (so searching doesn't reshape the charts).
  const { data: allEntries } = useJournal("", 200);

  const createJournal = useCreateJournal();
  const updateJournal = useUpdateJournal();
  const deleteJournal = useDeleteJournal();
  const { celebrate } = useReward();

  const [mood, setMood] = useState(6);
  const [energy, setEnergy] = useState(6);
  const [notes, setNotes] = useState("");
  const [saved, setSaved] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);

  const startEdit = (e: JournalEntry) => {
    setEditingId(e.id);
    setMood(e.mood);
    setEnergy(e.energy);
    setNotes(e.notes);
  };

  const cancelEdit = () => {
    setEditingId(null);
    setNotes("");
    setMood(6);
    setEnergy(6);
  };

  const submit = () => {
    if (editingId !== null) {
      updateJournal.mutate(
        { id: editingId, patch: { mood, energy, notes: notes.trim() } },
        {
          onSuccess: () => {
            pushToast("Reflection updated", "success");
            cancelEdit();
          },
        },
      );
      return;
    }
    createJournal.mutate(
      { mood, energy, notes: notes.trim() },
      {
        onSuccess: (res) => {
          setNotes("");
          if (res.xp_events.length > 0) {
            celebrate({
              title: "Daily reflection",
              xp_events: res.xp_events,
              level_ups: res.level_ups,
              label: "Journal",
            });
          } else {
            setSaved(true);
            window.setTimeout(() => setSaved(false), 2000);
          }
        },
      },
    );
  };

  const remove = (e: JournalEntry) => {
    if (!window.confirm("Delete this reflection? This cannot be undone.")) return;
    if (editingId === e.id) cancelEdit();
    deleteJournal.mutate(e.id, { onSuccess: () => pushToast("Reflection deleted", "info") });
  };

  const busy = createJournal.isPending || updateJournal.isPending;

  return (
    <div className="space-y-5">
      <div>
        <h1 className="font-display text-3xl leading-tight text-ink">Journal</h1>
        <p className="text-sm text-faint">
          Log how today felt. The first reflection of each day earns XP.
        </p>
      </div>

      {allEntries && <TrendsPanel entries={allEntries} />}

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Composer */}
        <section className="hud-panel clip-corner space-y-5 p-5">
          <SectionTitle hint={editingId !== null ? `Editing entry #${editingId}` : "Rate 1-10 and jot a note."}>
            {editingId !== null ? "Edit reflection" : "Today's check-in"}
          </SectionTitle>
          <ScoreSlider label="Mood" value={mood} onChange={setMood} icon={<Smile size={13} />} />
          <ScoreSlider label="Energy" value={energy} onChange={setEnergy} icon={<BatteryCharging size={13} />} />
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted">Notes</label>
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              rows={5}
              placeholder="What went well? What drained you? What will you do tomorrow?"
              className="w-full resize-none rounded-lg border border-edge bg-white/[0.03] px-3 py-2 text-sm text-ink placeholder:text-faint focus:border-[var(--color-gold)] focus:outline-none"
            />
          </div>
          <div className="flex items-center justify-end gap-3">
            {saved && (
              <motion.span initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="text-xs text-[var(--color-health)]">
                Saved ✓
              </motion.span>
            )}
            {editingId !== null && (
              <Btn variant="soft" onClick={cancelEdit}>
                <X size={14} /> Cancel
              </Btn>
            )}
            <Btn variant="primary" disabled={busy} onClick={submit} data-testid="save-journal">
              <BookHeart size={15} /> {editingId !== null ? "Save changes" : "Save reflection"}
            </Btn>
          </div>
        </section>

        {/* History */}
        <section>
          <SectionTitle
            hint="Most recent first."
            action={
              <label className="flex items-center gap-1.5 rounded-sm border border-edge bg-white/[0.02] px-2 py-1">
                <Search size={13} className="text-faint" />
                <input
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="search notes…"
                  data-testid="journal-search"
                  className="w-32 bg-transparent text-xs text-ink placeholder:text-faint focus:outline-none"
                />
                {query && (
                  <button onClick={() => setQuery("")} className="text-faint hover:text-ink" aria-label="Clear search">
                    <X size={12} />
                  </button>
                )}
              </label>
            }
          >
            Reflections
          </SectionTitle>
          {isLoading ? (
            <Spinner />
          ) : !entries || entries.length === 0 ? (
            <EmptyState
              icon={<BookHeart size={20} />}
              title={query ? "No matches" : "No reflections yet"}
              hint={query ? "Try a different search." : "Your first entry will appear here."}
            />
          ) : (
            <div className="space-y-2.5">
              {entries.map((e, i) => (
                <motion.div
                  key={e.id}
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: Math.min(i * 0.04, 0.3) }}
                  className={`hud-panel group p-3.5 ${editingId === e.id ? "border-[var(--color-gold)]" : ""}`}
                >
                  <div className="mb-1.5 flex items-center justify-between">
                    <div className="flex items-center gap-3 text-xs">
                      <span className="flex items-center gap-1" style={{ color: scoreColor(e.mood) }}>
                        <Smile size={13} /> {e.mood}
                      </span>
                      <span className="flex items-center gap-1" style={{ color: scoreColor(e.energy) }}>
                        <BatteryCharging size={13} /> {e.energy}
                      </span>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-[11px] text-faint">{relativeTime(e.created_at)}</span>
                      <button
                        onClick={() => startEdit(e)}
                        data-testid={`journal-edit-${e.id}`}
                        className="text-faint opacity-0 transition-opacity hover:text-ink group-hover:opacity-100"
                        aria-label="Edit reflection"
                      >
                        <Pencil size={13} />
                      </button>
                      <button
                        onClick={() => remove(e)}
                        data-testid={`journal-delete-${e.id}`}
                        className="text-faint opacity-0 transition-opacity hover:text-[#ff8a80] group-hover:opacity-100"
                        aria-label="Delete reflection"
                      >
                        <Trash2 size={13} />
                      </button>
                    </div>
                  </div>
                  {e.notes ? (
                    <p className="text-sm leading-relaxed text-muted">{e.notes}</p>
                  ) : (
                    <p className="text-sm italic text-faint">No notes.</p>
                  )}
                </motion.div>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
