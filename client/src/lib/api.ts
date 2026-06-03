import type {
  Attribute,
  AgentSuggestion,
  CompletionResult,
  Dashboard,
  JournalEntry,
  Quest,
  QuestInput,
  XPEvent,
} from "./types";

// Base path is relative; the Vite dev proxy (and the Go static server in prod)
// route /api to the backend. No hidden client — every call hits the documented API.
const API = "/api";

class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
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
  skipQuest: (id: number) => request<Quest>(`/quests/${id}/skip`, { method: "POST" }),
  archiveQuest: (id: number) => request<Quest>(`/quests/${id}/archive`, { method: "POST" }),

  getXPEvents: (limit = 50) => request<XPEvent[]>(`/xp-events?limit=${limit}`),

  listJournal: (limit = 30) => request<JournalEntry[]>(`/journal?limit=${limit}`),
  createJournal: (input: { mood: number; energy: number; notes: string }) =>
    request<JournalEntry>("/journal", { method: "POST", body: JSON.stringify(input) }),

  listSuggestions: (status?: string) =>
    request<AgentSuggestion[]>(`/agent/suggestions${status ? `?status=${status}` : ""}`),
  generateSuggestions: () =>
    request<AgentSuggestion[]>("/agent/suggestions/generate", { method: "POST" }),
  acceptSuggestion: (id: number) =>
    request<Quest>(`/agent/suggestions/${id}/accept`, { method: "POST" }),
  dismissSuggestion: (id: number) =>
    request<AgentSuggestion>(`/agent/suggestions/${id}/dismiss`, { method: "POST" }),
};

export { ApiError };
