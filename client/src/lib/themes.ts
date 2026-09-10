import { createContext, createElement, useContext, useEffect, useState, type ReactNode } from "react";
import type { MessageKey } from "./locales/en";

// Visual themes. Every theme is a set of CSS custom-property overrides in
// `index.css` (`:root[data-theme="…"]`) plus a matching hex palette here for
// the places that concatenate alpha onto a hex color (`${color}1a`), which a
// `var(--…)` reference cannot do. The active theme is a per-device preference
// (localStorage) and is applied as `data-theme` on <html> — the inline script
// in `index.html` does the same before the stylesheet loads so a saved theme
// never flashes CRT green first. Like the locale, a switch remounts the tree
// (`main.tsx`) so getter-based colors in `theme.ts` repaint at once.

export type Theme = "crt" | "slate" | "blossom";
export const THEMES: Theme[] = ["crt", "slate", "blossom"];
export const DEFAULT_THEME: Theme = "crt";
export const themeLabelKey: Record<Theme, MessageKey> = {
  crt: "theme.crt",
  slate: "theme.slate",
  blossom: "theme.blossom",
};

export const STORAGE_KEY = "edi.theme";

// Hex palettes per theme: the nine attribute hues plus the semantic accents
// that `lib/theme.ts` hands to badges, pips and the loot ramp. Keep every
// value a 6-digit hex — callers append alpha bytes.
export interface Palette {
  attrs: Record<string, string>;
  fallback: string; // unknown attribute / "side" type / common loot
  phos: string; // primary accent (success, running, active days)
  gold: string; // XP / gold / dailies
  boss: string; // always alarm-red-ish
  focus: string; // "main" type / rare loot / info
  epic: string; // learning purple / epic loot
  peach: string; // creativity / hard difficulty
  teal: string; // spirituality / recovery / trivial
  ink: string; // body text (mood meter cap, sliders)
}

const palettes: Record<Theme, Palette> = {
  crt: {
    attrs: {
      strength: "#ff5f56",
      discipline: "#6f7dff",
      focus: "#35e0ff",
      health: "#4bff7e",
      wealth: "#ffb000",
      relationships: "#ff6ac1",
      learning: "#b98aff",
      creativity: "#ffa23e",
      spirituality: "#2ee6c8",
    },
    fallback: "#6fae7e",
    phos: "#4bff7e",
    gold: "#ffb000",
    boss: "#ff4747",
    focus: "#35e0ff",
    epic: "#b98aff",
    peach: "#ffa23e",
    teal: "#2ee6c8",
    ink: "#d2f5d8",
  },
  slate: {
    attrs: {
      strength: "#f26d63",
      discipline: "#7f8cf7",
      focus: "#38bdf8",
      health: "#4ade80",
      wealth: "#f5b942",
      relationships: "#f472b6",
      learning: "#a78bfa",
      creativity: "#fb923c",
      spirituality: "#2dd4bf",
    },
    fallback: "#8b94a7",
    phos: "#7fb2ff",
    gold: "#f5b942",
    boss: "#f0525a",
    focus: "#38bdf8",
    epic: "#a78bfa",
    peach: "#fb923c",
    teal: "#2dd4bf",
    ink: "#e6e9ef",
  },
  blossom: {
    attrs: {
      strength: "#ff6f91",
      discipline: "#a393ff",
      focus: "#7fd4ff",
      health: "#8ef0b4",
      wealth: "#ffcf7a",
      relationships: "#ff9ff0",
      learning: "#cdb7ff",
      creativity: "#ffb28a",
      spirituality: "#9de9de",
    },
    fallback: "#b596b0",
    phos: "#ff8fc8",
    gold: "#ffc078",
    boss: "#ff5c7a",
    focus: "#7fd4ff",
    epic: "#cdb7ff",
    peach: "#ffb28a",
    teal: "#9de9de",
    ink: "#fbe9f5",
  },
};

// The <meta name="theme-color"> value (browser chrome / PWA title bar).
const chromeColor: Record<Theme, string> = { crt: "#050a06", slate: "#0f1115", blossom: "#1a1020" };

function isTheme(v: unknown): v is Theme {
  return typeof v === "string" && (THEMES as string[]).includes(v);
}

function detect(): Theme {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (isTheme(saved)) return saved;
  } catch {
    /* private mode — fall through */
  }
  return DEFAULT_THEME;
}

let current: Theme = detect();

function apply(theme: Theme): void {
  if (typeof document === "undefined") return;
  document.documentElement.dataset.theme = theme;
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", chromeColor[theme]);
}
apply(current);

type Listener = (t: Theme) => void;
let listeners: Listener[] = [];

export function getTheme(): Theme {
  return current;
}

/** Hex palette of the active theme (safe from module code and getters). */
export function palette(): Palette {
  return palettes[current];
}

export function setTheme(next: Theme): void {
  if (next === current) return;
  current = next;
  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch {
    /* keep in-memory only */
  }
  apply(next);
  listeners.forEach((l) => l(next));
}

// --- React binding ------------------------------------------------------------

interface ThemeValue {
  theme: Theme;
  setTheme: (t: Theme) => void;
}

const ThemeContext = createContext<ThemeValue>({ theme: current, setTheme });

export function ThemeProvider({ children }: { children: (theme: Theme) => ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(current);
  useEffect(() => {
    const l: Listener = (next) => setThemeState(next);
    listeners.push(l);
    return () => {
      listeners = listeners.filter((x) => x !== l);
    };
  }, []);
  return createElement(ThemeContext.Provider, { value: { theme, setTheme } }, children(theme));
}

export function useTheme(): ThemeValue {
  return useContext(ThemeContext);
}
