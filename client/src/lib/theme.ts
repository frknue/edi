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
  strength: { label: "Strength", color: "#ff5c5c", Icon: Dumbbell },
  discipline: { label: "Discipline", color: "#7c8cff", Icon: Shield },
  focus: { label: "Focus", color: "#2dd4ff", Icon: Target },
  health: { label: "Health", color: "#3ee594", Icon: Heart },
  wealth: { label: "Wealth", color: "#f4b740", Icon: Coins },
  relationships: { label: "Relationships", color: "#ff7eb6", Icon: Users },
  learning: { label: "Learning", color: "#b18bff", Icon: BookOpen },
  creativity: { label: "Creativity", color: "#ff9d4d", Icon: Palette },
  spirituality: { label: "Spirituality", color: "#45e0d0", Icon: Moon },
};

const fallbackAttr: AttrMeta = { label: "Attribute", color: "#8b91a8", Icon: Target };

export function getAttr(key: string): AttrMeta {
  return attributeMeta[key] ?? { ...fallbackAttr, label: key };
}

export interface TypeMeta {
  label: string;
  color: string;
  Icon: LucideIcon;
}

export const typeMeta: Record<QuestType, TypeMeta> = {
  daily: { label: "Daily", color: "#f4b740", Icon: CalendarCheck },
  weekly: { label: "Weekly", color: "#7c8cff", Icon: CalendarRange },
  main: { label: "Main", color: "#2dd4ff", Icon: Flag },
  side: { label: "Side", color: "#8b91a8", Icon: Compass },
  boss: { label: "Boss", color: "#ff3b6b", Icon: Skull },
  recovery: { label: "Recovery", color: "#45e0d0", Icon: Leaf },
};

export function getType(type: QuestType): TypeMeta {
  return typeMeta[type] ?? typeMeta.side;
}

export const difficultyMeta: Record<Difficulty, { label: string; pips: number; color: string }> = {
  trivial: { label: "Trivial", pips: 1, color: "#45e0d0" },
  easy: { label: "Easy", pips: 2, color: "#3ee594" },
  medium: { label: "Medium", pips: 3, color: "#f4b740" },
  hard: { label: "Hard", pips: 4, color: "#ff9d4d" },
  boss: { label: "Boss", pips: 5, color: "#ff3b6b" },
};

export const ATTRIBUTE_KEYS = Object.keys(attributeMeta);
