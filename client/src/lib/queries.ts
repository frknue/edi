import {
  useMutation,
  useMutationState,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "./api";
import type { ChatResult, MoodLog, QuestInput, ShopItemInput, SupplementInput, TelegramPushTimes } from "./types";

export const keys = {
  dashboard: ["dashboard"] as const,
  attributes: ["attributes"] as const,
  quests: (filters?: { type?: string; status?: string }) => ["quests", filters ?? {}] as const,
  journal: ["journal"] as const,
  suggestions: (status?: string) => ["suggestions", status ?? "all"] as const,
  xpEvents: ["xp-events"] as const,
  openaiStatus: ["openai-status"] as const,
  shop: ["shop"] as const,
  goldEvents: ["gold-events"] as const,
  multiplayer: ["multiplayer"] as const,
  supplements: ["supplements"] as const,
  supplementHistory: ["supplements", "history"] as const,
};

export function useDashboard() {
  return useQuery({ queryKey: keys.dashboard, queryFn: api.getDashboard });
}

export function useCreateAccountInvite() {
  return useMutation({ mutationFn: api.createAccountInvite });
}

export function useMultiplayerStatus() {
  return useQuery({ queryKey: keys.multiplayer, queryFn: api.multiplayerStatus });
}

function useInvalidateMultiplayer() {
  const qc = useQueryClient();
  return () => {
    qc.invalidateQueries({ queryKey: keys.multiplayer });
    qc.invalidateQueries({ queryKey: ["quests"] });
    qc.invalidateQueries({ queryKey: ["dashboard"] });
  };
}

export function useCreateQuestBoard() {
  const invalidate = useInvalidateMultiplayer();
  return useMutation({ mutationFn: (name: string) => api.createQuestBoard(name), onSuccess: invalidate });
}

export function useCreateQuestBoardInvite() {
  return useMutation({ mutationFn: api.createQuestBoardInvite });
}

export function useJoinQuestBoard() {
  const invalidate = useInvalidateMultiplayer();
  return useMutation({ mutationFn: (code: string) => api.joinQuestBoard(code), onSuccess: invalidate });
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
    qc.invalidateQueries({ queryKey: ["gold-events"] });
    qc.invalidateQueries({ queryKey: ["achievements"] });
    qc.invalidateQueries({ queryKey: ["items"] });
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

export function useRecordSpontaneousQuest() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: (input: QuestInput) => api.recordSpontaneousQuest(input),
    onSuccess: invalidate,
  });
}

// Drafting only proposes form values — nothing changes server-side, so there is
// nothing to invalidate. Failures surface through the global mutation toast.
export function useDraftQuest() {
  return useMutation({
    mutationFn: (body: { title: string; description: string }) => api.draftQuest(body),
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

// Remote servers can't receive the localhost OAuth redirect — the user pastes
// the URL they landed on and this finishes the exchange server-side.
export function useCompleteOpenAIConnect() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (callbackUrl: string) => api.openaiConnectComplete(callbackUrl),
    onSuccess: (status) => qc.setQueryData(keys.openaiStatus, status),
  });
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

export function useAchievements() {
  return useQuery({ queryKey: ["achievements"], queryFn: api.listAchievements });
}

export function useItems() {
  return useQuery({ queryKey: ["items"], queryFn: api.listItems });
}

// --- telegram presence --------------------------------------------------------

export function useTelegramStatus() {
  return useQuery({ queryKey: ["telegram-status"], queryFn: api.telegramStatus });
}

export function useTelegramPairCode() {
  return useMutation({ mutationFn: () => api.telegramPairCode() });
}

export function useTelegramPushTimes(enabled = true) {
  return useQuery({ queryKey: ["telegram-push-times"], queryFn: api.telegramPushTimes, enabled });
}

export function useSetTelegramPushTimes() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: Partial<TelegramPushTimes>) => api.setTelegramPushTimes(patch),
    onSuccess: (data) => qc.setQueryData(["telegram-push-times"], data),
  });
}

// --- story mode ----------------------------------------------------------------

export function useStory() {
  return useMutation({ mutationFn: () => api.story() });
}

export function useForgeBoss() {
  const invalidate = useInvalidateAll();
  return useMutation({ mutationFn: () => api.forgeBoss(), onSuccess: invalidate });
}

