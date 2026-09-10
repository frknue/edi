// The hero's pixel sheet — shared by PixelHero (2D SVG fallback) and Hero3D
// (voxel extrusion), so both render the same character. Legend:
//   . empty · h hair · s skin · e eye · b tunic · d boots/belt
//   w blade · g hilt · p shield · m helmet · c crown
import type { CosmeticSlot, EquippedCosmetic } from "./types";
import { palette } from "./themes";

export const SPRITE_W = 16;
export const SPRITE_H = 15;

export const baseMap = [
  "................",
  ".....hhhhhh.....",
  "....hhhhhhhh....",
  "....hssssssh....",
  "....hsessesh....",
  "....hssssssh....",
  ".....ssssss.....",
  "....bbbbbbbb....",
  "...sbbbbbbbbs...",
  "...sbbbdbbbbs...",
  "....bbbbbbbb....",
  "....dd....dd....",
  "....dd....dd....",
  "...ddd....ddd...",
  "................",
];

export const swordMap = [
  "..............w.",
  "..............w.",
  "..............w.",
  "..............w.",
  "..............w.",
  "..............w.",
  "..............w.",
  ".............gwg",
  "..............g.",
  "..............g.",
  "................",
  "................",
  "................",
  "................",
  "................",
];

export const shieldMap = [
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  ".pp.............",
  "pppp............",
  "pppp............",
  "pppp............",
  ".pp.............",
  "................",
  "................",
  "................",
  "................",
];

export const helmetMap = [
  "................",
  ".....mmmmmm.....",
  "....mmmmmmmm....",
  "....mm....mm....",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
];

export const crownMap = [
  ".....c..c..c....",
  ".....cccccc.....",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
  "................",
];

// Base colors of the sprite (skin/hair/boots never change with gear).
export const heroColors = {
  hair: "#8b5a2b",
  skin: "#ffd9a0",
  eye: "#0b1210",
  boots: "#123020",
  steel: "#cfd8dc",
  shieldBlue: "#34d0ff",
  helmSteel: "#b0bec5",
  crownGold: "#ffd700",
};

export function tunicColor(level: number): string {
  if (level >= 15) return palette().gold; // gold
  if (level >= 10) return "#b98aff"; // epic purple
  if (level >= 5) return "#34d0ff"; // rare blue
  return "#2fbf5f"; // starter green
}

// Look = one entry per slot: bought gear wins, otherwise the free level
// unlock the hero always had (sword Lv3, shield Lv6, helm Lv10, crown Lv15).
export type Look = Partial<Record<CosmeticSlot, EquippedCosmetic>>;

function levelPiece(slot: CosmeticSlot, key: string, shape: string, color: string, accent: string): EquippedCosmetic {
  return { slot, key, name: key, rarity: "common", shape, color, accent };
}

export function resolveLook(level: number, loadout: EquippedCosmetic[] | undefined): Look {
  const look: Look = {};
  if (level >= 3) look.weapon = levelPiece("weapon", "level_sword", "sword", heroColors.steel, palette().gold);
  if (level >= 6) look.offhand = levelPiece("offhand", "level_shield", "buckler", heroColors.shieldBlue, "#1b6f8a");
  if (level >= 10) look.head = levelPiece("head", "level_helm", "helm", heroColors.helmSteel, "#6b7b85");
  if (level >= 15) look.head = levelPiece("head", "level_crown", "crown", heroColors.crownGold, "#ff6b6b");
  for (const piece of loadout ?? []) look[piece.slot] = piece;
  return look;
}

// A stable key for memo/rebuild decisions.
export function lookKey(look: Look): string {
  return Object.values(look)
    .map((p) => `${p.slot}:${p.key}`)
    .sort()
    .join("|");
}
