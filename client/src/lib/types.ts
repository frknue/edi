// Mirrors the Go API JSON contract (server/internal/models). Keep in sync.

export type QuestType = "daily" | "weekly" | "main" | "side" | "boss" | "recovery";
export type Difficulty = "trivial" | "easy" | "medium" | "hard" | "boss";
export type QuestStatus = "active" | "completed" | "skipped" | "archived";

export interface User {
  id: number;
  name: string;
  created_at: string;
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
}

export interface Quest {
  id: number;
  title: string;
  description: string;
  type: QuestType;
  difficulty: Difficulty;
  status: QuestStatus;
  attribute_rewards: Record<string, number>;
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

export interface JournalEntry {
  id: number;
  mood: number;
  energy: number;
  notes: string;
  created_at: string;
}

export interface QuestInput {
  title: string;
  description?: string;
  type: QuestType;
  difficulty: Difficulty;
  attribute_rewards: Record<string, number>;
  due_date?: string | null;
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
}

export interface Dashboard {
  user: User;
  character: CharacterSummary;
  attributes: Attribute[];
  today_quests: Quest[];
  streak: Streak;
  recent_xp_events: XPEvent[];
  recommended_quest: Quest | null;
  daily_progress: DailyProgress;
  pending_suggestions: AgentSuggestion[];
}

export interface LevelUp {
  attribute_key: string;
  attribute_name: string;
  from_level: number;
  to_level: number;
}

export interface CompletionResult {
  completed_quest: Quest;
  xp_events: XPEvent[];
  level_ups: LevelUp[];
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
