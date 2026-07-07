// Dr. David Burns' Daily Mood Log vocabulary (TEAM-CBT): the negative-emotion
// groups and the ten cognitive distortions. Keys/codes match the backend.

export interface EmotionGroup {
  key: string;
  label: string;
  also: string; // the other feelings in the group
}

export const EMOTIONS: EmotionGroup[] = [
  { key: "sad", label: "Sad", also: "blue, depressed, down, unhappy" },
  { key: "anxious", label: "Anxious", also: "worried, panicky, nervous, frightened" },
  { key: "guilty", label: "Guilty", also: "remorseful, bad, ashamed" },
  { key: "inferior", label: "Inferior", also: "worthless, inadequate, incompetent" },
  { key: "lonely", label: "Lonely", also: "unloved, unwanted, rejected, alone" },
  { key: "embarrassed", label: "Embarrassed", also: "foolish, humiliated, self-conscious" },
  { key: "hopeless", label: "Hopeless", also: "discouraged, pessimistic, despairing" },
  { key: "frustrated", label: "Frustrated", also: "stuck, thwarted, defeated" },
  { key: "angry", label: "Angry", also: "mad, resentful, annoyed, irritated, furious" },
  { key: "other", label: "Other", also: "another feeling" },
];

export const emotionLabel = (key: string) => EMOTIONS.find((e) => e.key === key)?.label ?? key;

export interface Distortion {
  code: string;
  name: string;
  blurb: string;
}

export const DISTORTIONS: Distortion[] = [
  { code: "AON", name: "All-or-Nothing", blurb: "You see things in absolute, black-and-white categories." },
  { code: "OG", name: "Overgeneralization", blurb: "You view one event as a never-ending pattern of defeat." },
  { code: "MF", name: "Mental Filter", blurb: "You dwell on the negatives and filter out the positives." },
  { code: "DP", name: "Discounting the Positive", blurb: "You insist your positive qualities don't count." },
  { code: "JC", name: "Jumping to Conclusions", blurb: "Mind-reading or fortune-telling without the facts." },
  { code: "MAG", name: "Magnification / Minimization", blurb: "You blow things out of proportion, or shrink them." },
  { code: "ER", name: "Emotional Reasoning", blurb: "You reason from feelings: “I feel it, so it must be true.”" },
  { code: "SH", name: "Should Statements", blurb: "You use shoulds, shouldn'ts, musts, oughts, have-tos." },
  { code: "LAB", name: "Labeling", blurb: "You call yourself a name instead of noting a mistake." },
  { code: "SB", name: "Self- / Other-Blame", blurb: "You find fault instead of solving the problem." },
];

export const distortionName = (code: string) => DISTORTIONS.find((d) => d.code === code)?.name ?? code;
