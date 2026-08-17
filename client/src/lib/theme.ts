import {
  BookOpen,
  CalendarCheck,
  CalendarRange,
  Coins,
  Compass,
  Dumbbell,
  Flag,
  Heart,
  Leaf,
  Moon,
  Palette,
  Shield,
  Skull,
  Target,
  Users,
  type LucideIcon,
} from "lucide-react";
import type { Difficulty, QuestType } from "./types";
import { t } from "./i18n";
import type { MessageKey } from "./locales/en";

export interface AttrMeta {
  label: string; // localized display name (resolved at call time)
  color: string; // hex
  Icon: LucideIcon;
}

// Keep in sync with the server's default attributes. Labels are looked up
// through i18n on every access (getters) so a language switch repaints them.
const attrStatic: Record<string, { color: string; Icon: LucideIcon }> = {
  strength: { color: "#ff5f56", Icon: Dumbbell },
  discipline: { color: "#6f7dff", Icon: Shield },
  focus: { color: "#35e0ff", Icon: Target },
  health: { color: "#4bff7e", Icon: Heart },
  wealth: { color: "#ffb000", Icon: Coins },
  relationships: { color: "#ff6ac1", Icon: Users },
  learning: { color: "#b98aff", Icon: BookOpen },
  creativity: { color: "#ffa23e", Icon: Palette },
  spirituality: { color: "#2ee6c8", Icon: Moon },
};

function withLabel<T extends object>(key: MessageKey, meta: T): T & { readonly label: string } {
  return Object.defineProperty({ ...meta }, "label", { get: () => t(key), enumerable: true }) as T & {
    readonly label: string;
  };
}

export const attributeMeta: Record<string, AttrMeta> = Object.fromEntries(
  Object.entries(attrStatic).map(([key, meta]) => [key, withLabel(`attr.${key}` as MessageKey, meta)]),
);

const fallbackAttr: AttrMeta = withLabel("attr.fallback", { color: "#6fae7e", Icon: Target });

export function getAttr(key: string): AttrMeta {
  // Unknown keys (custom attributes) show their raw key as the label.
  return attributeMeta[key] ?? { color: fallbackAttr.color, Icon: fallbackAttr.Icon, label: key };
}

export interface TypeMeta {
  label: string;
  color: string;
  Icon: LucideIcon;
}

export const typeMeta: Record<QuestType, TypeMeta> = {
  daily: withLabel("type.daily", { color: "#ffb000", Icon: CalendarCheck }),
  weekly: withLabel("type.weekly", { color: "#6f7dff", Icon: CalendarRange }),
  main: withLabel("type.main", { color: "#35e0ff", Icon: Flag }),
  side: withLabel("type.side", { color: "#6fae7e", Icon: Compass }),
  boss: withLabel("type.boss", { color: "#ff4747", Icon: Skull }),
  recovery: withLabel("type.recovery", { color: "#2ee6c8", Icon: Leaf }),
};

export function getType(type: QuestType): TypeMeta {
  return typeMeta[type] ?? typeMeta.side;
}

export const difficultyMeta: Record<Difficulty, { label: string; pips: number; color: string }> = {
  trivial: withLabel("difficulty.trivial", { pips: 1, color: "#2ee6c8" }),
  easy: withLabel("difficulty.easy", { pips: 2, color: "#4bff7e" }),
  medium: withLabel("difficulty.medium", { pips: 3, color: "#ffb000" }),
  hard: withLabel("difficulty.hard", { pips: 4, color: "#ffa23e" }),
  boss: withLabel("difficulty.boss", { pips: 5, color: "#ff4747" }),
};

export const ATTRIBUTE_KEYS = Object.keys(attributeMeta);

// Loot rarity palette (the classic RPG ramp — instantly legible).
export const rarityColor: Record<string, string> = {
  common: "#9aa4a6",
  uncommon: "#4bff7e",
  rare: "#34d0ff",
  epic: "#b98aff",
  legendary: "#ffb000",
};
