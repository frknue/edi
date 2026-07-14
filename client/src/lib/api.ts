import type {
  Attribute,
  AgentSuggestion,
  CompletionResult,
  Dashboard,
  GoldEvent,
  JournalCreateResult,
  JournalEntry,
  MoodAssistResult,
  MoodLog,
  OpenAIModel,
  OpenAIStatus,
  PurchaseResult,
  Quest,
  QuestInput,
  RestState,
  ShopItem,
  ShopItemInput,
  Subtask,
  ToolCompletionResult,
  ToolDefinition,
  ToolEntry,
  WardResult,
  XPEvent,
} from "./types";

// Base path is relative; the Vite dev proxy (and the Go static server in prod)
// route /api to the backend. No hidden client — every call hits the documented API.
const API = "/api";

// Optional bearer auth: when the server runs with EDI_TOKEN, open the app once
// as http://host:8080/#token=<secret> — the token is stored locally and sent on
// every request from then on. Tokenless servers ignore the header.
const TOKEN_KEY = "edi_token";

function captureTokenFromHash(): void {
  const match = window.location.hash.match(/#token=([^&]+)/);
  if (match) {
    localStorage.setItem(TOKEN_KEY, decodeURIComponent(match[1]));
    history.replaceState(null, "", window.location.pathname + window.location.search);
  }
}
captureTokenFromHash();

function authHeaders(): Record<string, string> {
  const token = localStorage.getItem(TOKEN_KEY);
  return token ? { Authorization: `Bearer ${token}` } : {};
}

class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...authHeaders(), ...init?.headers },
  });
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* ignore */
    }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  getDashboard: () => request<Dashboard>("/dashboard"),
  getAttributes: () => request<Attribute[]>("/attributes"),

  listQuests: (params?: { type?: string; status?: string }) => {
    const q = new URLSearchParams();
    if (params?.type) q.set("type", params.type);
    if (params?.status) q.set("status", params.status);
    const qs = q.toString();
    return request<Quest[]>(`/quests${qs ? `?${qs}` : ""}`);
  },
  createQuest: (input: QuestInput) =>
    request<Quest>("/quests", { method: "POST", body: JSON.stringify(input) }),
  updateQuest: (id: number, patch: Partial<QuestInput> & { status?: string }) =>
    request<Quest>(`/quests/${id}`, { method: "PATCH", body: JSON.stringify(patch) }),
  completeQuest: (id: number) =>
    request<CompletionResult>(`/quests/${id}/complete`, { method: "POST" }),
  toggleSubtask: (questId: number, subtaskId: number) =>
    request<Subtask>(`/quests/${questId}/subtasks/${subtaskId}/toggle`, { method: "POST" }),
  skipQuest: (id: number) => request<Quest>(`/quests/${id}/skip`, { method: "POST" }),
  archiveQuest: (id: number) => request<Quest>(`/quests/${id}/archive`, { method: "POST" }),

  getXPEvents: (limit = 50) => request<XPEvent[]>(`/xp-events?limit=${limit}`),

  listJournal: (limit = 30, q = "") =>
    request<JournalEntry[]>(`/journal?limit=${limit}${q ? `&q=${encodeURIComponent(q)}` : ""}`),
  createJournal: (input: { mood: number; energy: number; notes: string }) =>
    request<JournalCreateResult>("/journal", { method: "POST", body: JSON.stringify(input) }),
  updateJournal: (id: number, patch: { mood?: number; energy?: number; notes?: string }) =>
    request<JournalEntry>(`/journal/${id}`, { method: "PATCH", body: JSON.stringify(patch) }),
  deleteJournal: (id: number) =>
    request<{ deleted: boolean }>(`/journal/${id}`, { method: "DELETE" }),

  listShop: () => request<ShopItem[]>("/shop"),
  createShopItem: (input: ShopItemInput) =>
    request<ShopItem>("/shop", { method: "POST", body: JSON.stringify(input) }),
  updateShopItem: (id: number, patch: { name?: string; price?: number }) =>
    request<ShopItem>(`/shop/${id}`, { method: "PATCH", body: JSON.stringify(patch) }),
  archiveShopItem: (id: number) =>
    request<{ archived: boolean }>(`/shop/${id}/archive`, { method: "POST" }),
  purchaseShopItem: (id: number) =>
    request<PurchaseResult>(`/shop/${id}/purchase`, { method: "POST" }),
  listGoldEvents: (limit = 30, source?: string) =>
    request<GoldEvent[]>(`/gold/events?limit=${limit}${source ? `&source=${encodeURIComponent(source)}` : ""}`),

  wardAttribute: (key: string) =>
    request<WardResult>(`/attributes/${key}/ward`, { method: "POST" }),
  setRestMode: (on: boolean) =>
    request<RestState>("/rest", { method: "POST", body: JSON.stringify({ on }) }),

  listSuggestions: (status?: string) =>
    request<AgentSuggestion[]>(`/agent/suggestions${status ? `?status=${status}` : ""}`),
  generateSuggestions: () =>
    request<AgentSuggestion[]>("/agent/suggestions/generate", { method: "POST" }),
  acceptSuggestion: (id: number) =>
    request<Quest>(`/agent/suggestions/${id}/accept`, { method: "POST" }),
  dismissSuggestion: (id: number) =>
    request<AgentSuggestion>(`/agent/suggestions/${id}/dismiss`, { method: "POST" }),

  openaiStatus: () => request<OpenAIStatus>("/openai/status"),
  openaiConnect: () => request<{ auth_url: string }>("/openai/connect", { method: "POST" }),
  openaiImportCodex: () => request<OpenAIStatus>("/openai/import-codex", { method: "POST" }),
  openaiDisconnect: () => request<{ connected: boolean }>("/openai/disconnect", { method: "POST" }),
  openaiConfig: (cfg: { model?: string; effort?: string }) =>
    request<OpenAIStatus>("/openai/config", { method: "POST", body: JSON.stringify(cfg) }),
  openaiModels: () => request<{ models: OpenAIModel[] }>("/openai/models"),

  listTools: () => request<{ tools: ToolDefinition[] }>("/tools").then((r) => r.tools),
  completeTool: (key: string, data: MoodLog) =>
    request<ToolCompletionResult>(`/tools/${key}/complete`, { method: "POST", body: JSON.stringify(data) }),
  toolEntries: (key: string, limit = 30) =>
    request<ToolEntry[]>(`/tools/${key}/entries?limit=${limit}`),
  toolAssist: (
    key: string,
    body: { mode: "distortions" | "responses"; event: string; thought: string; distortions: string[] },
  ) => request<MoodAssistResult>(`/tools/${key}/assist`, { method: "POST", body: JSON.stringify(body) }),
};

export { ApiError };
