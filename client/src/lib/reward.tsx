import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { AnimatePresence, motion } from "framer-motion";
import { ArrowUpRight, Sparkles, X } from "lucide-react";
import type { LevelUp, XPEvent } from "./types";
import { getAttr } from "./theme";

// A generic reward payload so any XP-awarding action (quest, tool, …) can
// trigger the celebration overlay.
export interface RewardPayload {
  title: string;
  xp_events: XPEvent[];
  level_ups: LevelUp[];
  label?: string; // small overline, e.g. "Quest Complete" / "Tool Complete"
}

interface RewardContextValue {
  celebrate: (payload: RewardPayload) => void;
}

const RewardContext = createContext<RewardContextValue>({ celebrate: () => {} });

export function useReward(): RewardContextValue {
  return useContext(RewardContext);
}

export function RewardProvider({ children }: { children: ReactNode }) {
  const [active, setActive] = useState<RewardPayload | null>(null);
  const timer = useRef<number | undefined>(undefined);

  const celebrate = useCallback((payload: RewardPayload) => {
    window.clearTimeout(timer.current);
    setActive(payload);
    timer.current = window.setTimeout(() => setActive(null), 3200);
  }, []);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  return (
    <RewardContext.Provider value={{ celebrate }}>
      {children}
      <RewardOverlay result={active} onClose={() => setActive(null)} />
    </RewardContext.Provider>
  );
}

function RewardOverlay({
  result,
  onClose,
}: {
  result: RewardPayload | null;
  onClose: () => void;
}) {
  const totalXP = result?.xp_events.reduce((sum, e) => sum + e.amount, 0) ?? 0;
  return (
    <AnimatePresence>
      {result && (
        <motion.div
          key="reward"
          className="fixed inset-0 z-50 flex items-center justify-center p-6"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          onClick={onClose}
          style={{ background: "rgba(4,5,9,0.72)", backdropFilter: "blur(6px)" }}
          data-testid="reward-overlay"
        >
          {/* radial burst */}
          <motion.div
            className="pointer-events-none absolute"
            initial={{ scale: 0.2, opacity: 0.8 }}
            animate={{ scale: 2.4, opacity: 0 }}
            transition={{ duration: 1.1, ease: "easeOut" }}
            style={{
              width: 360,
              height: 360,
              borderRadius: "50%",
              background:
                "radial-gradient(circle, rgba(255,176,0,0.55), rgba(255,176,0,0) 65%)",
            }}
          />
          <motion.div
            className="hud-panel relative w-full max-w-sm overflow-hidden p-7 text-center"
            initial={{ scale: 0.85, y: 24, opacity: 0 }}
            animate={{ scale: 1, y: 0, opacity: 1 }}
            exit={{ scale: 0.9, y: 10, opacity: 0 }}
            transition={{ type: "spring", stiffness: 320, damping: 24 }}
            onClick={(e) => e.stopPropagation()}
          >
            <button
              onClick={onClose}
              className="absolute right-3 top-3 text-faint transition-colors hover:text-ink"
              aria-label="Close"
            >
              <X size={18} />
            </button>

            <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-sm border"
              style={{ borderColor: "var(--color-gold)", color: "var(--color-gold)", boxShadow: "0 0 16px -4px rgba(255,176,0,0.8)" }}>
              <Sparkles size={22} />
            </div>

            <div className="font-display text-sm uppercase tracking-[0.3em] text-muted">
              *** {result.label ?? "Quest Complete"} ***
            </div>
            <div className="mt-1 truncate px-2 text-lg font-semibold text-ink">
              {result.title}
            </div>

            <motion.div
              className="glow mt-4 font-display text-6xl"
              style={{ color: "var(--color-goldhi)" }}
              initial={{ scale: 0.6 }}
              animate={{ scale: 1 }}
              transition={{ type: "spring", stiffness: 360, damping: 14, delay: 0.05 }}
            >
              +{totalXP} XP
            </motion.div>

            <div className="mt-5 flex flex-wrap justify-center gap-2">
              {result.xp_events.map((e, i) => {
                const meta = getAttr(e.attribute_key);
                const Icon = meta.Icon;
                return (
                  <motion.div
                    key={e.id ?? i}
                    initial={{ opacity: 0, y: 8 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ delay: 0.15 + i * 0.07 }}
                    className="flex items-center gap-1.5 rounded-full px-2.5 py-1 text-sm"
                    style={{ background: `${meta.color}1f`, color: meta.color }}
                  >
                    <Icon size={13} />
                    <span className="tabnum font-medium">+{e.amount}</span>
                    <span className="text-xs opacity-80">{meta.label}</span>
                  </motion.div>
                );
              })}
            </div>

            {result.level_ups.length > 0 && (
              <motion.div
                className="mt-5 space-y-1.5"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ delay: 0.4 }}
              >
                {result.level_ups.map((lu) => {
                  const meta = getAttr(lu.attribute_key);
                  return (
                    <div
                      key={lu.attribute_key}
                      className="flex items-center justify-center gap-2 rounded-lg py-1.5 text-sm font-semibold"
                      style={{ background: `${meta.color}1a`, color: meta.color }}
                    >
                      <ArrowUpRight size={15} />
                      {meta.label} reached Lv {lu.to_level}
                    </div>
                  );
                })}
              </motion.div>
            )}
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
