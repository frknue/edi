import { motion } from "framer-motion";
import type { XPEvent } from "../lib/types";
import { getAttr } from "../lib/theme";
import { relativeTime } from "../lib/format";
import { EmptyState } from "./ui";
import { Activity } from "lucide-react";

export function XPFeed({ events }: { events: XPEvent[] }) {
  if (events.length === 0) {
    return <EmptyState icon={<Activity size={20} />} title="No XP yet" hint="Complete a quest to start earning." />;
  }
  return (
    <ul className="space-y-1.5">
      {events.map((e, i) => {
        const meta = getAttr(e.attribute_key);
        const Icon = meta.Icon;
        return (
          <motion.li
            key={e.id}
            initial={{ opacity: 0, x: -8 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ delay: Math.min(i * 0.03, 0.3) }}
            className="flex items-center gap-3 rounded-lg border border-transparent px-2 py-1.5 transition-colors hover:border-edge hover:bg-white/[0.02]"
          >
            <div
              className="grid h-7 w-7 shrink-0 place-items-center rounded-md"
              style={{ background: `${meta.color}1a`, color: meta.color }}
            >
              <Icon size={14} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="truncate text-[13px] text-ink">
                {e.note || (e.source === "seed" ? "Starting progress" : meta.label)}
              </div>
              <div className="text-[10px] uppercase tracking-wide text-faint">
                {meta.label}
                {e.source === "seed" && " · seed"}
                {e.source === "decay" && " · decay"}
              </div>
            </div>
            <div className="text-right">
              <div className="tabnum text-sm font-semibold" style={{ color: e.amount < 0 ? "#ff6a3d" : meta.color }}>
                {e.amount < 0 ? e.amount : `+${e.amount}`}
              </div>
              <div className="text-[10px] text-faint">{relativeTime(e.created_at)}</div>
            </div>
          </motion.li>
        );
      })}
    </ul>
  );
}
