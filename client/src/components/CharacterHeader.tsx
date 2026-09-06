import { motion } from "framer-motion";
import { PixelHero } from "./PixelHero";
import { Coins, Flame } from "lucide-react";
import type { ActiveDay, CharacterSummary, DailyProgress, Streak } from "../lib/types";
import { ProgressBar } from "./ui";
import { formatNumber, pct } from "../lib/format";
import { useI18n } from "../lib/i18n";

function DailyRing({ ratio, completed, goal }: { ratio: number; completed: number; goal: number }) {
  const r = 26;
  const c = 2 * Math.PI * r;
  const offset = c * (1 - Math.max(0, Math.min(1, ratio)));
  const cleared = completed >= goal;
  return (
    <div className="relative flex h-[68px] w-[68px] items-center justify-center" data-testid="daily-ring">
      <svg width="68" height="68" className="-rotate-90">
        <circle cx="34" cy="34" r={r} fill="none" stroke="rgba(255,255,255,0.08)" strokeWidth="6" />
        <motion.circle
          cx="34"
          cy="34"
          r={r}
          fill="none"
          stroke={cleared ? "var(--color-phos)" : "var(--color-gold)"}
          strokeWidth="6"
          strokeLinecap="round"
          strokeDasharray={c}
          initial={{ strokeDashoffset: c }}
          animate={{ strokeDashoffset: offset }}
          transition={{ duration: 0.9, ease: [0.16, 1, 0.3, 1] }}
          style={{ filter: cleared ? "drop-shadow(0 0 6px rgba(75,255,126,0.8))" : "drop-shadow(0 0 5px rgba(255,176,0,0.7))" }}
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

// ActiveDays: 14 squares, one per local day — "days you showed up". This is
// the headline instead of an unbroken-streak counter: a gap is a gap, not a
// reset, and the strip never shames. Today is outlined; active days glow.
function ActiveDays({ days }: { days: ActiveDay[] }) {
  const { t } = useI18n();
  const active = days.filter((d) => d.active).length;
  return (
    <div data-testid="active-days" title={t("hero.activeDaysTitle")}>
      <div className="flex items-center gap-[5px]">
        {days.map((d) => (
          <span
            key={d.day}
            title={d.day}
            className="block h-3 w-3 rounded-[2px] transition-colors"
            style={{
              background: d.active ? "var(--color-phos)" : "rgba(255,255,255,0.06)",
              boxShadow: d.active ? "0 0 6px rgba(75,255,126,0.6)" : undefined,
              outline: d.today ? "1px solid var(--color-goldhi)" : undefined,
              outlineOffset: 1,
            }}
            data-active={d.active ? "1" : "0"}
          />
        ))}
      </div>
      <div className="mt-1.5 font-display text-[10px] uppercase tracking-wider text-faint">
        {t("hero.activeDays", { n: active, total: days.length })}
      </div>
    </div>
  );
}

export function CharacterHeader({
  character,
  streak,
  daily,
  gold,
  activeDays,
}: {
  character: CharacterSummary;
  streak: Streak;
  daily: DailyProgress;
  gold: number;
  activeDays: ActiveDay[];
}) {
  const { t } = useI18n();
  const today = activeDays.find((d) => d.today)?.day;
  const mendedToday = !!streak.last_mend_date && streak.last_mend_date === today;
  return (
    <motion.section
      initial={{ opacity: 0, y: 14 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.5, ease: [0.16, 1, 0.3, 1] }}
      className="hud-panel clip-corner relative overflow-hidden p-4 sm:p-5"
    >
      <div
        className="pointer-events-none absolute -right-10 -top-16 h-56 w-56 rounded-full"
        style={{ background: "radial-gradient(circle, rgba(255,176,0,0.18), transparent 70%)" }}
      />
      <div className="relative flex flex-col gap-5 lg:flex-row lg:items-center">
        {/* identity: the hero + one level */}
        <div className="flex items-center gap-4">
          <PixelHero level={character.level} titled={!!character.title} size={56} />
          <div
            className="relative grid h-[64px] w-[68px] place-items-center rounded-sm border"
            style={{
              borderColor: "var(--color-gold)",
              boxShadow: "0 0 22px -6px rgba(255,176,0,0.7), inset 0 0 16px rgba(255,176,0,0.10)",
            }}
          >
            <span className="absolute -left-px -top-px h-2.5 w-2.5 border-l-2 border-t-2" style={{ borderColor: "var(--color-goldhi)" }} />
            <span className="absolute -bottom-px -right-px h-2.5 w-2.5 border-b-2 border-r-2" style={{ borderColor: "var(--color-goldhi)" }} />
            <div className="relative text-center leading-none">
              <div className="font-display text-[10px] uppercase tracking-widest text-faint">LV</div>
              <div className="font-display text-3xl" style={{ color: "var(--color-goldhi)" }} data-testid="hero-level">
                {character.level}
              </div>
            </div>
          </div>
          <div className="min-w-0">
            <div className="font-display text-[10px] uppercase tracking-[0.32em] text-muted">{t("hero.operator")}</div>
            <h1 className="cursor-blink truncate font-display text-2xl leading-tight text-ink">{character.name}</h1>
            {character.title && (
              <div className="font-display text-[11px] uppercase tracking-[0.2em]" style={{ color: "var(--color-goldhi)" }} data-testid="character-title">
                {character.title}
              </div>
            )}
            <div className="tabnum mt-0.5 text-[11px] text-faint">{t("hero.totalXp", { xp: formatNumber(character.total_xp) })}</div>
          </div>
        </div>

        {/* xp bar */}
        <div className="flex-1 lg:px-4">
          <div className="mb-1.5 flex items-center justify-between text-xs">
            <span className="font-display uppercase tracking-wider text-muted">{t("hero.progressTo", { level: character.level + 1 })}</span>
            <span className="tabnum text-faint">
              {character.xp_into_level} / {character.xp_for_next_level}
            </span>
          </div>
          <ProgressBar value={character.progress} height={10} />
          <div className="mt-1 text-right text-[11px] text-faint">{pct(character.progress)}%</div>
        </div>

        {/* showed-up strip + today + gold */}
        <div className="flex items-center gap-5 border-t border-edge pt-4 lg:border-l lg:border-t-0 lg:pl-6 lg:pt-0">
          <div>
            <ActiveDays days={activeDays} />
            <div className="mt-1 flex items-center gap-1.5 text-[11px] text-faint" data-testid="streak-small">
              <span className={streak.current > 0 ? "flame-flicker inline-flex" : "inline-flex"}>
                <Flame size={12} style={{ color: streak.current > 0 ? "#ffa23e" : "var(--color-faint)" }} />
              </span>
              <span className="tabnum">{t("hero.streakSmall", { n: streak.current, best: streak.longest })}</span>
              {mendedToday && (
                <span className="rounded px-1 font-display text-[9px] uppercase tracking-wider" style={{ color: "var(--color-phos)", border: "1px solid rgba(75,255,126,0.4)" }} data-testid="streak-mended">
                  {t("hero.mended")}
                </span>
              )}
            </div>
          </div>
          <div className="text-center">
            <DailyRing ratio={daily.ratio} completed={daily.completed_today} goal={daily.goal} />
            <div className="mt-0.5 font-display text-[10px] uppercase tracking-wider text-faint">{t("hero.today")}</div>
            {daily.next_combo_multiplier > 1 && daily.completed_today < daily.goal && (
              <div className="tabnum text-[10px] font-semibold" style={{ color: "var(--color-spirituality)" }} title={t("hero.comboTitle")} data-testid="combo-chip">
                🔗 {t("hero.nextCombo", { mult: daily.next_combo_multiplier })}
              </div>
            )}
          </div>
          <div className="text-center" data-testid="header-gold">
            <div className="flex items-center justify-center gap-1.5">
              <Coins size={18} style={{ color: "var(--color-gold)" }} />
              <span className="tabnum text-xl font-bold text-ink">{gold}</span>
            </div>
            <div className="mt-0.5 font-display text-[10px] uppercase tracking-wider text-faint">{t("hero.gold")}</div>
          </div>
        </div>
      </div>
    </motion.section>
  );
}
