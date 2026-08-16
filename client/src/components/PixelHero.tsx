// PixelHero: the character. A hand-drawn pixel knight rendered as SVG rects
// (crisp on the CRT), who VISIBLY grows with the player: tunic color upgrades
// by level band, a sword appears at Lv 3, a shield at Lv 6, a helmet at
// Lv 10, a crown at Lv 15, and earned titles give an aura. Idle bob + blink
// by default; "celebrate" jumps, "crit" shakes (see index.css keyframes).

const W = 16;
const H = 15;

// Legend: . empty · h hair · s skin · e eye · b tunic · d boots/belt
//         w blade · g hilt · p shield · m helmet · c crown
const baseMap = [
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

const swordMap = [
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

const shieldMap = [
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

const helmetMap = [
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

const crownMap = [
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

function tunicColor(level: number): string {
  if (level >= 15) return "#ffb000"; // gold
  if (level >= 10) return "#b98aff"; // epic purple
  if (level >= 5) return "#34d0ff"; // rare blue
  return "#2fbf5f"; // starter green
}

export function PixelHero({
  level,
  titled = false,
  mood = "idle",
  size = 72,
}: {
  level: number;
  titled?: boolean;
  mood?: "idle" | "celebrate" | "crit";
  size?: number;
}) {
  const colors: Record<string, string> = {
    h: "#8b5a2b",
    s: "#ffd9a0",
    e: "#0b1210",
    b: tunicColor(level),
    d: "#123020",
    w: "#cfd8dc",
    g: "#ffb000",
    p: "#34d0ff",
    m: "#b0bec5",
    c: "#ffd700",
  };

  const layers: string[][] = [baseMap];
  if (level >= 3) layers.push(swordMap);
  if (level >= 6) layers.push(shieldMap);
  if (level >= 10) layers.push(helmetMap);
  if (level >= 15) layers.push(crownMap);

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

  const moodClass = mood === "celebrate" ? "hero-celebrate" : mood === "crit" ? "hero-crit" : "hero-idle";

  return (
    <div className={moodClass} style={{ width: size, height: (size * H) / W, position: "relative" }} data-testid="pixel-hero">
      {titled && (
        <div
          className="hero-aura"
          style={{
            position: "absolute",
            inset: "-12%",
            borderRadius: "50%",
            background: "radial-gradient(circle, rgba(255,176,0,0.35), transparent 65%)",
          }}
        />
      )}
      <svg
        viewBox={`0 0 ${W} ${H}`}
        width={size}
        height={(size * H) / W}
        style={{ imageRendering: "pixelated", shapeRendering: "crispEdges", position: "relative" }}
        aria-label={`Pixel hero, level ${level}`}
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
