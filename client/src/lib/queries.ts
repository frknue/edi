import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "./api";
import type { MoodLog, QuestInput } from "./types";

export const keys = {
  dashboard: ["dashboard"] as const,
  attributes: ["attributes"] as const,
  quests: (filters?: { type?: string; status?: string }) => ["quests", filters ?? {}] as const,
  journal: ["journal"] as const,
  suggestions: (status?: string) => ["suggestions", status ?? "all"] as const,
  xpEvents: ["xp-events"] as const,
  openaiStatus: ["openai-status"] as const,
};

export function useDashboard() {
  return useQuery({ queryKey: keys.dashboard, queryFn: api.getDashboard });
}

export function useQuests(filters?: { type?: string; status?: string }) {
  return useQuery({
    queryKey: keys.quests(filters),
    queryFn: () => api.listQuests(filters),
  });
}

export function useJournal(q = "", limit = 60) {
  return useQuery({ queryKey: [...keys.journal, q, limit], queryFn: () => api.listJournal(limit, q) });
}

export function useSuggestions(status?: string) {
  return useQuery({
    queryKey: keys.suggestions(status),
    queryFn: () => api.listSuggestions(status),
  });
}

export function useXPEvents() {
  return useQuery({ queryKey: keys.xpEvents, queryFn: () => api.getXPEvents(50) });
}

// Invalidate everything that a state change can touch.
function useInvalidateAll() {
  const qc = useQueryClient();
  return () => {
    qc.invalidateQueries({ queryKey: ["dashboard"] });
    qc.invalidateQueries({ queryKey: ["attributes"] });
    qc.invalidateQueries({ queryKey: ["quests"] });
    qc.invalidateQueries({ queryKey: ["suggestions"] });
    qc.invalidateQueries({ queryKey: ["xp-events"] });
  };
}

export function useCompleteQuest() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: (id: number) => api.completeQuest(id),
    onSuccess: invalidate,
  });
}

export function useCreateQuest() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: (input: QuestInput) => api.createQuest(input),
    onSuccess: invalidate,
  });
}

export function useUpdateQuest() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: ({ id, patch }: { id: number; patch: Partial<QuestInput> & { status?: string } }) =>
      api.updateQuest(id, patch),
    onSuccess: invalidate,
  });
}

export function useToggleSubtask() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: ({ questId, subtaskId }: { questId: number; subtaskId: number }) =>
      api.toggleSubtask(questId, subtaskId),
    onSuccess: invalidate,
  });
}

export function useSkipQuest() {
  const invalidate = useInvalidateAll();
  return useMutation({ mutationFn: (id: number) => api.skipQuest(id), onSuccess: invalidate });
}

export function useArchiveQuest() {
  const invalidate = useInvalidateAll();
  return useMutation({ mutationFn: (id: number) => api.archiveQuest(id), onSuccess: invalidate });
}

export function useCreateJournal() {
  const invalidate = useInvalidateAll();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { mood: number; energy: number; notes: string }) => api.createJournal(input),
    onSuccess: () => {
      invalidate();
      qc.invalidateQueries({ queryKey: keys.journal });
    },
  });
}

export function useUpdateJournal() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }: { id: number; patch: { mood?: number; energy?: number; notes?: string } }) =>
      api.updateJournal(id, patch),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.journal }),
  });
}

export function useDeleteJournal() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.deleteJournal(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.journal }),
  });
}

export function useGenerateSuggestions() {
  const invalidate = useInvalidateAll();
  return useMutation({ mutationFn: () => api.generateSuggestions(), onSuccess: invalidate });
}

export function useAcceptSuggestion() {
  const invalidate = useInvalidateAll();
  return useMutation({ mutationFn: (id: number) => api.acceptSuggestion(id), onSuccess: invalidate });
}

export function useDismissSuggestion() {
  const invalidate = useInvalidateAll();
  return useMutation({ mutationFn: (id: number) => api.dismissSuggestion(id), onSuccess: invalidate });
}

// --- OpenAI (ChatGPT subscription) connection -------------------------------

export function useOpenAIStatus(pollWhileConnecting = false) {
  return useQuery({
    queryKey: keys.openaiStatus,
    queryFn: api.openaiStatus,
    refetchInterval: pollWhileConnecting ? 2000 : false,
  });
}

export function useConnectOpenAI() {
  return useMutation({ mutationFn: () => api.openaiConnect() });
}

export function useImportCodex() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.openaiImportCodex(),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.openaiStatus }),
  });
}

export function useDisconnectOpenAI() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.openaiDisconnect(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.openaiStatus });
      qc.invalidateQueries({ queryKey: ["suggestions"] });
      qc.invalidateQueries({ queryKey: ["dashboard"] });
    },
  });
}

// --- tools (guided instruments) ---------------------------------------------

export function useTools() {
  return useQuery({ queryKey: ["tools"], queryFn: () => api.listTools() });
}

export function useToolEntries(key: string) {
  return useQuery({ queryKey: ["tool-entries", key], queryFn: () => api.toolEntries(key) });
}

export function useMoodAssist() {
  return useMutation({
    mutationFn: (body: {
      mode: "distortions" | "responses";
      event: string;
      thought: string;
      distortions: string[];
    }) => api.toolAssist("daily_mood_log", body),
  });
}

export function useCompleteTool(key: string) {
  const invalidate = useInvalidateAll();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: MoodLog) => api.completeTool(key, data),
    onSuccess: () => {
      invalidate();
      qc.invalidateQueries({ queryKey: ["tool-entries", key] });
    },
  });
}

export function useOpenAIModels(enabled: boolean) {
  return useQuery({
    queryKey: ["openai-models"],
    queryFn: () => api.openaiModels().then((r) => r.models),
    enabled,
    staleTime: 5 * 60 * 1000,
  });
}

export function useSetOpenAIConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (cfg: { model?: string; effort?: string }) => api.openaiConfig(cfg),
    onSuccess: (status) => qc.setQueryData(keys.openaiStatus, status),
  });
}
