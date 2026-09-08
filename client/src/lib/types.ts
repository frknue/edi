// Mirrors the Go API JSON contract (server/internal/models). Keep in sync.

export type QuestType = "daily" | "weekly" | "main" | "side" | "boss" | "recovery";
export type Difficulty = "trivial" | "easy" | "medium" | "hard" | "boss";
export type QuestStatus = "active" | "completed" | "skipped" | "archived";

export interface User {
  id: number;
  name: string;
  created_at: string;
}

export interface QuestBoardMember {
  user_id: number;
  name: string;
}

export interface QuestBoard {
  id: number;
  name: string;
  members: QuestBoardMember[];
  created_at: string;
}

export interface MultiplayerStatus {
  board: QuestBoard | null;
}

export interface QuestBoardInvite {
  code: string;
  expires_at: string;
}

export interface AttributeDecay {
  state: "fresh" | "grace" | "decaying" | "warded" | "rest";
  idle_days: number;
  warded_until?: string;
  projected_daily_loss: number;
  floor_level: number;
}

export interface Ward {
  id: number;
  attribute_key: string;
  expires_at: string;
  created_at: string;
}

export interface WardResult {
  ward: Ward;
  balance: number;
}

export interface RestState {
  on: boolean;
  since?: string;
}

// The opt-in punishment layer (decay, daily stakes, wards). Off by default.
export interface HardcoreState {
  on: boolean;
  since?: string;
}

// One run of a quest in active mode. Presence only — never XP.
export interface QuestSession {
  id: number;
  quest_id: number;
  title: string;
  quest_type: QuestType;
  attribute_rewards: Record<string, number>;
  resume_note: string;
  started_at: string;
  ended_at?: string;
  reason: "" | "stopped" | "completed" | "switched" | "expired";
  note: string;
  elapsed_seconds: number;
  running: boolean;
}

// One local day of the 14-day activity strip on the dashboard.
export interface ActiveDay {
  day: string; // YYYY-MM-DD
  active: boolean;
  today: boolean;
}

export interface Attribute {
  id: number;
  key: string;
  name: string;
  total_xp: number;
  level: number;
  xp_into_level: number;
  xp_for_next_level: number;
  progress: number; // 0..1
  decay?: AttributeDecay;
}

export interface Subtask {
  id: number;
  quest_id: number;
  title: string;
  attribute_rewards: Record<string, number>;
  done: boolean;
}

export interface SubtaskInput {
  title: string;
  attribute_rewards: Record<string, number>;
}

export interface Quest {
  id: number;
  title: string;
  description: string;
  type: QuestType;
  difficulty: Difficulty;
  status: QuestStatus;
  attribute_rewards: Record<string, number>;
  subtasks: Subtask[];
  skip_count: number;
  resume_note: string; // "next physical action" captured at Stop; cleared on completion
  trigger: string; // if-then cue ("after coffee")
  trigger_at: string; // HH:MM local anchor, "" = none
  projected_xp?: number; // dashboard only: what completing it pays right now
  recommend_reason?: RecommendReason; // dashboard only, on the recommended quest
  created_at: string;
  completed_at: string | null;
  due_date: string | null;
  shared_quest_id?: number;
  assignees: QuestAssignee[];
  assigned_to_me: boolean;
  my_status?: QuestStatus;
  all_completed: boolean;
}

export interface QuestAssignee {
  user_id: number;
  name: string;
  status: QuestStatus;
  completed_at: string | null;
}

export interface XPEvent {
  id: number;
  attribute_key: string;
  attribute_name?: string;
  amount: number;
  source: string;
  source_id?: number;
  note?: string;
  created_at: string;
}

export interface Streak {
  current: number;
  longest: number;
  last_active_date: string | null;
  last_mend_date?: string; // a one-day gap was bridged for free on this day
}

export interface ShopItem {
  id: number;
  name: string;
  price: number;
  created_at: string;
  archived_at?: string;
}

export interface ShopItemInput {
  name: string;
  price: number;
}

export interface GoldEvent {
  id: number;
  amount: number; // positive = mint, negative = purchase
  source: string; // quest, subtask, tool, journal, purchase, grant
  label?: string;
  shop_item_id?: number;
  created_at: string;
}

