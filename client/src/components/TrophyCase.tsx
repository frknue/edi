import { useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { Trophy, X } from "lucide-react";
import { useAchievements } from "../lib/queries";

// TrophyCase: a compact badge row on the dashboard (earned count + icons)
// that opens the full hall as a modal grid. Hidden achievements only appear
// once earned — permanent open loops, cheap to glance at.
export function TrophyCase() {
  const { data: hall } = useAchievements();
  const [open, setOpen] = useState(false);
  if (!hall || hall.length === 0) return null;
  const earned = hall.filter((a) => a.earned);

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        className="hud-panel hud-panel-hover flex w-full items-center justify-between gap-3 p-3 text-left"
        data-testid="trophy-case"
      >
        <div className="flex min-w-0 items-center gap-2.5">
          <Trophy size={16} style={{ color: "var(--color-gold)" }} />
          <span className="font-display text-[11px] uppercase tracking-[0.18em] text-muted">
            Trophies {earned.length}/{hall.length}
          </span>
          <span className="truncate text-base">
            {earned.slice(-8).map((a) => a.icon).join(" ")}
          </span>
        </div>
        <span className="shrink-0 text-[11px] text-faint">view hall →</span>
      </button>

      <AnimatePresence>
        {open && (
          <motion.div
            className="sheet-safe fixed inset-0 z-50 flex items-end justify-center p-0 sm:items-center sm:p-6"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            style={{ background: "rgba(4,5,9,0.75)", backdropFilter: "blur(4px)" }}
            onClick={() => setOpen(false)}
          >
            <motion.div
              className="hud-panel w-full max-w-lg overflow-hidden"
              initial={{ y: 24, opacity: 0 }}
              animate={{ y: 0, opacity: 1 }}
              exit={{ y: 16, opacity: 0 }}
              onClick={(e) => e.stopPropagation()}
            >
              <div className="flex items-center justify-between border-b border-edge px-5 py-3.5">
                <h2 className="font-display text-sm font-semibold uppercase tracking-[0.18em] text-ink">
                  Trophy Hall
                </h2>
                <button onClick={() => setOpen(false)} className="text-faint hover:text-ink" aria-label="Close">
                  <X size={18} />
                </button>
              </div>
              <div className="grid max-h-[70vh] grid-cols-1 gap-2 overflow-y-auto p-4 sm:grid-cols-2">
                {hall.map((a) => (
                  <div
                    key={a.key}
                    className="rounded-lg border p-3"
                    style={{
                      borderColor: a.earned ? "var(--color-gold)" : "var(--color-edge)",
                      background: a.earned ? "rgba(255,176,0,0.06)" : "transparent",
                      opacity: a.earned ? 1 : 0.55,
                    }}
                  >
                    <div className="flex items-center gap-2">
                      <span className="text-xl" style={a.earned ? undefined : { filter: "grayscale(1)" }}>
                        {a.icon}
                      </span>
                      <div className="min-w-0">
                        <div className="truncate text-sm font-semibold text-ink">{a.name}</div>
                        <div className="text-[11px] leading-snug text-muted">{a.desc}</div>
                        {a.title && (
                          <div className="text-[10px]" style={{ color: "var(--color-goldhi)" }}>
                            grants title: {a.title}
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </>
  );
}
