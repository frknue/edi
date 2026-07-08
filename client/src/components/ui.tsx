import type { ButtonHTMLAttributes, ReactNode } from "react";
import { getAttr, getType, difficultyMeta } from "../lib/theme";
import type { Difficulty, QuestType } from "../lib/types";
import { pct } from "../lib/format";

// --- progress bar -----------------------------------------------------------

export function ProgressBar({
  value,
  color = "var(--color-gold)",
  height = 8,
  shimmer = true,
}: {
  value: number; // 0..1
  color?: string;
  height?: number;
  shimmer?: boolean;
}) {
  const width = pct(value);
  return (
    <div className="xp-track" style={{ height }}>
      <div
        className={shimmer ? "xp-fill" : ""}
        style={{
          width: `${width}%`,
          height: "100%",
          borderRadius: 999,
          background: `linear-gradient(90deg, ${color}cc, ${color})`,
          boxShadow: `0 0 12px -2px ${color}`,
          transition: "width 0.7s cubic-bezier(0.16,1,0.3,1)",
        }}
      />
    </div>
  );
}

// --- buttons ----------------------------------------------------------------

type BtnVariant = "primary" | "ghost" | "danger" | "soft";

// Terminal buttons: bracketed commands. Primary is amber phosphor that inverts
// to "inverse video" on hover, like selecting a menu item on a CRT.
export function Btn({
  variant = "ghost",
  className = "",
  children,
  ...rest
}: { variant?: BtnVariant; children: ReactNode } & ButtonHTMLAttributes<HTMLButtonElement>) {
  const base =
    "btn-term inline-flex items-center justify-center gap-2 rounded-sm px-3.5 py-2 text-[13px] font-medium uppercase tracking-wide transition-all duration-100 disabled:cursor-not-allowed disabled:opacity-50 focus:outline-none";
  const variants: Record<BtnVariant, string> = {
    primary:
      "border border-[var(--color-gold)] bg-[rgba(255,176,0,0.08)] text-[var(--color-goldhi)] shadow-[0_0_14px_-6px_rgba(255,176,0,0.8)] hover:bg-[var(--color-gold)] hover:text-[#1a1200] hover:shadow-[0_0_18px_-2px_rgba(255,176,0,0.9)] active:scale-[0.99]",
    ghost:
      "border border-edge bg-transparent text-ink hover:border-edge2 hover:bg-[rgba(75,255,126,0.06)] active:scale-[0.99]",
    danger:
      "border border-[#ff4747]/50 bg-[#ff4747]/08 text-[#ff8a80] hover:bg-[#ff4747] hover:text-[#1a0000] active:scale-[0.99]",
    soft: "border border-transparent bg-white/[0.04] text-muted hover:text-ink hover:bg-white/[0.07] active:scale-[0.99]",
  };
  // Brackets mark primary "commands"; compact ghost/soft/icon buttons stay bare.
  const bracketed = variant === "primary";
  return (
    <button className={`${base} ${variants[variant]} ${className}`} {...rest}>
      {bracketed && <span aria-hidden className="opacity-60">[</span>}
      {children}
      {bracketed && <span aria-hidden className="opacity-60">]</span>}
    </button>
  );
}

// --- badges -----------------------------------------------------------------

export function TypeBadge({ type }: { type: QuestType }) {
  const meta = getType(type);
  const Icon = meta.Icon;
  return (
    <span
      className="inline-flex items-center gap-1 rounded-md px-2 py-0.5 font-display text-[10px] font-semibold uppercase tracking-wider"
      style={{ background: `${meta.color}1a`, color: meta.color }}
    >
      <Icon size={11} />
      {meta.label}
    </span>
  );
}

export function DifficultyPips({ difficulty }: { difficulty: Difficulty }) {
  const meta = difficultyMeta[difficulty];
  return (
    <span className="inline-flex items-center gap-1.5" title={`Difficulty: ${meta.label}`}>
      <span className="flex gap-0.5">
        {Array.from({ length: 5 }).map((_, i) => (
          <span
            key={i}
            className="h-1.5 w-1.5 rounded-full"
            style={{ background: i < meta.pips ? meta.color : "rgba(255,255,255,0.12)" }}
          />
        ))}
      </span>
      <span className="text-[11px] text-muted">{meta.label}</span>
    </span>
  );
}

export function RewardChips({ rewards }: { rewards: Record<string, number> }) {
  const entries = Object.entries(rewards ?? {}).filter(([, v]) => v > 0);
  if (entries.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-1.5">
      {entries.map(([key, amount]) => {
        const meta = getAttr(key);
        const Icon = meta.Icon;
        return (
          <span
            key={key}
            className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs"
            style={{ background: `${meta.color}14`, color: meta.color }}
          >
            <Icon size={12} />
            <span className="tabnum font-medium">+{amount}</span>
          </span>
        );
      })}
    </div>
  );
}

// --- layout helpers ---------------------------------------------------------

export function SectionTitle({
  children,
  hint,
  action,
}: {
  children: ReactNode;
  hint?: string;
  action?: ReactNode;
}) {
  return (
    <div className="mb-3 flex items-end justify-between gap-3">
      <div>
        <h2 className="font-display text-lg uppercase tracking-[0.14em] text-ink">
          <span aria-hidden className="mr-1.5 text-[var(--color-phos)]">▸</span>
          {children}
        </h2>
        {hint && <p className="mt-0.5 text-xs text-faint">{hint}</p>}
      </div>
      {action}
    </div>
  );
}

export function EmptyState({ icon, title, hint }: { icon?: ReactNode; title: string; hint?: string }) {
  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-edge px-6 py-10 text-center">
      {icon && <div className="mb-2 text-faint">{icon}</div>}
      <p className="text-sm font-medium text-muted">{title}</p>
      {hint && <p className="mt-1 text-xs text-faint">{hint}</p>}
    </div>
  );
}

export function Spinner({ label }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-3 py-16 text-muted">
      <span className="h-4 w-4 animate-spin rounded-full border-2 border-edge2 border-t-[var(--color-gold)]" />
      {label && <span className="text-sm">{label}</span>}
    </div>
  );
}