export interface PurchaseResult {
  item: ShopItem;
  event: GoldEvent;
  balance: number;
}

export interface JournalEntry {
  id: number;
  mood: number;
  energy: number;
  notes: string;
  created_at: string;
}

// First entry of a day awards XP; later ones return empty xp_events.
export interface JournalCreateResult {
  entry: JournalEntry;
  xp_events: XPEvent[];
  level_ups: LevelUp[];
  gold: number;
}

export interface QuestInput {
  title: string;
  description?: string;
  type: QuestType;
  difficulty: Difficulty;
  attribute_rewards: Record<string, number>;
  subtasks?: SubtaskInput[];
  due_date?: string | null;
  trigger?: string;
  trigger_at?: string;
  assignee_ids?: number[];
}

export interface StoryChapter {
  id: number;
  number: number;
  text: string;
  created_at: string;
}

export interface PartnerSession {
  name: string;
  title: string;
  elapsed_seconds: number;
}

// What the AI proposes for a half-typed quest. A suggestion only — the user
// still edits and submits the form.
export interface User {
  id: number;
  name: string;
  is_admin: boolean;
  created_at: string;
}

// Pre-auth server discovery: does it want a token, can you sign up?
export interface AuthConfig {
  auth_required: boolean;
  registration_open: boolean;
}

export interface AccountInvite {
  code: string;
  expires_at: string;
}

// Returned once at signup/rotation — the only time the token is visible.
export interface CreatedUser {
  user: User;
  token: string;
}

export interface TelegramStatus {
  configured: boolean;
  linked: boolean;
  bot_username: string;
}

// Per-user push times (HH:MM, server-local); "" = server default.
export interface TelegramPushTimes {
  briefing: string;
  nudge: string;
}

export interface TelegramPairCode {
  code: string;
  bot_username: string;
  expires_at: string;
}

export interface QuestDraft {
  type: QuestType;
  difficulty: Difficulty;
  attribute_rewards: Record<string, number>;
  reason: string;
}

export interface AgentSuggestion {
  id: number;
  type: string;
  title: string;
  reason: string;
  suggested_quest: QuestInput;
  status: "pending" | "accepted" | "dismissed";
  created_quest_id?: number;
  source_quest_id?: number;
  created_at: string;
  resolved_at?: string;
}

export interface CharacterSummary {
  title: string;
  name: string;
  level: number;
  total_xp: number;
  xp_into_level: number;
  xp_for_next_level: number;
  progress: number;
}

export type RecommendReason = "first_move" | "near_level" | "buff" | "combo" | "weakest" | "default";

export interface DailyProgress {
  completed_today: number; // every completion (drives the combo chain)
  goal: number; // dailies on the board (min 1)
  dailies_done: number; // the closable set
  cleared: boolean; // today's set is closed — camp
  ratio: number;
  next_combo_multiplier: number;
}

export interface LootPity {
  dropless: number;
  guaranteed_after: number;
}

export interface FirstMove {
  day: string; // YYYY-MM-DD
  quest: Quest;
}

export interface Dashboard {
  user: User;
  character: CharacterSummary;
  attributes: Attribute[];
  today_quests: Quest[];
  streak: Streak;
  gold_balance: number;
  rest_mode: boolean;
  rest_since?: string;
  hardcore: boolean;
  decayed_today: number;
  daily_penalty_xp: number;
  active_days: ActiveDay[];
  active_session: QuestSession | null;
  day_state: "open" | "camp";
  xp_today: number;
  board_clear_today: boolean;
  loot_pity: LootPity;
  first_move: FirstMove | null;
  partner_session: PartnerSession | null;
  latest_chapter: StoryChapter | null;
  recent_xp_events: XPEvent[];
  recommended_quest: Quest | null;
  daily_progress: DailyProgress;
  pending_suggestions: AgentSuggestion[];
  active_buffs: ActiveBuff[];
}

export interface LevelUp {
  attribute_key: string;
  attribute_name: string;
  from_level: number;
  to_level: number;
}

