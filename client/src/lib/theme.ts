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

export interface AttrMeta {
  label: string;
  color: string; // hex
  Icon: LucideIcon;
}

// Keep in sync with the server's default attributes.
export const attributeMeta: Record<string, AttrMeta> = {
  strength: { label: "Strength", color: "#ff5f56", Icon: Dumbbell },
  discipline: { label: "Discipline", color: "#6f7dff", Icon: Shield },
  focus: { label: "Focus", color: "#35e0ff", Icon: Target },
  health: { label: "Health", color: "#4bff7e", Icon: Heart },
  wealth: { label: "Wealth", color: "#ffb000", Icon: Coins },
  relationships: { label: "Relationships", color: "#ff6ac1", Icon: Users },
  learning: { label: "Learning", color: "#b98aff", Icon: BookOpen },
  creativity: { label: "Creativity", color: "#ffa23e", Icon: Palette },
  spirituality: { label: "Spirituality", color: "#2ee6c8", Icon: Moon },
};

const fallbackAttr: AttrMeta = { label: "Attribute", color: "#6fae7e", Icon: Target };

export function getAttr(key: string): AttrMeta {
  return attributeMeta[key] ?? { ...fallbackAttr, label: key };
}

export interface TypeMeta {
  label: string;
  color: string;
  Icon: LucideIcon;
}

export const typeMeta: Record<QuestType, TypeMeta> = {
  daily: { label: "Daily", color: "#ffb000", Icon: CalendarCheck },
  weekly: { label: "Weekly", color: "#6f7dff", Icon: CalendarRange },
  main: { label: "Main", color: "#35e0ff", Icon: Flag },
  side: { label: "Side", color: "#6fae7e", Icon: Compass },
  boss: { label: "Boss", color: "#ff4747", Icon: Skull },
  recovery: { label: "Recovery", color: "#2ee6c8", Icon: Leaf },
};

export function getType(type: QuestType): TypeMeta {
  return typeMeta[type] ?? typeMeta.side;
}

export const difficultyMeta: Record<Difficulty, { label: string; pips: number; color: string }> = {
  trivial: { label: "Trivial", pips: 1, color: "#2ee6c8" },
  easy: { label: "Easy", pips: 2, color: "#4bff7e" },
  medium: { label: "Medium", pips: 3, color: "#ffb000" },
  hard: { label: "Hard", pips: 4, color: "#ffa23e" },
  boss: { label: "Boss", pips: 5, color: "#ff4747" },
};

export const ATTRIBUTE_KEYS = Object.keys(attributeMeta);
