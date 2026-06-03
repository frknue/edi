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

export function Btn({
  variant = "ghost",
  className = "",
  children,
  ...rest
}: { variant?: BtnVariant; children: ReactNode } & ButtonHTMLAttributes<HTMLButtonElement>) {
  const base =
    "inline-flex items-center justify-center gap-2 rounded-lg px-3.5 py-2 text-sm font-medium transition-all duration-150 disabled:cursor-not-allowed disabled:opacity-50 focus:outline-none";
  const variants: Record<BtnVariant, string> = {
    primary:
      "text-[#1a1305] shadow-[0_8px_24px_-10px_rgba(244,183,64,0.8)] hover:brightness-110 active:scale-[0.98]",
    ghost:
      "border border-edge bg-white/[0.02] text-ink hover:border-edge2 hover:bg-white/[0.05] active:scale-[0.98]",
    danger:
      "border border-transparent bg-[#ff3b6b]/10 text-[#ff7d9d] hover:bg-[#ff3b6b]/20 active:scale-[0.98]",
    soft: "bg-white/[0.04] text-muted hover:text-ink hover:bg-white/[0.07] active:scale-[0.98]",
  };
  const style =
    variant === "primary"
      ? { background: "linear-gradient(180deg, var(--color-goldhi), var(--color-gold))" }
      : undefined;
  return (
    <button className={`${base} ${variants[variant]} ${className}`} style={style} {...rest}>
      {children}
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
        <h2 className="font-display text-sm font-semibold uppercase tracking-[0.18em] text-ink">
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
