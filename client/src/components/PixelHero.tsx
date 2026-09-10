// PixelHero: the character. A hand-drawn pixel knight rendered as SVG rects
// (crisp on the CRT), who VISIBLY grows with the player: tunic color upgrades
// by level band, a sword appears at Lv 3, a shield at Lv 6, a helmet at
// Lv 10, a crown at Lv 15, and earned titles give an aura. Idle bob + blink
// by default; "celebrate" jumps, "crit" shakes, "focus" leans in and works
// (active quest mode) — see index.css keyframes.

import { palette } from "../lib/themes";
import { t } from "../lib/i18n";
import type { EquippedCosmetic } from "../lib/types";
import {
  SPRITE_H as H,
  SPRITE_W as W,
  baseMap,
  crownMap,
  helmetMap,
  heroColors,
  resolveLook,
  shieldMap,
  swordMap,
  tunicColor,
} from "../lib/heroSprite";

export function PixelHero({
  level,
  titled = false,
  mood = "idle",
  size = 72,
  loadout,
}: {
  level: number;
  titled?: boolean;
  mood?: "idle" | "celebrate" | "crit" | "focus" | "camp";
  size?: number;
  loadout?: EquippedCosmetic[];
}) {
  // Bought gear recolors the matching layer; the 3D hero renders the real
  // shapes, this fallback keeps the silhouette and borrows the colors.
  const look = resolveLook(level, loadout);
  const colors: Record<string, string> = {
    h: heroColors.hair,
    s: heroColors.skin,
    e: heroColors.eye,
    b: look.body?.color ?? tunicColor(level),
    d: heroColors.boots,
    w: look.weapon?.color ?? heroColors.steel,
    g: look.weapon?.accent ?? palette().gold,
    p: look.offhand?.color ?? heroColors.shieldBlue,
    m: look.head?.color ?? heroColors.helmSteel,
    c: look.head?.color ?? heroColors.crownGold,
  };

  const layers: string[][] = [baseMap];
  if (look.weapon) layers.push(swordMap);
  if (look.offhand) layers.push(shieldMap);
  if (look.head) layers.push(look.head.shape === "crown" ? crownMap : helmetMap);

  // Later layers overwrite earlier pixels (helmet over hair).
  const grid: string[][] = Array.from({ length: H }, () => Array(W).fill("."));
  for (const map of layers) {
    for (let y = 0; y < H; y++) {
      for (let x = 0; x < W; x++) {
        const ch = map[y]?.[x] ?? ".";
        if (ch !== ".") grid[y][x] = ch;
      }
    }
  }

  const moodClass =
    mood === "celebrate"
      ? "hero-celebrate"
      : mood === "crit"
        ? "hero-crit"
        : mood === "focus"
          ? "hero-focus"
          : mood === "camp"
            ? "hero-camp"
            : "hero-idle";

  return (
    <div className={moodClass} style={{ width: size, height: (size * H) / W, position: "relative" }} data-testid="pixel-hero">
      {titled && (
        <div
          className="hero-aura"
          style={{
            position: "absolute",
            inset: "-12%",
            borderRadius: "50%",
            background: "radial-gradient(circle, rgba(var(--gold-rgb),0.35), transparent 65%)",
          }}
        />
      )}
      <svg
        viewBox={`0 0 ${W} ${H}`}
        width={size}
        height={(size * H) / W}
        style={{ imageRendering: "pixelated", shapeRendering: "crispEdges", position: "relative" }}
        aria-label={t("hero.alt", { level })}
        role="img"
      >
        {grid.flatMap((row, y) =>
          row.map((ch, x) =>
            ch === "." ? null : (
              <rect
                key={`${x}-${y}`}
                x={x}
                y={y}
                width={1}
                height={1}
                fill={colors[ch]}
                className={ch === "e" ? "hero-eye" : undefined}
              />
            ),
          ),
        )}
      </svg>
    </div>
  );
}
