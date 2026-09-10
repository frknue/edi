// Hero: the character, 3D when the device can (WebGL), the pixel sprite
// otherwise (or when `localStorage edi.hero3d = "0"` opts out). A
// reduced-motion preference keeps the 3D hero but stills the animation
// (Hero3D reads it itself). The 3D chunk (three.js) is loaded lazily so
// the first paint never waits for it; the sprite is the Suspense fallback.
import { Suspense, lazy } from "react";
import { PixelHero } from "./PixelHero";
import type { Hero3DProps } from "./Hero3D";

const Hero3D = lazy(() => import("./Hero3D"));

let supported: boolean | null = null;

export function supports3D(): boolean {
  if (supported !== null) return supported;
  try {
    if (localStorage.getItem("edi.hero3d") === "0") return (supported = false);
  } catch {
    // ignore
  }
  try {
    const canvas = document.createElement("canvas");
    const gl = canvas.getContext("webgl2") ?? canvas.getContext("webgl");
    supported = !!gl;
    const ext = (gl as WebGLRenderingContext | null)?.getExtension("WEBGL_lose_context");
    ext?.loseContext();
  } catch {
    supported = false;
  }
  return supported;
}

export function Hero(props: Hero3DProps) {
  const fallback = (
    <PixelHero
      level={props.level}
      titled={props.titled}
      mood={props.mood}
      size={Math.round((props.size ?? 96) * 0.7)}
      loadout={props.loadout}
    />
  );
  if (!supports3D()) {
    return (
      <div style={{ width: props.size ?? 96, height: props.size ?? 96 }} className={`grid place-items-center ${props.className ?? ""}`}>
        {fallback}
      </div>
    );
  }
  return (
    <Suspense
      fallback={
        <div style={{ width: props.size ?? 96, height: props.size ?? 96 }} className={`grid place-items-center ${props.className ?? ""}`}>
          {fallback}
        </div>
      }
    >
      <Hero3D {...props} />
    </Suspense>
  );
}
