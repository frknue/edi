import { getLocale, t } from "./i18n";

export function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const diff = Date.now() - then;
  const sec = Math.round(diff / 1000);
  if (sec < 45) return t("time.justNow");
  const min = Math.round(sec / 60);
  if (min < 60) return t("time.minAgo", { n: min });
  const hr = Math.round(min / 60);
  if (hr < 24) return t("time.hourAgo", { n: hr });
  const day = Math.round(hr / 24);
  if (day < 7) return t("time.dayAgo", { n: day });
  return new Date(iso).toLocaleDateString(getLocale(), { month: "short", day: "numeric" });
}

export function formatDateTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return t("common.unknown");
  return date.toLocaleString(getLocale(), { dateStyle: "medium", timeStyle: "short" });
}

export function formatDate(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "?";
  return date.toLocaleDateString(getLocale());
}

export function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(getLocale(), { hour: "2-digit", minute: "2-digit" });
}

export function formatNumber(n: number): string {
  return n.toLocaleString(getLocale());
}

export function compactNumber(n: number): string {
  return new Intl.NumberFormat(getLocale(), { notation: "compact", maximumFractionDigits: 1 }).format(n);
}

export function pct(ratio: number): number {
  return Math.max(0, Math.min(100, Math.round(ratio * 100)));
}
