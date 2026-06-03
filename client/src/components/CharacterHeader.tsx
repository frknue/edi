import { motion } from "framer-motion";
import { Flame } from "lucide-react";
import type { CharacterSummary, DailyProgress, Streak } from "../lib/types";
import { ProgressBar } from "./ui";
import { pct } from "../lib/format";

function DailyRing({ ratio, completed, goal }: { ratio: number; completed: number; goal: number }) {
  const r = 26;
  const c = 2 * Math.PI * r;
  const offset = c * (1 - Math.max(0, Math.min(1, ratio)));
  return (
    <div className="relative flex h-[68px] w-[68px] items-center justify-center">
      <svg width="68" height="68" className="-rotate-90">
        <circle cx="34" cy="34" r={r} fill="none" stroke="rgba(255,255,255,0.08)" strokeWidth="6" />
        <motion.circle
          cx="34"
          cy="34"
          r={r}
          fill="none"
          stroke="var(--color-gold)"
          strokeWidth="6"
          strokeLinecap="round"
          strokeDasharray={c}
          initial={{ strokeDashoffset: c }}
          animate={{ strokeDashoffset: offset }}
          transition={{ duration: 0.9, ease: [0.16, 1, 0.3, 1] }}
          style={{ filter: "drop-shadow(0 0 5px rgba(244,183,64,0.7))" }}
        />
      </svg>
      <div className="absolute text-center">
        <div className="tabnum text-sm font-bold text-ink">
          {completed}
          <span className="text-faint">/{goal}</span>
        </div>
      </div>
    </div>
  );
}

export function CharacterHeader({
  character,
  streak,
  daily,
}: {
  character: CharacterSummary;
  streak: Streak;
  daily: DailyProgress;
}) {
  return (
    <motion.section
      initial={{ opacity: 0, y: 14 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.5, ease: [0.16, 1, 0.3, 1] }}
      className="hud-panel clip-corner relative overflow-hidden p-5 sm:p-6"
    >
      <div
        className="pointer-events-none absolute -right-10 -top-16 h-56 w-56 rounded-full"
        style={{ background: "radial-gradient(circle, rgba(244,183,64,0.18), transparent 70%)" }}
      />
      <div className="relative flex flex-col gap-6 lg:flex-row lg:items-center">
        {/* identity */}
        <div className="flex items-center gap-4">
          <div className="relative grid h-[72px] w-[72px] place-items-center">
            <div
              className="absolute inset-0"
              style={{
                background: "linear-gradient(160deg, var(--color-goldhi), var(--color-gold))",
                clipPath: "polygon(50% 0, 100% 25%, 100% 75%, 50% 100%, 0 75%, 0 25%)",
                boxShadow: "0 0 26px -6px rgba(244,183,64,0.8)",
              }}
            />
            <div
              className="absolute inset-[3px]"
              style={{
                background: "var(--color-panel)",
                clipPath: "polygon(50% 0, 100% 25%, 100% 75%, 50% 100%, 0 75%, 0 25%)",
              }}
            />
            <div className="relative text-center leading-none">
              <div className="font-display text-[9px] uppercase tracking-widest text-faint">Lv</div>
              <div className="tabnum text-2xl font-bold" style={{ color: "var(--color-goldhi)" }}>
                {character.level}
              </div>
            </div>
          </div>
          <div>
            <div className="font-display text-[10px] uppercase tracking-[0.32em] text-muted">
              Character
            </div>
            <h1 className="font-display text-2xl font-bold tracking-tight text-ink">
              {character.name}
            </h1>
            <div className="tabnum mt-0.5 text-xs text-faint">
              {character.total_xp.toLocaleString()} total XP
            </div>
          </div>
        </div>

        {/* xp bar */}
        <div className="flex-1 lg:px-4">
          <div className="mb-1.5 flex items-center justify-between text-xs">
            <span className="font-display uppercase tracking-wider text-muted">
              Progress to Lv {character.level + 1}
            </span>
            <span className="tabnum text-faint">
              {character.xp_into_level} / {character.xp_for_next_level}
            </span>
          </div>
          <ProgressBar value={character.progress} height={10} />
          <div className="mt-1 text-right text-[11px] text-faint">{pct(character.progress)}%</div>
        </div>

        {/* streak + daily */}
        <div className="flex items-center gap-5 border-t border-edge pt-4 lg:border-l lg:border-t-0 lg:pl-6 lg:pt-0">
          <div className="text-center">
            <div className="flex items-center justify-center gap-1.5">
              <Flame size={20} style={{ color: streak.current > 0 ? "#ff9d4d" : "var(--color-faint)" }} />
              <span className="tabnum text-2xl font-bold text-ink">{streak.current}</span>
            </div>
            <div className="mt-0.5 font-display text-[10px] uppercase tracking-wider text-faint">
              Day streak
            </div>
            <div className="text-[10px] text-faint">best {streak.longest}</div>
          </div>
          <div className="text-center">
            <DailyRing ratio={daily.ratio} completed={daily.completed_today} goal={daily.goal} />
            <div className="mt-0.5 font-display text-[10px] uppercase tracking-wider text-faint">
              Today
            </div>
          </div>
        </div>
      </div>
    </motion.section>
  );
}
