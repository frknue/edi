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
import { PixelHero } from "../components/PixelHero";
import { ArrowUpRight, Coins, X } from "lucide-react";
import type { Achievement, ItemDrop, LevelUp, XPEvent } from "./types";
import { getAttr, rarityColor } from "./theme";

// A generic reward payload so any XP-awarding action (quest, tool, …) can
// trigger the celebration overlay.
export interface RewardPayload {
  title: string;
  xp_events: XPEvent[];
  level_ups: LevelUp[];
  label?: string; // small overline, e.g. "Quest Complete" / "Tool Complete"
  gold?: number; // gold minted alongside the XP
  crit?: boolean; // critical hit — the payout was doubled
  combo?: number; // combo multiplier this completion paid at (1.0 = none)
  drop?: ItemDrop; // loot, if the dice smiled
  achievements?: Achievement[]; // badges unlocked by this action
  level?: number; // character level, for the hero cameo
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
    timer.current = window.setTimeout(() => setActive(null), payload.achievements?.length || payload.drop ? 5200 : payload.crit ? 4200 : 3200);
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
  const shownXP = useCountUp(totalXP);
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
          {/* radial burst — gold normally, red-hot on a crit */}
          <motion.div
            className="pointer-events-none absolute"
            initial={{ scale: 0.2, opacity: 0.8 }}
            animate={{ scale: result.crit ? 3.2 : 2.4, opacity: 0 }}
            transition={{ duration: result.crit ? 1.4 : 1.1, ease: "easeOut" }}
            style={{
              width: 360,
              height: 360,
              borderRadius: "50%",
              background: result.crit
                ? "radial-gradient(circle, rgba(255,71,71,0.6), rgba(255,71,71,0) 65%)"
                : "radial-gradient(circle, rgba(255,176,0,0.55), rgba(255,176,0,0) 65%)",
            }}
          />
          <SparkBurst crit={result.crit} />
          <motion.div
            className={`hud-panel relative w-full max-w-sm p-7 text-center ${result.crit ? "shake-crit" : ""}`}
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

            <div className="mx-auto mb-3 flex justify-center">
              <PixelHero
                level={result.level ?? 1}
                titled={result.crit === true}
                mood={result.crit ? "crit" : "celebrate"}
                size={60}
              />
            </div>

            {result.crit && (
              <motion.div
                initial={{ scale: 2.2, opacity: 0, rotate: -6 }}
                animate={{ scale: 1, opacity: 1, rotate: -3 }}
                transition={{ type: "spring", stiffness: 400, damping: 12 }}
                className="mx-auto mb-2 inline-block rounded px-3 py-1 font-display text-xl font-bold tracking-widest"
                style={{
                  color: "#ff4747",
                  border: "2px solid #ff4747",
                  boxShadow: "0 0 24px -4px rgba(255,71,71,0.9)",
                  textShadow: "0 0 12px rgba(255,71,71,0.8)",
                }}
                data-testid="crit-banner"
              >
                CRITICAL HIT!
              </motion.div>
            )}
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
              +{shownXP} XP
            </motion.div>

            {typeof result.combo === "number" && result.combo > 1 && (
              <motion.div
                className="mt-1 font-display text-sm tracking-widest"
                style={{ color: "var(--color-spirituality)" }}
                initial={{ opacity: 0, x: -10 }}
                animate={{ opacity: 1, x: 0 }}
                transition={{ delay: 0.15 }}
                data-testid="combo-line"
              >
                🔗 COMBO ×{result.combo}
              </motion.div>
            )}

            {typeof result.gold === "number" && result.gold > 0 && (
              <motion.div
                className="mt-1.5 flex items-center justify-center gap-1.5 text-base font-semibold"
                style={{ color: "var(--color-gold)" }}
                initial={{ opacity: 0, y: 6 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.2 }}
              >
                <Coins size={16} />
                +{result.gold} gold
              </motion.div>
            )}

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

