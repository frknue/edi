import { useEffect, useState } from "react";
import { Bot, Check, Clock, Download, RefreshCw, ScrollText, Send, Sparkles, Swords, Unplug } from "lucide-react";
import {
  useSuggestions,
  useGenerateSuggestions,
  useAcceptSuggestion,
  useDismissSuggestion,
  useOpenAIStatus,
  useCompleteOpenAIConnect,
  useConnectOpenAI,
  useImportCodex,
  useDisconnectOpenAI,
  useSetOpenAIConfig,
  useOpenAIModels,
  useTelegramPairCode,
  useTelegramStatus,
  useTelegramUnlink,
  useTelegramPushTimes,
  useSetTelegramPushTimes,
  useStory,
  useForgeBoss,
} from "../lib/queries";
import { SuggestionCard } from "../components/SuggestionCard";
import { Btn, EmptyState, SectionTitle, Spinner } from "../components/ui";
import { pushToast } from "../lib/toast";
import type { OpenAIStatus } from "../lib/types";
import { useI18n } from "../lib/i18n";
import type { MessageKey } from "../lib/locales/en";

// Reasoning-effort tiers, fastest → deepest. Which tiers show comes from the
// selected model (gpt-5.6 adds "max"; sol/terra also "ultra"). Labels/hints
// live in the i18n tables as effort.<tier> / effort.<tier>.hint.
const EFFORT_TIERS = ["none", "low", "medium", "high", "xhigh", "max", "ultra"];

export function SuggestionsPage() {
  const { t } = useI18n();
  const [connecting, setConnecting] = useState(false);
  const { data: status, isLoading: statusLoading } = useOpenAIStatus(connecting);
  const connected = !!status?.connected;

  const { data: suggestions, isLoading } = useSuggestions();
  const generate = useGenerateSuggestions();
  const accept = useAcceptSuggestion();
  const dismiss = useDismissSuggestion();

  const pending = suggestions?.filter((s) => s.status === "pending") ?? [];
  const resolved = suggestions?.filter((s) => s.status !== "pending") ?? [];
  const busy = accept.isPending || dismiss.isPending;

  // Stop polling + celebrate once the OAuth round-trip lands.
  useEffect(() => {
    if (connecting && connected) {
      setConnecting(false);
      pushToast(status?.email ? t("agent.connectedAs", { email: status.email }) : t("agent.connected"), "success");
    }
  }, [connecting, connected, status?.email, t]);

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 font-display text-xl font-bold tracking-tight text-ink">
            <Bot size={20} style={{ color: "#b98aff" }} /> {t("agent.title")}
          </h1>
          <p className="text-sm text-faint">{t("agent.subtitle")}</p>
        </div>
        {connected && (
          <Btn
            variant="primary"
            disabled={generate.isPending}
            onClick={() => generate.mutate()}
            data-testid="generate-suggestions"
          >
            <RefreshCw size={15} className={generate.isPending ? "animate-spin" : ""} /> {t("agent.generate")}
          </Btn>
        )}
      </div>

      {statusLoading ? (
        <Spinner />
      ) : !connected ? (
        <ConnectCard connecting={connecting} setConnecting={setConnecting} />
      ) : (
        <>
          {status && <ConnectedBar status={status} />}

          <OracleCard />

          {isLoading ? (
            <Spinner />
          ) : (
            <>
              <section>
                <SectionTitle hint={t("agent.pendingHint")}>{t("agent.pending")}</SectionTitle>
                {pending.length === 0 ? (
                  <EmptyState
                    icon={<Sparkles size={22} />}
                    title={t("agent.noSuggestions")}
                    hint={t("agent.noSuggestionsHint")}
                  />
                ) : (
                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    {pending.map((s, i) => (
                      <SuggestionCard
                        key={s.id}
                        suggestion={s}
                        index={i}
                        busy={busy}
                        onAccept={(id) =>
                          accept.mutate(id, {
                            onSuccess: (q) => pushToast(t("dash.addedQuest", { title: q.title }), "success"),
                          })
                        }
                        onDismiss={(id) => dismiss.mutate(id)}
                      />
                    ))}
                  </div>
                )}
              </section>

              {resolved.length > 0 && (
                <section>
                  <SectionTitle hint={t("agent.historyHint")}>{t("agent.history")}</SectionTitle>
                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    {resolved.map((s, i) => (
                      <SuggestionCard key={s.id} suggestion={s} index={i} />
                    ))}
                  </div>
                </section>
              )}
            </>
          )}
        </>
      )}

      <TelegramCard />
    </div>
  );
}