export function useTelegramUnlink() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.telegramUnlink(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["telegram-status"] }),
  });
}

// --- agent chat ---------------------------------------------------------------

export interface ChatVars {
  message: string;
  session: string;
  reset: boolean;
  clientId?: string; // the caller's id for the optimistic user bubble (not sent)
}

const chatKey = ["agent-chat"] as const;

// useChat sends one free-text message. Callbacks are hook-level so they still
// fire when the Agent page was unmounted mid-reply (tab switch); the caller
// persists the transcript itself. A reply can have moved anything (quests,
// supplements, gold…), so every query is invalidated afterwards.
export function useChat(opts?: {
  onSuccess?: (res: ChatResult, vars: ChatVars) => void;
  onError?: (err: Error, vars: ChatVars) => void;
}) {
  const qc = useQueryClient();
  return useMutation({
    mutationKey: chatKey,
    mutationFn: (vars: ChatVars) => api.chat(vars.message, vars.session, vars.reset),
    onSuccess: (res, vars) => {
      opts?.onSuccess?.(res, vars);
      if (res.tools_used.length > 0) qc.invalidateQueries();
    },
    onError: (err, vars) => opts?.onError?.(err, vars),
  });
}

// True while any chat request is in flight (survives remounts of the page).
export function useChatPending(): boolean {
  return useMutationState({ filters: { mutationKey: chatKey, status: "pending" } }).length > 0;
}

// --- supplements (daily stack) ----------------------------------------------

export function useSupplements() {
  return useQuery({ queryKey: keys.supplements, queryFn: api.listSupplements });
}

export function useSupplementHistory(days = 70) {
  return useQuery({ queryKey: [...keys.supplementHistory, days], queryFn: () => api.supplementHistory(days) });
}

function useInvalidateSupplements() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: keys.supplements });
}

export function useAddSupplement() {
  const invalidate = useInvalidateSupplements();
  return useMutation({ mutationFn: (input: SupplementInput) => api.addSupplement(input), onSuccess: invalidate });
}

export function useUpdateSupplement() {
  const invalidate = useInvalidateSupplements();
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: SupplementInput }) => api.updateSupplement(id, input),
    onSuccess: invalidate,
  });
}

export function useArchiveSupplement() {
  const invalidate = useInvalidateSupplements();
  return useMutation({ mutationFn: (id: number) => api.archiveSupplement(id), onSuccess: invalidate });
}

// Taking awards XP (and maybe the full-stack bonus) — refresh everything the
// dashboard shows, like a quest completion does.
export function useTakeSupplement() {
  const invalidateAll = useInvalidateAll();
  const invalidateSupps = useInvalidateSupplements();
  return useMutation({
    mutationFn: (id: number) => api.takeSupplement(id),
    onSuccess: () => {
      invalidateAll();
      invalidateSupps();
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

// --- gold economy / reward shop ----------------------------------------------

export function useShopItems() {
  return useQuery({ queryKey: keys.shop, queryFn: api.listShop });
}

export function useGoldEvents(limit = 30, source?: string) {
  return useQuery({
    queryKey: [...keys.goldEvents, limit, source ?? "all"],
    queryFn: () => api.listGoldEvents(limit, source),
  });
}

export function useCreateShopItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ShopItemInput) => api.createShopItem(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.shop }),
  });
}

export function useUpdateShopItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }: { id: number; patch: { name?: string; price?: number } }) =>
      api.updateShopItem(id, patch),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.shop }),
  });
}

export function useArchiveShopItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.archiveShopItem(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.shop }),
  });
}

export function usePurchaseShopItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.purchaseShopItem(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["dashboard"] });
      qc.invalidateQueries({ queryKey: ["gold-events"] });
    },
  });
}

export function useWardAttribute() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (key: string) => api.wardAttribute(key),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["dashboard"] });
      qc.invalidateQueries({ queryKey: ["attributes"] });
      qc.invalidateQueries({ queryKey: ["gold-events"] });
    },
  });
}

export function useSetRestMode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (on: boolean) => api.setRestMode(on),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["dashboard"] });
      qc.invalidateQueries({ queryKey: ["attributes"] });
    },
  });
}