            {result.drop && (
              <motion.div
                className="mt-5 rounded-xl border-2 p-3 text-left"
                style={{
                  borderColor: rarityColor[result.drop.rarity],
                  background: `${rarityColor[result.drop.rarity]}14`,
                  boxShadow: `0 0 24px -6px ${rarityColor[result.drop.rarity]}`,
                }}
                initial={{ opacity: 0, scale: 0.7, rotate: -3 }}
                animate={{ opacity: 1, scale: 1, rotate: 0 }}
                transition={{ type: "spring", stiffness: 300, damping: 16, delay: 0.5 }}
                data-testid="loot-drop"
              >
                <div className="font-display text-[10px] uppercase tracking-[0.25em]" style={{ color: rarityColor[result.drop.rarity] }}>
                  {result.drop.rarity} drop!
                </div>
                <div className="mt-1 flex items-center gap-2">
                  <span className="text-2xl">{result.drop.icon}</span>
                  <div className="min-w-0">
                    <div className="truncate text-sm font-semibold" style={{ color: rarityColor[result.drop.rarity] }}>
                      {result.drop.name}
                    </div>
                    <div className="text-[11px] leading-snug text-muted">{result.drop.flavor}</div>
                  </div>
                </div>
              </motion.div>
            )}

            {(result.achievements?.length ?? 0) > 0 && (
              <motion.div
                className="mt-4 space-y-1.5"
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.6 }}
                data-testid="achievement-unlocks"
              >
                {result.achievements!.map((a) => (
                  <div
                    key={a.key}
                    className="flex items-center justify-center gap-2 rounded-lg border py-1.5 text-sm font-semibold"
                    style={{ borderColor: "var(--color-gold)", background: "rgba(255,176,0,0.1)", color: "var(--color-goldhi)" }}
                  >
                    🏆 {a.icon} {a.name}
                    {a.title && <span className="text-[11px] font-normal opacity-80">· title: {a.title}</span>}
                  </div>
                ))}
              </motion.div>
            )}

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


// useCountUp animates a number from 0 to target (ease-out) — the XP total
// ticks up like an arcade score instead of appearing fully formed.
function useCountUp(target: number, ms = 700): number {
  const [value, setValue] = useState(0);
  useEffect(() => {
    let raf = 0;
    const start = performance.now();
    const tick = (t: number) => {
      const p = Math.min(1, (t - start) / ms);
      setValue(Math.round(target * (1 - Math.pow(1 - p, 3))));
      if (p < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [target, ms]);
  return value;
}

// Particles: a burst of phosphor sparks flying out of the panel.
function SparkBurst({ crit }: { crit?: boolean }) {
  const sparks = Array.from({ length: crit ? 22 : 14 }, (_, i) => {
    const angle = (i / (crit ? 22 : 14)) * Math.PI * 2 + Math.random() * 0.5;
    const dist = 90 + Math.random() * (crit ? 130 : 70);
    return {
      x: Math.cos(angle) * dist,
      y: Math.sin(angle) * dist,
      size: 3 + Math.random() * (crit ? 5 : 3),
      delay: Math.random() * 0.15,
      color: crit && i % 3 === 0 ? "#ff4747" : i % 2 === 0 ? "var(--color-gold)" : "var(--color-phos)",
    };
  });
  return (
    <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
      {sparks.map((s, i) => (
        <motion.span
          key={i}
          className="absolute rounded-full"
          style={{ width: s.size, height: s.size, background: s.color, boxShadow: `0 0 6px ${s.color}` }}
          initial={{ x: 0, y: 0, opacity: 1, scale: 1 }}
          animate={{ x: s.x, y: s.y, opacity: 0, scale: 0.3 }}
          transition={{ duration: 0.9 + Math.random() * 0.4, delay: s.delay, ease: "easeOut" }}
        />
      ))}
    </div>
  );
}
