import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, clearToken, hasToken } from "../lib/api";
import {
  Bot,
  BookHeart,
  BrainCircuit,
  ChevronDown,
  Pill,
  ChevronsLeft,
  ChevronsRight,
  LayoutDashboard,
  LogOut,
  Palette,
  ScrollText,
  Store,
  Wrench,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { CSSProperties } from "react";
import { LOCALES, localeLabel, useI18n } from "../lib/i18n";
import { THEMES, themeLabelKey, useTheme } from "../lib/themes";
import type { MessageKey } from "../lib/locales/en";
import { AccountInviteButton } from "./AccountInvite";

export type View = "dashboard" | "quests" | "shop" | "moodlog" | "supplements" | "journal" | "agent";

type NavItem = { id: View; labelKey: MessageKey; Icon: LucideIcon };

// "Tools" is not a page — in the expanded sidebar it's a collapsible group whose
// children (Daily Mood Log, Supplements, Journal) are the actual destinations. In the
// collapsed rail the children render as direct icons and the group disappears.
export const TOOL_CHILDREN: NavItem[] = [
  { id: "moodlog", labelKey: "nav.moodlog", Icon: BrainCircuit },
  { id: "supplements", labelKey: "nav.supplements", Icon: Pill },
  { id: "journal", labelKey: "nav.journal", Icon: BookHeart },
];

const TOP_ITEMS: NavItem[] = [
  { id: "dashboard", labelKey: "nav.dashboard", Icon: LayoutDashboard },
  { id: "quests", labelKey: "nav.quests", Icon: ScrollText },
  { id: "shop", labelKey: "nav.shop", Icon: Store },
];

const AGENT_ITEM: NavItem = { id: "agent", labelKey: "nav.agent", Icon: Bot };

export function Logo({ collapsed = false }: { collapsed?: boolean }) {
  const { t } = useI18n();
  return (
    <div className="flex items-center gap-2.5">
      <div
        className="grid h-8 w-8 shrink-0 place-items-center rounded-sm border font-display text-lg leading-none"
        style={{
          borderColor: "var(--color-phos)",
          color: "var(--color-phos)",
          boxShadow: "0 0 14px -4px rgba(var(--phos-rgb),0.8), inset 0 0 10px rgba(var(--phos-rgb),0.12)",
        }}
      >
        &gt;_
      </div>
      {!collapsed && (
        <div>
          <div
            className="cursor-blink font-display text-xl leading-none text-ink"
            style={{ color: "var(--color-phos)" }}
          >
            edi
          </div>
          <div className="text-[9px] uppercase tracking-[0.3em] text-faint">{t("app.tagline")}</div>
        </div>
      )}
    </div>
  );
}

const goldStyle = (active: boolean): CSSProperties => ({
  background: active ? "rgba(var(--gold-rgb),0.08)" : "transparent",
  color: active ? "var(--color-goldhi)" : "var(--color-muted)",
});

const tealStyle = (active: boolean): CSSProperties => ({
  background: active ? "rgba(46,230,200,0.10)" : "transparent",
  color: active ? "var(--color-spirituality)" : "var(--color-faint)",
});

export function Sidebar({
  view,
  setView,
  collapsed,
  onToggle,
}: {
  view: View;
  setView: (v: View) => void;
  collapsed: boolean;
  onToggle: () => void;
}) {
  const { t } = useI18n();
  const inToolsGroup = TOOL_CHILDREN.some((c) => c.id === view);
  const [toolsOpen, setToolsOpen] = useState(false);
  const showChildren = toolsOpen || inToolsGroup;

  const navBtn = (
    { id, labelKey, Icon }: NavItem,
    styleFor: (active: boolean) => CSSProperties,
    testId: string,
  ) => {
    const active = view === id;
    const label = t(labelKey);
    return (
      <button
        key={id}
        onClick={() => setView(id)}
        data-testid={testId}
        aria-label={label}
        title={collapsed ? label : undefined}
        className={`group relative flex w-full items-center rounded-lg py-2.5 text-sm font-medium transition-colors ${
          collapsed ? "justify-center px-0" : "gap-3 px-3"
        }`}
        style={styleFor(active)}
      >
        {active && (
          <span
            className="absolute left-0 top-1/2 h-5 w-1 -translate-y-1/2 rounded-r-full"
            style={{ background: "var(--color-gold)" }}
          />
        )}
        <Icon size={18} className="transition-transform group-hover:scale-110" />
        {!collapsed && label}
      </button>
    );
  };

  return (
    <aside
      className={`sticky top-0 hidden h-screen shrink-0 flex-col border-r border-edge py-6 transition-[width] duration-200 lg:flex ${
        collapsed ? "w-14 px-2" : "w-60 px-5"
      }`}
    >
      <div className={collapsed ? "flex justify-center" : undefined}>
        <Logo collapsed={collapsed} />
      </div>
      <nav className="mt-10 flex flex-1 flex-col gap-1">
        {TOP_ITEMS.map((item) => navBtn(item, goldStyle, `nav-${item.id}`))}

        {collapsed ? (
          // Rail: the Tools group flattens — its children become direct icons.
          TOOL_CHILDREN.map((item) => navBtn(item, tealStyle, `nav-tool-${item.id}`))
        ) : (
          <>
            {/* Tools: collapsible group, no page of its own */}
            <button
              onClick={() => setToolsOpen((o) => !o)}
              data-testid="nav-tools"
              aria-expanded={showChildren}
              className="group relative flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors"
              style={goldStyle(inToolsGroup)}
            >
              {inToolsGroup && (
                <span
                  className="absolute left-0 top-1/2 h-5 w-1 -translate-y-1/2 rounded-r-full"
                  style={{ background: "var(--color-gold)" }}
                />
              )}
              <Wrench size={18} className="transition-transform group-hover:scale-110" />
              {t("nav.tools")}
              <ChevronDown
                size={14}
                className="ml-auto transition-transform"
                style={{ transform: showChildren ? "rotate(180deg)" : "none" }}
              />
            </button>
            {showChildren && (
              <div className="ml-4 flex flex-col gap-0.5 border-l border-edge pl-3">
                {TOOL_CHILDREN.map(({ id, labelKey, Icon }) => {
                  const active = view === id;
                  const label = t(labelKey);
                  return (
                    <button
                      key={id}
                      onClick={() => setView(id)}
                      data-testid={`nav-tool-${id}`}
                      className="flex items-center gap-2.5 rounded-md px-2.5 py-2 text-[13px] font-medium transition-colors"
                      style={tealStyle(active)}
                    >
                      <Icon size={15} />
                      {label}
                    </button>
                  );
                })}
              </div>
            )}
          </>
        )}

        {navBtn(AGENT_ITEM, goldStyle, "nav-agent")}
      </nav>

      {!collapsed && <SessionCard />}
      <div className={`mt-3 flex items-center gap-2 ${collapsed ? "flex-col" : ""}`}>
        <LanguageToggle compact={collapsed} />
        <ThemeToggle compact={collapsed} />
        <button
          onClick={onToggle}
          data-testid="sidebar-toggle"
          title={collapsed ? t("nav.expandSidebar") : t("nav.collapseSidebar")}
          aria-label={collapsed ? t("nav.expandSidebar") : t("nav.collapseSidebar")}
          aria-expanded={!collapsed}
          className="flex flex-1 items-center justify-center rounded-lg border border-edge py-2 text-faint transition-colors hover:text-muted"
        >
          {collapsed ? <ChevronsRight size={16} /> : <ChevronsLeft size={16} />}
        </button>
      </div>
    </aside>
  );
}

// SessionCard shows who is signed in on a token-protected server (with a
// sign-out that forgets the token on THIS device only) and the plain
// self-hosted note in tokenless dev mode.
function SessionCard() {
  const { t } = useI18n();
  const { data: me } = useQuery({ queryKey: ["me"], queryFn: api.me, enabled: hasToken(), retry: false });

  if (!hasToken()) {
    return (
      <div className="rounded-xl border border-edge bg-white/[0.02] p-3">
        <div className="text-[11px] font-medium text-muted">{t("session.devMode")}</div>
        <div className="mt-0.5 text-[10px] text-faint">{t("session.devHint")}</div>
      </div>
    );
  }
  return (
    <div className="rounded-xl border border-edge bg-white/[0.02] p-3">
      <div className="flex items-center justify-between">
        <div className="min-w-0">
          <div className="truncate text-[11px] font-medium text-muted">{me?.name ?? "…"}</div>
          <div className="mt-0.5 text-[10px] text-faint">
            {me?.is_admin ? `${t("session.admin")} · ` : ""}
            {t("session.selfHosted")}
          </div>
        </div>
        <button
          onClick={() => {
            clearToken();
            window.location.reload();
          }}
          title={t("session.signOutTitle")}
          aria-label={t("session.signOut")}
          className="shrink-0 text-faint hover:text-ink"
        >
          <LogOut size={14} />
        </button>
      </div>
      <AccountInviteButton />
    </div>
  );
}

// LanguageToggle cycles the UI language (a per-device preference). The tree
// remounts on change (see main.tsx), so every label repaints at once.
export function LanguageToggle({ compact = false }: { compact?: boolean }) {
  const { locale, setLocale, t } = useI18n();
  const next = LOCALES[(LOCALES.indexOf(locale) + 1) % LOCALES.length];
  return (
    <button
      onClick={() => setLocale(next)}
      data-testid="lang-toggle"
      title={t("app.langToggleTitle", { lang: localeLabel[locale] })}
      aria-label={t("app.langToggleTitle", { lang: localeLabel[locale] })}
      className={`flex items-center justify-center rounded-lg border border-edge font-display text-[11px] uppercase tracking-widest text-faint transition-colors hover:text-muted ${
        compact ? "w-full py-2" : "px-3 py-2"
      }`}
    >
      {locale}
    </button>
  );
}

// ThemeToggle cycles the visual theme (a per-device preference, see
// lib/themes.ts). Like the language, a switch remounts the tree.
export function ThemeToggle({ compact = false }: { compact?: boolean }) {
  const { theme, setTheme } = useTheme();
  const { t } = useI18n();
  const next = THEMES[(THEMES.indexOf(theme) + 1) % THEMES.length];
  const title = t("app.themeToggleTitle", { theme: t(themeLabelKey[theme]) });
  return (
    <button
      onClick={() => setTheme(next)}
      data-testid="theme-toggle"
      data-theme-active={theme}
      title={title}
      aria-label={title}
      className={`flex items-center justify-center rounded-lg border border-edge text-faint transition-colors hover:text-muted ${
        compact ? "w-full py-2" : "px-3 py-2"
      }`}
    >
      <Palette size={14} />
    </button>
  );
}
