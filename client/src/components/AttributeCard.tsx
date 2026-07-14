import { motion } from "framer-motion";
import { Shield, TrendingDown } from "lucide-react";
import type { Attribute } from "../lib/types";
import { getAttr } from "../lib/theme";
import { useWardAttribute } from "../lib/queries";
import { pushToast } from "../lib/toast";
import { ProgressBar } from "./ui";

const RUST = "#ff6a3d";
const WARD_COST = 30;

function DecayBadge({ attribute }: { attribute: Attribute }) {
  const d = attribute.decay;
  if (!d || d.state === "fresh" || d.state === "rest") return null;
  if (d.state === "warded") {
    return (
      <div className="mt-2 flex items-center gap-1.5 text-[10px] uppercase tracking-wider" style={{ color: "var(--color-gold)" }}>
        <Shield size={11} />
        warded until {d.warded_until ? new Date(d.warded_until).toLocaleDateString() : "?"}
      </div>
    );
  }
  if (d.state === "grace") {
    return (
      <div className="mt-2 text-[10px] uppercase tracking-wider text-faint">
        idle {d.idle_days}d — decays in {4 - d.idle_days}d
      </div>
    );
  }
  return (
    <div className="mt-2 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider" style={{ color: RUST }}>
      <TrendingDown size={11} />
      rusting · {d.idle_days}d idle · -{d.projected_daily_loss} XP/day
    </div>
  );
}

export function AttributeCard({
  attribute,
  index = 0,
  goldBalance,
}: {
  attribute: Attribute;
  index?: number;
  goldBalance?: number;
}) {
  const meta = getAttr(attribute.key);
  const Icon = meta.Icon;
  const ward = useWardAttribute();
  const decaying = attribute.decay?.state === "decaying";
  const canWard =
    typeof goldBalance === "number" &&
    goldBalance >= WARD_COST &&
    (attribute.decay?.state === "grace" || attribute.decay?.state === "decaying");

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, delay: index * 0.04, ease: [0.16, 1, 0.3, 1] }}
      className="hud-panel hud-panel-hover group relative overflow-hidden p-4"
      style={decaying ? { borderColor: `${RUST}55`, filter: "saturate(0.75)" } : undefined}
    >
      <div
        className="pointer-events-none absolute -right-6 -top-6 h-20 w-20 rounded-full opacity-50 transition-opacity group-hover:opacity-90"
        style={{ background: `radial-gradient(circle, ${decaying ? RUST : meta.color}33, transparent 70%)` }}
      />
      <div className="relative flex items-center justify-between">
        <div className="flex items-center gap-2.5">
          <div
            className="grid h-9 w-9 place-items-center rounded-lg"
            style={{ background: `${meta.color}1a`, color: meta.color }}
          >
            <Icon size={18} />
          </div>
          <div>
            <div className="text-sm font-semibold text-ink">{meta.label}</div>
            <div className="tabnum text-[11px] text-faint">{attribute.total_xp.toLocaleString()} XP</div>
          </div>
        </div>
        <div className="flex items-center gap-1.5">
          {canWard && (
            <button
              onClick={() =>
                ward.mutate(attribute.key, {
                  onSuccess: (res) =>
                    pushToast(`${meta.label} warded for 7 days (-${WARD_COST}g, ${res.balance}g left).`, "success"),
                })
              }
              disabled={ward.isPending}
              title={`Ward for 7 days (${WARD_COST}g)`}
              aria-label={`Ward ${meta.label}`}
              className="grid h-8 w-8 place-items-center rounded-md transition-colors hover:bg-white/[0.06]"
              style={{ color: "var(--color-gold)" }}
              data-testid={`ward-${attribute.key}`}
            >
              <Shield size={15} />
            </button>
          )}
          <div
            className="flex h-8 min-w-8 items-center justify-center rounded-md px-2 font-display text-sm font-bold"
            style={{ background: `${meta.color}14`, color: meta.color }}
          >
            {attribute.level}
          </div>
        </div>
      </div>

      <div className="relative mt-3.5">
        <ProgressBar value={attribute.progress} color={decaying ? RUST : meta.color} height={6} shimmer={false} />
        <div className="tabnum mt-1.5 flex justify-between text-[10px] text-faint">
          <span>Lv {attribute.level}</span>
          <span>
            {attribute.xp_into_level}/{attribute.xp_for_next_level}
          </span>
        </div>
        <DecayBadge attribute={attribute} />
      </div>
    </motion.div>
  );
}
