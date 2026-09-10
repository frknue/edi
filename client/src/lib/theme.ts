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
import { palette, type Palette as ThemePalette } from "./themes";
import type { MessageKey } from "./locales/en";

export interface AttrMeta {
  label: string; // localized display name (resolved at call time)
  color: string; // 6-digit hex (callers append alpha bytes), per active theme
  Icon: LucideIcon;
}

// Keep in sync with the server's default attributes. Labels are looked up
// through i18n and colors through the active theme's palette on every access
// (getters), so a language or theme switch repaints them on remount.
const attrIcons: Record<string, LucideIcon> = {
  strength: Dumbbell,
  discipline: Shield,
  focus: Target,
  health: Heart,
  wealth: Coins,
  relationships: Users,
  learning: BookOpen,
  creativity: Palette,
  spirituality: Moon,
};

type PaletteColor = Exclude<keyof ThemePalette, "attrs">;

function withLabel<T extends object>(key: MessageKey, meta: T): T & { readonly label: string } {
  return Object.defineProperty({ ...meta }, "label", { get: () => t(key), enumerable: true }) as T & {
    readonly label: string;
  };
}

// Resolve `color` from the active theme's palette at access time. `pick` is
// either an attribute key or one of the semantic accent slots.
function withColor<T extends object>(pick: (p: ThemePalette) => string, meta: T): T & { readonly color: string } {
  return Object.defineProperty(meta, "color", { get: () => pick(palette()), enumerable: true }) as T & {
    readonly color: string;
  };
}

const themed = <T extends object>(key: MessageKey, pick: PaletteColor | ((p: ThemePalette) => string), meta: T) =>
  withColor(typeof pick === "string" ? (p) => p[pick] : pick, withLabel(key, meta));

export const attributeMeta: Record<string, AttrMeta> = Object.fromEntries(
  Object.entries(attrIcons).map(([key, Icon]) => [
    key,
    themed(`attr.${key}` as MessageKey, (p) => p.attrs[key] ?? p.fallback, { Icon }),
  ]),
);

const fallbackAttr: AttrMeta = themed("attr.fallback", "fallback", { Icon: Target });

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
  daily: themed("type.daily", "gold", { Icon: CalendarCheck }),
  weekly: themed("type.weekly", (p) => p.attrs.discipline, { Icon: CalendarRange }),
  main: themed("type.main", "focus", { Icon: Flag }),
  side: themed("type.side", "fallback", { Icon: Compass }),
  boss: themed("type.boss", "boss", { Icon: Skull }),
  recovery: themed("type.recovery", "teal", { Icon: Leaf }),
};

export function getType(type: QuestType): TypeMeta {
  return typeMeta[type] ?? typeMeta.side;
}

export const difficultyMeta: Record<Difficulty, { label: string; pips: number; color: string }> = {
  trivial: themed("difficulty.trivial", "teal", { pips: 1 }),
  easy: themed("difficulty.easy", "phos", { pips: 2 }),
  medium: themed("difficulty.medium", "gold", { pips: 3 }),
  hard: themed("difficulty.hard", "peach", { pips: 4 }),
  boss: themed("difficulty.boss", "boss", { pips: 5 }),
};

export const ATTRIBUTE_KEYS = Object.keys(attributeMeta);

// Loot rarity palette (the classic RPG ramp — instantly legible). Common stays
// grey in every theme; the rest follow the active palette (getters).
export const rarityColor: Record<string, string> = withColors({
  common: () => "#9aa4a6",
  uncommon: (p) => p.phos,
  rare: (p) => p.focus,
  epic: (p) => p.epic,
  legendary: (p) => p.gold,
});

function withColors(picks: Record<string, (p: ThemePalette) => string>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, pick] of Object.entries(picks)) {
    Object.defineProperty(out, k, { get: () => pick(palette()), enumerable: true });
  }
  return out;
}
