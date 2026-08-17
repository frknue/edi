import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { en, type MessageKey } from "./locales/en";
import { de } from "./locales/de";

// Tiny, dependency-free i18n. Messages are flat keyed strings with `{name}`
// placeholders; the German table is typed against the English keys so a
// missing translation fails `tsc`, not the user. The active locale is a
// per-device preference (localStorage), defaulting to the browser language.
//
// `t()` is safe to call from anywhere (module code, MutationCache callbacks,
// theme lookups) — it reads the current locale. React code that must repaint
// on a switch uses `useI18n()`; the provider also remounts the tree via
// `key={locale}` in main.tsx, so a language change is a clean full repaint.

export type Locale = "en" | "de";
export const LOCALES: Locale[] = ["en", "de"];
export const localeLabel: Record<Locale, string> = { en: "English", de: "Deutsch" };

const STORAGE_KEY = "edi.locale";
const tables: Record<Locale, Record<MessageKey, string>> = { en, de };

function detect(): Locale {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === "en" || saved === "de") return saved;
  } catch {
    /* private mode — fall through */
  }
  const lang = (typeof navigator !== "undefined" ? navigator.language : "en") || "en";
  return lang.toLowerCase().startsWith("de") ? "de" : "en";
}

let current: Locale = detect();
if (typeof document !== "undefined") document.documentElement.lang = current;

type Listener = (l: Locale) => void;
let listeners: Listener[] = [];

export function getLocale(): Locale {
  return current;
}

export function setLocale(next: Locale): void {
  if (next === current) return;
  current = next;
  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch {
    /* keep in-memory only */
  }
  if (typeof document !== "undefined") document.documentElement.lang = next;
  listeners.forEach((l) => l(next));
}

type Vars = Record<string, string | number>;

function interpolate(template: string, vars?: Vars): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (m, name: string) =>
    name in vars ? String(vars[name]) : m,
  );
}

/** Translate a message key in the current locale (falls back to English). */
export function t(key: MessageKey, vars?: Vars): string {
  const table = tables[current];
  return interpolate(table[key] ?? en[key] ?? key, vars);
}

// Plural pairs are `<base>.one` / `<base>.other`; `tp("quests.count", 3)`
// picks the form by count and injects it as `{n}`.
type PluralBase = {
  [K in MessageKey]: K extends `${infer B}.one` ? (`${B}.other` extends MessageKey ? B : never) : never;
}[MessageKey];

export function tp(base: PluralBase, n: number, vars?: Vars): string {
  const key = `${base}.${n === 1 ? "one" : "other"}` as MessageKey;
  return t(key, { n, ...vars });
}

// --- React binding ------------------------------------------------------------

interface I18nValue {
  locale: Locale;
  setLocale: (l: Locale) => void;
  t: typeof t;
  tp: typeof tp;
}

const I18nContext = createContext<I18nValue>({ locale: current, setLocale, t, tp });

export function I18nProvider({ children }: { children: (locale: Locale) => ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(current);
  useEffect(() => {
    const l: Listener = (next) => setLocaleState(next);
    listeners.push(l);
    return () => {
      listeners = listeners.filter((x) => x !== l);
    };
  }, []);
  return <I18nContext.Provider value={{ locale, setLocale, t, tp }}>{children(locale)}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  return useContext(I18nContext);
}