function ConnectCard({
  connecting,
  setConnecting,
}: {
  connecting: boolean;
  setConnecting: (v: boolean) => void;
}) {
  const { t } = useI18n();
  const connect = useConnectOpenAI();
  const complete = useCompleteOpenAIConnect();
  const importCodex = useImportCodex();
  const [pasted, setPasted] = useState("");

  const startConnect = () =>
    connect.mutate(undefined, {
      onSuccess: (res) => {
        setConnecting(true);
        window.open(res.auth_url, "_blank", "noopener,noreferrer");
      },
    });

  const finishConnect = () =>
    complete.mutate(pasted, {
      onSuccess: () => setPasted(""),
    });

  return (
    <div className="hud-panel clip-corner relative overflow-hidden p-6 text-center">
      <div
        className="pointer-events-none absolute -right-10 -top-16 h-56 w-56 rounded-full"
        style={{ background: "radial-gradient(circle, rgba(52,208,255,0.16), transparent 70%)" }}
      />
      <div
        className="relative mx-auto mb-3 grid h-12 w-12 place-items-center rounded-full"
        style={{ background: "rgba(185,138,255,0.14)", color: "#b98aff" }}
      >
        <Sparkles size={22} />
      </div>
      <h2 className="relative font-display text-lg font-bold text-ink">{t("agent.connectTitle")}</h2>
      <p className="relative mx-auto mt-1 max-w-md text-sm text-muted">
        {t("agent.connectBody1")}
        <strong className="text-ink">{t("agent.yourOwn")}</strong>
        {t("agent.connectBody2")}
      </p>

      <div className="relative mt-5 flex flex-col items-center justify-center gap-2 sm:flex-row">
        <Btn variant="primary" disabled={connecting || connect.isPending} onClick={startConnect} data-testid="connect-openai">
          {connecting ? (
            <>
              <RefreshCw size={15} className="animate-spin" /> {t("agent.waiting")}
            </>
          ) : (
            <>
              <Sparkles size={15} /> {t("agent.connect")}
            </>
          )}
        </Btn>
        <Btn
          variant="ghost"
          disabled={importCodex.isPending}
          onClick={() => importCodex.mutate()}
          data-testid="import-codex"
        >
          <Download size={15} /> {t("agent.importCodex")}
        </Btn>
      </div>
      {connecting && (
        <div className="relative mx-auto mt-5 max-w-md space-y-2 text-left">
          <p className="text-[11px] leading-relaxed text-faint">
            {t("agent.callback1")}
            <code className="text-muted">localhost:1455</code>
            {t("agent.callback2")}
            <span className="text-muted">{t("agent.callbackCopy")}</span>
            {t("agent.callback3")}
          </p>
          <div className="flex gap-2">
            <input
              value={pasted}
              onChange={(e) => setPasted(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && pasted.trim() !== "" && finishConnect()}
              placeholder="http://localhost:1455/auth/callback?code=…"
              autoCapitalize="off"
              autoCorrect="off"
              spellCheck={false}
              data-testid="oauth-paste"
              className="min-w-0 flex-1 rounded-lg border border-edge bg-white/[0.03] px-3 py-2 text-xs text-ink placeholder:text-faint focus:border-[var(--color-gold)] focus:outline-none"
            />
            <Btn
              variant="primary"
              disabled={pasted.trim() === "" || complete.isPending}
              onClick={finishConnect}
              data-testid="oauth-paste-submit"
            >
              {t("agent.finish")}
            </Btn>
          </div>
        </div>
      )}
      <p className="relative mt-3 text-[11px] text-faint">
        {t("agent.codexNote")}
      </p>
    </div>
  );
}

function ConnectedBar({ status }: { status: OpenAIStatus }) {
  const { t } = useI18n();
  const disconnect = useDisconnectOpenAI();
  const setConfig = useSetOpenAIConfig();
  const { data: models } = useOpenAIModels(true);

  const currentModel = status.model ?? "";
  const selected = models?.find((m) => m.slug === currentModel);
  // Effort choices come from the selected model when known, else the status list.
  const effortOptions = selected?.efforts ?? status.effort_options ?? ["low", "medium", "high", "xhigh"];
  const currentEffort = status.effort ?? "medium";

  const changeModel = (slug: string) => {
    const m = models?.find((x) => x.slug === slug);
    // If the new model doesn't support the current effort, fall back to its default.
    const effort = m && !m.efforts.includes(currentEffort) ? m.default_effort || m.efforts[0] : undefined;
    setConfig.mutate(
      { model: slug, ...(effort ? { effort } : {}) },
      { onSuccess: () => pushToast(t("agent.modelSet", { model: m?.display_name ?? slug }), "success") },
    );
  };

  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3 rounded-xl border border-edge bg-white/[0.02] px-4 py-2.5">
      <div className="flex items-center gap-2 text-sm">
        <span className="grid h-6 w-6 place-items-center rounded-full" style={{ background: "rgba(75,255,126,0.16)", color: "#4bff7e" }}>
          <Check size={13} />
        </span>
        <span className="text-ink">{status.email || t("agent.chatgptConnected")}</span>
      </div>

      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        {/* Model dropdown (populated from the account's available models) */}
        <label className="flex items-center gap-1.5">
          <span className="hidden text-[11px] text-faint sm:inline">{t("agent.model")}</span>
          <select
            value={currentModel}
            disabled={setConfig.isPending || !models}
            onChange={(e) => changeModel(e.target.value)}
            data-testid="model-select"
            className="rounded-lg border border-edge bg-void/60 px-2 py-1 text-xs text-ink focus:border-[var(--color-gold)] focus:outline-none"
          >
            {!models && <option>{currentModel || "…"}</option>}
            {models?.map((m) => (
              <option key={m.slug} value={m.slug}>
                {m.display_name}
              </option>
            ))}
          </select>
        </label>

        {/* Reasoning-effort segmented control (per the selected model) */}
        <div className="flex items-center gap-1.5">
          <span className="hidden text-[11px] text-faint sm:inline">{t("agent.reasoning")}</span>
          <div className="flex rounded-lg border border-edge bg-void/40 p-0.5" role="group" aria-label={t("agent.reasoningEffort")}>
            {effortOptions.map((e) => {
              const known = EFFORT_TIERS.includes(e);
              const meta = known
                ? { label: t(`effort.${e}` as MessageKey), hint: t(`effort.${e}.hint` as MessageKey) }
                : { label: e, hint: "" };
              const active = e === currentEffort;
              return (
                <button
                  key={e}
                  title={meta.hint}
                  disabled={setConfig.isPending}
                  onClick={() =>
                    setConfig.mutate(
                      { effort: e },
                      { onSuccess: () => pushToast(t("agent.reasoningSet", { level: meta.label }), "success") },
                    )
                  }
                  data-testid={`effort-${e}`}
                  className="rounded-md px-2 py-1 text-[11px] font-medium transition-colors"
                  style={{
                    background: active ? "rgba(185,138,255,0.18)" : "transparent",
                    color: active ? "#cbaaff" : "var(--color-faint)",
                  }}
                >
                  {meta.label}
                </button>
              );
            })}
          </div>
        </div>

        <Btn variant="soft" disabled={disconnect.isPending} onClick={() => disconnect.mutate()} data-testid="disconnect-openai">
          <Unplug size={14} /> {t("agent.disconnect")}
        </Btn>
      </div>
    </div>
  );
}

