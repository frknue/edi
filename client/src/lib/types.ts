// Mirrors the Go API JSON contract (server/internal/models). Keep in sync.

export type QuestType = "daily" | "weekly" | "main" | "side" | "boss" | "recovery";
export type Difficulty = "trivial" | "easy" | "medium" | "hard" | "boss";
export type QuestStatus = "active" | "completed" | "skipped" | "archived";

export interface User {
  id: number;
  name: string;
  created_at: string;
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
  created_at: string;
  completed_at: string | null;
  due_date: string | null;
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

export interface DailyProgress {
  completed_today: number;
  goal: number;
  ratio: number;
  next_combo_multiplier: number;
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
  decayed_today: number;
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
  item_key: string;
  attribute: string; // "" = all
  percent: number;
  expires_at: string;
}

export interface CompletionResult {
  completed_quest: Quest;
  xp_events: XPEvent[];
  level_ups: LevelUp[];
  gold: number;
  crit: boolean;
  combo_multiplier: number;
  drop?: ItemDrop;
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