// Loot: a completion may drop a trophy, a temporal XP buff, or a gold cache.
export interface Achievement {
  key: string;
  name: string;
  desc: string;
  icon: string;
  title?: string;
  earned: boolean;
  awarded_at?: string;
}

export interface ItemDrop {
  id: number;
  key: string;
  name: string;
  icon: string;
  rarity: "common" | "uncommon" | "rare" | "epic" | "legendary";
  kind: "trophy" | "buff" | "gold";
  flavor: string;
  percent?: number;
  attribute?: string;
  gold?: number;
  expires_at?: string;
  dropped_at?: string;
}

export interface ActiveBuff {
  id: number;
  item_key: string;
  attribute: string; // "" = all
  percent: number;
  expires_at: string;
  uses_left?: number; // undefined = unlimited (legacy drop)
}

export interface CompletionResult {
  completed_quest: Quest;
  xp_events: XPEvent[];
  level_ups: LevelUp[];
  gold: number;
  crit: boolean;
  combo_multiplier: number;
  drop?: ItemDrop;
  board_clear: boolean; // this completion closed today's set (bonus paid)
  achievements_unlocked: Achievement[];
  dashboard: Dashboard;
}

export interface OpenAIStatus {
  connected: boolean;
  email?: string;
  account_id?: string;
  model?: string;
  effort?: string;
  effort_options?: string[];
  expires_at?: string;
}

export interface ToolDefinition {
  key: string;
  name: string;
  tagline: string;
  description: string;
  category: string;
  attribute_rewards: Record<string, number>;
}

export interface MoodEmotion {
  category: string;
  before: number;
  after: number;
}

export interface MoodThought {
  thought: string;
  belief_before: number;
  distortions: string[];
  positive_thought: string;
  positive_belief: number;
  belief_after: number;
}

export interface MoodLog {
  event: string;
  emotions: MoodEmotion[];
  thoughts: MoodThought[];
}

export interface ToolEntry {
  id: number;
  tool_key: string;
  data: MoodLog;
  xp_awarded: number;
  summary?: string;
  created_at: string;
}

export interface ToolCompletionResult {
  entry: ToolEntry;
  xp_events: XPEvent[];
  level_ups: LevelUp[];
  gold: number;
  dashboard: Dashboard;
}

export interface MoodDistortionHit {
  code: string;
  why: string;
}

export interface MoodResponseIdea {
  technique: string;
  text: string;
}

export interface MoodAssistResult {
  mode: string;
  distortions?: MoodDistortionHit[];
  responses?: MoodResponseIdea[];
  crisis: boolean;
  crisis_message?: string;
}

export interface OpenAIModel {
  slug: string;
  display_name: string;
  description?: string;
  efforts: string[];
  default_effort?: string;
}

// --- supplements (daily stack) ------------------------------------------------

export interface Supplement {
  id: number;
  name: string;
  dose: string;
  sort_order: number;
  created_at: string;
  taken: boolean;
  taken_at?: string;
}

export interface SupplementInput {
  name: string;
  dose: string;
}

export interface SupplementsToday {
  day: string;
  supplements: Supplement[];
  taken: number;
  total: number;
  all_taken: boolean;
  bonus_awarded: boolean;
  item_rewards: Record<string, number>;
  bonus_rewards: Record<string, number>;
}

export interface SupplementIntake {
  id: number;
  supplement_id: number;
  name: string;
  day: string;
  xp_awarded: number;
  bonus: boolean;
  created_at: string;
}

export interface SupplementTakeResult {
  intake: SupplementIntake;
  bonus_awarded: boolean;
  xp_events: XPEvent[];
  level_ups: LevelUp[];
  gold: number;
  today: SupplementsToday;
  dashboard: Dashboard;
}

export interface SupplementDay {
  day: string;
  taken: number;
  bonus: boolean;
  xp: number;
  names: string[];
}

// --- agent chat (free text over the same tool registry) -------------------------

export interface ChatResult {
  reply: string;
  tools_used: string[];
}

// One visible line of the web transcript (kept per user in localStorage; the
// server keeps its own in-memory history for the model).
export interface ChatMessage {
  id: string;
  role: "user" | "agent";
  text: string;
  tools?: string[];
  at: string;
}