// OracleCard exposes the two AI "story mode" actions that Telegram has had
// all along (/story, /boss) — same service methods, same gating.
function OracleCard() {
  const { t } = useI18n();
  const story = useStory();
  const forge = useForgeBoss();
  const [text, setText] = useState<string | null>(null);

  return (
    <div className="hud-panel space-y-3 p-4" data-testid="oracle-card">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium text-ink">
            <ScrollText size={15} style={{ color: "#b98aff" }} /> {t("oracle.title")}
          </div>
          <p className="mt-0.5 text-xs text-faint">{t("oracle.hint")}</p>
        </div>
        <div className="flex gap-2">
          <Btn
            disabled={story.isPending}
            onClick={() => story.mutate(undefined, { onSuccess: (r) => setText(r.story) })}
            data-testid="oracle-story"
          >
            <ScrollText size={14} className={story.isPending ? "animate-pulse" : ""} /> {t("oracle.story")}
          </Btn>
          <Btn
            variant="primary"
            disabled={forge.isPending}
            onClick={() =>
              forge.mutate(undefined, {
                onSuccess: (q) => pushToast(t("oracle.bossForged", { title: q.title }), "success"),
              })
            }
            data-testid="oracle-boss"
          >
            <Swords size={14} className={forge.isPending ? "animate-pulse" : ""} /> {t("oracle.boss")}
          </Btn>
        </div>
      </div>
      {text && (
        <p className="border-l-2 border-[#b98aff]/50 pl-3 text-sm italic leading-relaxed text-muted" data-testid="oracle-text">
          {text}
        </p>
      )}
    </div>
  );
}

