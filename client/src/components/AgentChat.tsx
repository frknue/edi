import { useEffect, useRef, useState } from "react";
import type { FormEvent, KeyboardEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { Bot, Eraser, Loader2, MessageSquare, Send, Wrench } from "lucide-react";
import { api, hasToken } from "../lib/api";
import { useChat, useChatPending } from "../lib/queries";
import { useI18n } from "../lib/i18n";
import { formatTime } from "../lib/format";
import type { ChatMessage } from "../lib/types";
import { Btn } from "./ui";
import type { MessageKey } from "../lib/locales/en";

// AgentChat is the web's free-text line to the agent — the SAME loop Telegram
// and `edi-cli chat` use (POST /api/agent/chat), so "add X as a daily" or
// "I took my magnesium" act through the tool registry. The server keeps the
// model's history in memory per session; the visible transcript lives in
// localStorage per user on this device (a redeploy forgets the server side,
// which is fine — every action already landed through the service).
const SESSION = "web";
const MAX_MESSAGES = 100;

const EXAMPLE_KEYS: MessageKey[] = ["chat.example1", "chat.example2", "chat.example3"];

function readTranscript(key: string): ChatMessage[] {
  try {
    const raw = localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as ChatMessage[]) : [];
  } catch {
    return [];
  }
}

function writeTranscript(key: string, msgs: ChatMessage[]): void {
  try {
    localStorage.setItem(key, JSON.stringify(msgs.slice(-MAX_MESSAGES)));
  } catch {
    /* private mode / quota — keep in-memory only */
  }
}

function appendTranscript(key: string, msg: ChatMessage): ChatMessage[] {
  const next = [...readTranscript(key), msg];
  writeTranscript(key, next);
  return next;
}

function dropLastUserMessage(key: string, id: string): ChatMessage[] {
  const next = readTranscript(key).filter((m) => m.id !== id);
  writeTranscript(key, next);
  return next;
}

const newId = () => `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`;

export function AgentChat() {
  // Transcripts are per user: on a token-protected server wait for /me so a
  // second person pasting their token on this device never sees another's.
  const tokened = hasToken();
  const { data: me } = useQuery({ queryKey: ["me"], queryFn: api.me, enabled: tokened, retry: false });
  const userKey = tokened ? (me ? String(me.id) : null) : "dev";
  if (!userKey) return null;
  return <ChatPanel key={userKey} storageKey={`edi.chat.${userKey}`} />;
}

