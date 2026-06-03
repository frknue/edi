import { useState } from "react";
import { motion } from "framer-motion";
import { BatteryCharging, BookHeart, Smile } from "lucide-react";
import { useJournal, useCreateJournal } from "../lib/queries";
import { Btn, EmptyState, SectionTitle, Spinner } from "../components/ui";
import { relativeTime } from "../lib/format";

function scoreColor(v: number): string {
  if (v <= 3) return "#ff5c5c";
  if (v <= 6) return "#f4b740";
  return "#3ee594";
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

export function JournalPage() {
  const { data: entries, isLoading } = useJournal();
  const createJournal = useCreateJournal();
  const [mood, setMood] = useState(6);
  const [energy, setEnergy] = useState(6);
  const [notes, setNotes] = useState("");
  const [saved, setSaved] = useState(false);

  const submit = () => {
    createJournal.mutate(
      { mood, energy, notes: notes.trim() },
      {
        onSuccess: () => {
          setNotes("");
          setSaved(true);
          window.setTimeout(() => setSaved(false), 2000);
        },
      },
    );
  };

  return (
    <div className="space-y-5">
      <div>
        <h1 className="font-display text-xl font-bold tracking-tight text-ink">Reflection</h1>
        <p className="text-sm text-faint">Log how today felt. Patterns emerge over time.</p>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Composer */}
        <section className="hud-panel clip-corner space-y-5 p-5">
          <SectionTitle hint="Rate 1-10 and jot a note.">Today's check-in</SectionTitle>
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
            <Btn variant="primary" disabled={createJournal.isPending} onClick={submit} data-testid="save-journal">
              <BookHeart size={15} /> Save reflection
            </Btn>
          </div>
        </section>

        {/* History */}
        <section>
          <SectionTitle hint="Most recent first.">Recent reflections</SectionTitle>
          {isLoading ? (
            <Spinner />
          ) : !entries || entries.length === 0 ? (
            <EmptyState icon={<BookHeart size={20} />} title="No reflections yet" hint="Your first entry will appear here." />
          ) : (
            <div className="space-y-2.5">
              {entries.map((e, i) => (
                <motion.div
                  key={e.id}
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: Math.min(i * 0.04, 0.3) }}
                  className="hud-panel p-3.5"
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
                    <span className="text-[11px] text-faint">{relativeTime(e.created_at)}</span>
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