// PushTimes lets a linked user set the briefing/nudge clock from the web —
// the same setting /briefing HH:MM writes in chat (empty = server default).
function PushTimes() {
  const { t } = useI18n();
  const { data } = useTelegramPushTimes();
  const save = useSetTelegramPushTimes();
  // The reset control lives NEXT TO the label, not inside it — a button nested
  // in a <label> gets its click re-targeted to the input and never fires.
  const field = (kind: "briefing" | "nudge") => (
    <span className="flex items-center gap-2 text-xs text-muted">
      <label className="flex items-center gap-2">
        <span className="w-16">{t(kind === "briefing" ? "tg.briefing" : "tg.nudge")}</span>
        <input
          type="time"
          value={data?.[kind] ?? ""}
          onChange={(e) => save.mutate({ [kind]: e.target.value })}
          className="rounded border border-edge bg-transparent px-2 py-1 text-ink tabnum"
          data-testid={`tg-time-${kind}`}
        />
      </label>
      {data?.[kind] ? (
        <button
          type="button"
          className="text-faint underline"
          onClick={() => save.mutate({ [kind]: "" })}
          data-testid={`tg-time-${kind}-reset`}
        >
          {t("tg.reset")}
        </button>
      ) : (
        <span className="text-faint">{t("tg.serverDefault")}</span>
      )}
    </span>
  );
  return (
    <div className="mt-2 flex flex-wrap items-center gap-x-5 gap-y-1" data-testid="tg-push-times">
      <Clock size={13} className="text-faint" />
      {field("briefing")}
      {field("nudge")}
    </div>
  );
}

// TelegramCard pairs this account with the Telegram bot: pushes (briefing,
// nudge) and chat commands (/status, /done). Hidden when the server has no
// bot configured. The pair code is shown once and burns on use.
function TelegramCard() {
  const { t } = useI18n();
  const { data: tg, refetch } = useTelegramStatus();
  const pair = useTelegramPairCode();
  const unlink = useTelegramUnlink();
  const [code, setCode] = useState<{ code: string; bot: string } | null>(null);

  // The status flips to linked once the user sends the code in Telegram —
  // poll briefly while a code is showing.
  useEffect(() => {
    if (!code) return;
    const t = setInterval(() => refetch(), 3000);
    return () => clearInterval(t);
  }, [code, refetch]);
  useEffect(() => {
    if (tg?.linked && code) {
      setCode(null);
      pushToast(t("tg.linkedToast"), "success");
    }
  }, [tg?.linked, code, t]);

  if (!tg?.configured) return null;

  return (
    <div className="hud-panel flex flex-wrap items-center justify-between gap-3 p-4">
      <div className="min-w-0">
        <div className="flex items-center gap-2 text-sm font-medium text-ink">
          <Send size={15} style={{ color: "#34d0ff" }} /> Telegram
          {tg.linked && (
            <span className="rounded px-1.5 py-0.5 text-[10px] uppercase" style={{ background: "rgba(75,255,126,0.14)", color: "var(--color-phos)" }}>
              {t("tg.linked")}
            </span>
          )}
        </div>
        <p className="mt-0.5 text-xs text-faint">
          {tg.linked ? t("tg.linkedHint") : t("tg.unlinkedHint")}
        </p>
        {tg.linked && <PushTimes />}
        {code && (
          <div className="mt-2 space-y-1">
            <p className="text-xs text-muted">
              {t("tg.sendCode")}<code className="tabnum text-ink" data-testid="tg-code">/pair {code.code}</code>
            </p>
            <a
              href={`https://t.me/${code.bot}?start=${code.code}`}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-block text-xs underline"
              style={{ color: "#34d0ff" }}
            >
              {t("tg.openBot", { bot: code.bot })}
            </a>
          </div>
        )}
      </div>
      {tg.linked ? (
        <Btn variant="ghost" disabled={unlink.isPending} onClick={() => unlink.mutate()} data-testid="tg-unlink">
          <Unplug size={14} /> {t("tg.unlink")}
        </Btn>
      ) : (
        <Btn
          variant="primary"
          disabled={pair.isPending}
          onClick={() => pair.mutate(undefined, { onSuccess: (r) => setCode({ code: r.code, bot: r.bot_username }) })}
          data-testid="tg-pair"
        >
          <Send size={14} /> {t("tg.link")}
        </Btn>
      )}
    </div>
  );
}
