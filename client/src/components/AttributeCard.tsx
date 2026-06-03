import { motion } from "framer-motion";
import type { Attribute } from "../lib/types";
import { getAttr } from "../lib/theme";
import { ProgressBar } from "./ui";

export function AttributeCard({ attribute, index = 0 }: { attribute: Attribute; index?: number }) {
  const meta = getAttr(attribute.key);
  const Icon = meta.Icon;
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, delay: index * 0.04, ease: [0.16, 1, 0.3, 1] }}
      className="hud-panel hud-panel-hover group relative overflow-hidden p-4"
    >
      <div
        className="pointer-events-none absolute -right-6 -top-6 h-20 w-20 rounded-full opacity-50 transition-opacity group-hover:opacity-90"
        style={{ background: `radial-gradient(circle, ${meta.color}33, transparent 70%)` }}
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
        <div
          className="flex h-8 min-w-8 items-center justify-center rounded-md px-2 font-display text-sm font-bold"
          style={{ background: `${meta.color}14`, color: meta.color }}
        >
          {attribute.level}
        </div>
      </div>

      <div className="relative mt-3.5">
        <ProgressBar value={attribute.progress} color={meta.color} height={6} shimmer={false} />
        <div className="tabnum mt-1.5 flex justify-between text-[10px] text-faint">
          <span>Lv {attribute.level}</span>
          <span>
            {attribute.xp_into_level}/{attribute.xp_for_next_level}
          </span>
        </div>
      </div>
    </motion.div>
  );
}
