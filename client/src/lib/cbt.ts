// Dr. David Burns' Daily Mood Log vocabulary (TEAM-CBT): the negative-emotion
// groups and the ten cognitive distortions. Keys/codes match the backend;
// display names/blurbs are localized through i18n (resolved on access).
import { t } from "./i18n";
import type { MessageKey } from "./locales/en";

export interface EmotionGroup {
  key: string;
  readonly label: string;
  readonly also: string; // the other feelings in the group
}

const EMOTION_KEYS = [
  "sad",
  "anxious",
  "guilty",
  "inferior",
  "lonely",
  "embarrassed",
  "hopeless",
  "frustrated",
  "angry",
  "other",
] as const;

export const EMOTIONS: EmotionGroup[] = EMOTION_KEYS.map((key) => ({
  key,
  get label() {
    return t(`emotion.${key}` as MessageKey);
  },
  get also() {
    return t(`emotion.${key}.also` as MessageKey);
  },
}));

export const emotionLabel = (key: string) => EMOTIONS.find((e) => e.key === key)?.label ?? key;

export interface Distortion {
  code: string;
  readonly name: string;
  readonly blurb: string;
}

const DISTORTION_CODES = ["AON", "OG", "MF", "DP", "JC", "MAG", "ER", "SH", "LAB", "SB"] as const;

export const DISTORTIONS: Distortion[] = DISTORTION_CODES.map((code) => ({
  code,
  get name() {
    return t(`distortion.${code}` as MessageKey);
  },
  get blurb() {
    return t(`distortion.${code}.blurb` as MessageKey);
  },
}));

export const distortionName = (code: string) => DISTORTIONS.find((d) => d.code === code)?.name ?? code;