function ChatPanel({ storageKey }: { storageKey: string }) {
  const { t } = useI18n();
  const [messages, setMessages] = useState<ChatMessage[]>(() => readTranscript(storageKey));
  const [draft, setDraft] = useState("");
  const [resetNext, setResetNext] = useState(false);
  const pending = useChatPending();
  const listRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const chat = useChat({
    onSuccess: (res) => {
      // Hook-level: runs even if this panel unmounted mid-reply.
      const next = appendTranscript(storageKey, {
        id: newId(),
        role: "agent",
        text: res.reply,
        tools: res.tools_used,
        at: new Date().toISOString(),
      });
      setMessages(next);
    },
    onError: (_err, vars) => {
      // The global toast shows the error; give the draft back instead of
      // leaving an orphaned bubble.
      const next = dropLastUserMessage(storageKey, vars.clientId ?? "");
      setMessages(next);
      setDraft(vars.message);
    },
  });

  // Keep the newest line in view.
  useEffect(() => {
    const el = listRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages.length, pending]);

  const send = (text: string) => {
    const message = text.trim();
    if (!message || pending) return;
    const id = newId();
    setMessages(appendTranscript(storageKey, { id, role: "user", text: message, at: new Date().toISOString() }));
    setDraft("");
    chat.mutate({ message, session: SESSION, reset: resetNext, clientId: id });
    setResetNext(false);
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    send(draft);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      send(draft);
    }
  };

  // "New conversation": the server rejects an empty message, so the reset
  // rides along with the next message; the visible transcript clears now.
  const startNew = () => {
    writeTranscript(storageKey, []);
    setMessages([]);
    setResetNext(true);
    inputRef.current?.focus();
  };

  return (
    <div className="hud-panel clip-corner flex flex-col p-4" data-testid="agent-chat">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium text-ink">
            <MessageSquare size={15} style={{ color: "#b98aff" }} /> {t("chat.title")}
          </div>
          <p className="mt-0.5 text-xs text-faint">{t("chat.hint")}</p>
        </div>
        {(messages.length > 0 || resetNext) && (
          <Btn variant="soft" onClick={startNew} disabled={pending} data-testid="chat-new" className="!px-2.5 !py-1.5 !text-[11px]">
            <Eraser size={13} /> {t("chat.new")}
          </Btn>
        )}
      </div>

      <div
        ref={listRef}
        className="max-h-[52vh] min-h-[160px] space-y-3 overflow-y-auto pr-1"
        data-testid="chat-transcript"
        aria-live="polite"
      >
        {messages.length === 0 && !pending ? (
          <div className="flex h-full flex-col items-start justify-center gap-2 py-4">
            <p className="text-sm text-muted">{t("chat.emptyTitle")}</p>
            <div className="flex flex-wrap gap-1.5">
              {EXAMPLE_KEYS.map((k) => (
                <button
                  key={k}
                  onClick={() => send(t(k))}
                  className="rounded-sm border border-edge px-2 py-1 text-left text-[12px] text-muted transition-colors hover:border-edge2 hover:text-ink"
                  data-testid="chat-example"
                >
                  “{t(k)}”
                </button>
              ))}
            </div>
          </div>
        ) : (
          messages.map((m) => <Bubble key={m.id} message={m} />)
        )}
        {pending && (
          <div className="flex items-center gap-2 text-xs text-faint" data-testid="chat-thinking">
            <Loader2 size={13} className="animate-spin" style={{ color: "#b98aff" }} /> {t("chat.thinking")}
          </div>
        )}
      </div>

      <form onSubmit={onSubmit} className="mt-3 flex items-end gap-2">
        <textarea
          ref={inputRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={onKeyDown}
          rows={draft.includes("\n") ? 3 : 1}
          maxLength={4000}
          placeholder={t("chat.placeholder")}
          aria-label={t("chat.placeholder")}
          data-testid="chat-input"
          className="min-h-[40px] w-full resize-none rounded-sm border border-edge bg-white/[0.03] px-3 py-2 text-sm text-ink placeholder:text-faint focus:border-edge2 focus:outline-none"
        />
        <Btn type="submit" variant="primary" disabled={pending || !draft.trim()} data-testid="chat-send" aria-label={t("chat.send")}>
          {pending ? <Loader2 size={14} className="animate-spin" /> : <Send size={14} />}
          <span className="hidden sm:inline">{t("chat.send")}</span>
        </Btn>
      </form>
      <p className="mt-1.5 text-[10px] text-faint">{t("chat.enterHint")}</p>
    </div>
  );
}

function Bubble({ message: m }: { message: ChatMessage }) {
  const { t } = useI18n();
  const mine = m.role === "user";
  return (
    <div className={`flex ${mine ? "justify-end" : "justify-start"}`} data-testid={`chat-msg-${m.role}`}>
      <div className={`max-w-[85%] ${mine ? "text-right" : ""}`}>
        <div
          className="inline-block whitespace-pre-wrap rounded-md px-3 py-2 text-left text-sm leading-relaxed text-ink"
          style={
            mine
              ? { background: "rgba(var(--gold-rgb),0.10)", border: "1px solid rgba(var(--gold-rgb),0.25)" }
              : { background: "rgba(185,138,255,0.08)", border: "1px solid rgba(185,138,255,0.25)" }
          }
        >
          {!mine && <Bot size={12} className="mb-0.5 mr-1.5 inline" style={{ color: "#b98aff" }} />}
          {m.text}
        </div>
        <div className={`mt-1 flex flex-wrap items-center gap-1.5 text-[10px] text-faint ${mine ? "justify-end" : ""}`}>
          <span>{formatTime(m.at)}</span>
          {m.tools && m.tools.length > 0 && (
            <span className="inline-flex items-center gap-1" title={t("chat.toolsUsed")} data-testid="chat-tools">
              <Wrench size={10} /> {m.tools.join(", ")}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
