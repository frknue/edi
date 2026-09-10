// Hero3D: the character as a voxel figure rendered with three.js. The pixel
// sheet in lib/heroSprite.ts is extruded into boxes (so the 2D fallback and
// the 3D hero are the same person), then gear from the loadout is built as
// real geometry per slot (helm, sword, cape, aura ring, companion…).
//
// Moods map 1:1 to the CSS moods of PixelHero: idle (bob + sway + blink),
// celebrate (jump + spin + particle burst), crit (shake + red flash), focus
// (lean in, weapon swing), camp (slow breathing, warm dim light). The
// wardrobe passes interactive=true for drag-to-rotate.
//
// prefers-reduced-motion keeps the figure but stills it: no bob, spin,
// shake, blink or particles — the hero just stands there, wearing the gear.
//
// Loaded lazily (see Hero.tsx); everything is disposed on unmount so the
// reward overlay can mount/unmount freely without leaking WebGL contexts.

import { useEffect, useRef } from "react";
import {
  ACESFilmicToneMapping,
  BoxGeometry,
  CircleGeometry,
  Color,
  ConeGeometry,
  CylinderGeometry,
  DirectionalLight,
  Group,
  HemisphereLight,
  InstancedMesh,
  Material,
  Matrix4,
  Mesh,
  MeshStandardMaterial,
  Object3D,
  PerspectiveCamera,
  PointLight,
  Quaternion,
  Scene,
  SphereGeometry,
  TorusGeometry,
  Vector3,
  WebGLRenderer,
} from "three";
import type { EquippedCosmetic } from "../lib/types";
import { SPRITE_H, SPRITE_W, baseMap, heroColors, resolveLook, tunicColor, type Look } from "../lib/heroSprite";
import { palette } from "../lib/themes";
import { t } from "../lib/i18n";

export type HeroMood = "idle" | "celebrate" | "crit" | "focus" | "camp";

export interface Hero3DProps {
  level: number;
  titled?: boolean;
  mood?: HeroMood;
  size?: number; // css px (square)
  loadout?: EquippedCosmetic[];
  preview?: EquippedCosmetic | null; // wardrobe try-on: overrides its slot
  interactive?: boolean; // drag to rotate + slow auto-turn
  frame?: "tight" | "wide"; // camera distance (wide leaves room for pet/aura)
  burst?: number; // bump to fire a particle burst (purchase / equip)
  burstColor?: string;
  className?: string;
}

// ---------------------------------------------------------------------------
// geometry helpers

const unitBox = new BoxGeometry(1, 1, 1);

function mat(color: string, opts: { metal?: number; rough?: number; emissive?: string; emissiveIntensity?: number; opacity?: number } = {}) {
  const m = new MeshStandardMaterial({
    color: new Color(color),
    metalness: opts.metal ?? 0.1,
    roughness: opts.rough ?? 0.75,
    flatShading: true,
  });
  if (opts.emissive) {
    m.emissive = new Color(opts.emissive);
    m.emissiveIntensity = opts.emissiveIntensity ?? 0.6;
  }
  if (opts.opacity !== undefined) {
    m.transparent = true;
    m.opacity = opts.opacity;
  }
  return m;
}

function box(w: number, h: number, d: number, color: string, opts?: Parameters<typeof mat>[1]): Mesh {
  const m = new Mesh(unitBox, mat(color, opts));
  m.scale.set(w, h, d);
  return m;
}

function at<T extends Object3D>(o: T, x: number, y: number, z: number): T {
  o.position.set(x, y, z);
  return o;
}

// Sprite → world: column x → x - 8 + 0.5, row y → 15 - y - 0.5 (feet ≈ y 1).
const px = (col: number) => col - SPRITE_W / 2 + 0.5;
const py = (row: number) => SPRITE_H - row - 0.5;

// Depth of the extrusion per body region (head is the roundest part).
function depthForRow(row: number): number {
  if (row <= 6) return 6; // head
  if (row <= 10) return 4.6; // torso + arms
  return 3.6; // legs
}

interface Rig {
  root: Group; // bob / spin / shake
  body: Group; // lean (focus)
  eyes: Mesh[];
  weapon: Group | null; // swing pivot
  cape: Group | null; // sway pivot (top edge)
  wings: Group[]; // flap pivots
  auraRing: Object3D | null;
  auraOrbiters: Object3D[];
  pet: Group | null;
  petKind: string;
  petWings: Group[];
  halo: Object3D | null;
  disposables: Array<{ dispose(): void }>;
}

function shade(hex: string, factor: number): string {
  const c = new Color(hex);
  c.multiplyScalar(factor);
  return `#${c.getHexString()}`;
}

function buildHero(level: number, look: Look, titled: boolean): Rig {
  const root = new Group();
  const body = new Group();
  root.add(body);
  const disposables: Array<{ dispose(): void }> = [];
  const track = <T extends Object3D>(o: T): T => {
    o.traverse((child) => {
      if (child instanceof Mesh) {
        const m = child.material as Material | Material[];
        (Array.isArray(m) ? m : [m]).forEach((mm) => disposables.push(mm));
        if (child.geometry !== unitBox) disposables.push(child.geometry);
      }
    });
    return o;
  };

  const bodyColor = look.body?.color ?? tunicColor(level);
  const bodyAccent = look.body?.accent ?? shade(bodyColor, 0.65);
  const bodyShape = look.body?.shape ?? "tunic";
  const gold = palette().gold;

  // --- voxels from the sprite sheet -------------------------------------
  const colorOf = (ch: string, row: number, col: number): string | null => {
    switch (ch) {
      case "h":
        return look.head && look.head.shape !== "crown" ? null : heroColors.hair; // hood/helm/hat hide the hair
      case "s":
        return heroColors.skin;
      case "e":
        return null; // eyes are separate meshes (they blink)
      case "b":
        if (bodyShape === "mail" && row % 2 === 0) return bodyAccent;
        if (bodyShape === "plate" && (col === 7 || col === 8) && row === 8) return bodyAccent;
        return bodyColor;
      case "d":
        return row >= 11 && bodyShape === "robe" ? bodyColor : heroColors.boots;
      default:
        return null;
    }
  };
  const voxels: Array<{ x: number; y: number; d: number; color: string }> = [];
  for (let row = 0; row < SPRITE_H; row++) {
    for (let col = 0; col < SPRITE_W; col++) {
      const color = colorOf(baseMap[row][col], row, col);
      if (color) voxels.push({ x: px(col), y: py(row), d: depthForRow(row), color });
    }
  }
  const voxelMat = mat("#ffffff", { rough: 0.8 });
  const inst = new InstancedMesh(unitBox, voxelMat, voxels.length);
  const m4 = new Matrix4();
  const q = new Quaternion();
  voxels.forEach((v, i) => {
    m4.compose(new Vector3(v.x, v.y, 0), q, new Vector3(1, 1, v.d));
    inst.setMatrixAt(i, m4);
    inst.setColorAt(i, new Color(v.color));
  });
  inst.instanceMatrix.needsUpdate = true;
  if (inst.instanceColor) inst.instanceColor.needsUpdate = true;
  disposables.push(voxelMat, inst);
  body.add(inst);

  // eyes: sit on the face (front of the head extrusion)
  const eyes: Mesh[] = [];
  for (const col of [6, 9]) {
    const e = box(0.9, 0.9, 0.5, heroColors.eye, { rough: 0.3 });
    at(e, px(col), py(4), depthForRow(4) / 2 + 0.05);
    eyes.push(e);
    body.add(track(e));
  }

  // --- head gear ----------------------------------------------------------
  if (look.head) {
    const { shape, color, accent } = look.head;
    const g = new Group();
    const headTop = py(1) + 0.5; // top of hair
    switch (shape) {
      case "hood": {
        g.add(at(box(9.2, 3.4, 7.2, color), 0, py(2), 0));
        g.add(at(box(9.2, 3.5, 2.2, color), 0, py(4.4), -2.6)); // back of the hood
        g.add(at(box(1.2, 2.2, 7.4, accent), -4.5, py(4), 0));
        g.add(at(box(1.2, 2.2, 7.4, accent), 4.5, py(4), 0));
        break;
      }
      case "helm": {
        g.add(at(box(9.2, 3, 7.2, color, { metal: 0.7, rough: 0.35 }), 0, py(2), 0));
        g.add(at(box(9.4, 0.8, 7.4, accent, { metal: 0.7, rough: 0.35 }), 0, py(3.3), 0)); // brow band
        g.add(at(box(1, 4, 7.4, color, { metal: 0.7, rough: 0.35 }), -4.6, py(4.5), 0)); // cheek guards
        g.add(at(box(1, 4, 7.4, color, { metal: 0.7, rough: 0.35 }), 4.6, py(4.5), 0));
        g.add(at(box(0.9, 3.4, 0.9, accent, { metal: 0.6, rough: 0.4 }), 0, headTop + 1.6, 0)); // plume stub
        break;
      }
      case "hat": {
        const brim = new Mesh(new CylinderGeometry(6.4, 6.4, 0.5, 8), mat(color));
        g.add(at(brim, 0, headTop - 0.2, 0));
        const cone = new Mesh(new ConeGeometry(4.2, 7.5, 8), mat(color));
        cone.rotation.z = -0.18;
        g.add(at(cone, 0.4, headTop + 3.5, 0));
        const band = new Mesh(new CylinderGeometry(4.3, 4.6, 0.9, 8), mat(accent, { emissive: accent, emissiveIntensity: 0.25 }));
        g.add(at(band, 0, headTop + 0.4, 0));
        break;
      }
      case "horned": {
        g.add(at(box(9.2, 3, 7.2, color, { metal: 0.6, rough: 0.4 }), 0, py(2), 0));
        g.add(at(box(9.4, 0.8, 7.4, accent, { metal: 0.6, rough: 0.4 }), 0, py(3.3), 0));
        for (const side of [-1, 1]) {
          const horn = new Mesh(new ConeGeometry(1, 4.2, 6), mat(accent, { rough: 0.5 }));
          horn.rotation.z = -side * 0.9;
          g.add(at(horn, side * 5.6, headTop + 1.2, 0));
        }
        break;
      }
      case "crown":
      default: {
        const ring = new Mesh(new CylinderGeometry(4.9, 4.9, 1.3, 8, 1, true), mat(color, { metal: 0.8, rough: 0.3 }));
        (ring.material as MeshStandardMaterial).side = 2; // DoubleSide
        g.add(at(ring, 0, headTop + 0.4, 0));
        for (let i = 0; i < 6; i++) {
          const a = (i / 6) * Math.PI * 2;
          const spike = box(0.9, 1.8, 0.9, i % 2 === 0 ? accent : color, { metal: 0.8, rough: 0.3, emissive: i % 2 === 0 ? accent : undefined, emissiveIntensity: 0.4 });
          g.add(at(spike, Math.sin(a) * 4.6, headTop + 1.7, Math.cos(a) * 4.6));
        }
        break;
      }
    }
    body.add(track(g));
  }

  // --- body extras --------------------------------------------------------
  if (bodyShape === "robe") {
    body.add(track(at(box(9, 3.6, 5.2, bodyColor), 0, py(12), 0)));
    body.add(track(at(box(9.2, 0.7, 5.4, bodyAccent), 0, py(13.4), 0)));
    body.add(track(at(box(2, 3.6, 5.3, bodyAccent), 0, py(8.5), 0))); // front trim
  }
  if (bodyShape === "plate") {
    for (const side of [-1, 1]) {
      body.add(track(at(box(2.6, 1.6, 5.4, bodyAccent, { metal: 0.6, rough: 0.4 }), side * 5.3, py(7.2), 0)));
    }
    body.add(track(at(box(1.6, 1.6, 0.6, bodyAccent, { emissive: bodyAccent, emissiveIntensity: 0.5 }), 0, py(8.5), depthForRow(8) / 2 + 0.05)));
  }
  if (bodyShape === "mail") {
    body.add(track(at(box(8.6, 0.8, 5, bodyAccent, { metal: 0.5, rough: 0.5 }), 0, py(10.5), 0))); // belt
  }

  // --- weapon (right hand pivot) -----------------------------------------
  let weapon: Group | null = null;
  if (look.weapon) {
    const { shape, color, accent } = look.weapon;
    weapon = new Group();
    weapon.position.set(px(13) + 0.6, py(9), 0.4);
    const metal = { metal: 0.7, rough: 0.3 };
    switch (shape) {
      case "staff": {
        weapon.add(at(new Mesh(new CylinderGeometry(0.4, 0.4, 13, 6), mat(color)), 0, 2.5, 0));
        weapon.add(at(new Mesh(new SphereGeometry(1.2, 10, 8), mat(accent, { emissive: accent, emissiveIntensity: 0.9 })), 0, 9.6, 0));
        break;
      }
      case "axe": {
        weapon.add(at(new Mesh(new CylinderGeometry(0.4, 0.4, 10, 6), mat(accent)), 0, 2.5, 0));
        weapon.add(at(box(3.4, 4, 0.5, color, metal), 1.8, 5.5, 0));
        weapon.add(at(box(1.2, 5, 0.6, color, metal), 0.3, 5.5, 0));
        break;
      }
      case "spear": {
        weapon.add(at(new Mesh(new CylinderGeometry(0.32, 0.32, 15, 6), mat(shade(color, 0.6))), 0, 3, 0));
        weapon.add(at(new Mesh(new ConeGeometry(0.9, 3.2, 6), mat(color, { ...metal, emissive: accent, emissiveIntensity: 0.35 })), 0, 11.8, 0));
        weapon.add(at(box(1.4, 0.8, 1.4, accent, metal), 0, 9.9, 0));
        break;
      }
      case "sword":
      default: {
        weapon.add(at(box(0.9, 9, 0.45, color, { ...metal, emissive: color === "#ff7a1a" ? color : undefined, emissiveIntensity: 0.5 }), 0, 5.4, 0));
        weapon.add(at(box(3.2, 0.7, 0.8, accent, metal), 0, 0.8, 0));
        weapon.add(at(box(0.7, 2, 0.7, shade(accent, 0.55)), 0, -0.8, 0));
        weapon.add(at(box(1.1, 0.8, 1.1, accent, metal), 0, -2, 0));
        break;
      }
    }
    body.add(track(weapon));
  }

  // --- offhand (left hand) -------------------------------------------------
  if (look.offhand) {
    const { shape, color, accent } = look.offhand;
    const g = new Group();
    g.position.set(px(2) - 0.4, py(8.5), 1);
    switch (shape) {
      case "tower": {
        g.add(box(3.8, 6.4, 0.6, color, { metal: 0.4, rough: 0.5 }));
        g.add(at(box(1.6, 2.4, 0.4, accent, { emissive: accent, emissiveIntensity: 0.4 }), 0, 0.4, 0.5));
        g.add(at(box(4.1, 0.6, 0.7, accent, { metal: 0.5 }), 0, 3.2, 0));
        break;
      }
      case "tome": {
        g.add(box(2.8, 3.6, 1.1, color));
        g.add(at(box(0.6, 3.7, 1.2, accent, { metal: 0.5 }), -1.2, 0, 0));
        g.add(at(box(1.4, 1.4, 0.3, accent, { emissive: accent, emissiveIntensity: 0.5 }), 0.2, 0.4, 0.6));
        break;
      }
      case "buckler":
      default: {
        const disc = new Mesh(new CylinderGeometry(2.4, 2.4, 0.5, 10), mat(color, { metal: 0.3, rough: 0.55 }));
        disc.rotation.x = Math.PI / 2;
        g.add(disc);
        g.add(at(new Mesh(new SphereGeometry(0.7, 8, 6), mat(accent, { metal: 0.6, rough: 0.3 })), 0, 0, 0.35));
        break;
      }
    }
    body.add(track(g));
  }

  // --- back (cape / wings) ---------------------------------------------------
  let cape: Group | null = null;
  const wings: Group[] = [];
  if (look.back) {
    const { shape, color, accent } = look.back;
    if (shape === "wings") {
      for (const side of [-1, 1]) {
        const w = new Group();
        w.position.set(side * 4.2, py(7.5), -2.8);
        const feather = box(7, 3.2, 0.35, color, { emissive: color, emissiveIntensity: 0.35 });
        feather.position.set(side * 3.5, 0.8, 0);
        feather.rotation.z = side * 0.35;
        w.add(feather);
        const tip = box(4.4, 2, 0.3, accent, { emissive: accent, emissiveIntensity: 0.4 });
        tip.position.set(side * 7.2, 2.4, 0);
        tip.rotation.z = side * 0.65;
        w.add(tip);
        wings.push(w);
        body.add(track(w));
      }
    } else {
      cape = new Group();
      cape.position.set(0, py(7) + 0.4, -2.5);
      const cloth = box(8.6, 8.8, 0.35, color);
      cloth.position.set(0, -4.4, 0);
      cape.add(cloth);
      const hem = box(8.8, 1, 0.4, accent);
      hem.position.set(0, -8.5, 0);
      cape.add(hem);
      body.add(track(cape));
    }
  }

  // --- aura --------------------------------------------------------------------
  let auraRing: Object3D | null = null;
  const auraOrbiters: Object3D[] = [];
  if (look.aura) {
    const { shape, color, accent } = look.aura;
    if (shape === "orbit") {
      for (let i = 0; i < 3; i++) {
        const orb = new Mesh(new SphereGeometry(0.75, 8, 6), mat(color, { emissive: color, emissiveIntensity: 1.2 }));
        orb.userData.phase = (i / 3) * Math.PI * 2;
        orb.userData.tilt = i * 0.6;
        auraOrbiters.push(orb);
        root.add(track(orb));
      }
      const light = new PointLight(color, 12, 30);
      light.position.set(0, 9, 4);
      root.add(light);
    } else {
      const ring = new Mesh(new TorusGeometry(8, 0.35, 6, 40), mat(color, { emissive: color, emissiveIntensity: 1.1 }));
      ring.rotation.x = Math.PI / 2;
      ring.position.y = 0.5;
      auraRing = ring;
      root.add(track(ring));
      for (let i = 0; i < 10; i++) {
        const spark = box(0.5, 0.5, 0.5, accent, { emissive: accent, emissiveIntensity: 1.3 });
        spark.userData.phase = (i / 10) * Math.PI * 2;
        auraOrbiters.push(spark);
        root.add(track(spark));
      }
      const light = new PointLight(color, 10, 26);
      light.position.set(0, 3, 5);
      root.add(light);
    }
  }

  // --- companion ---------------------------------------------------------------
  let pet: Group | null = null;
  let petKind = "";
  const petWings: Group[] = [];
  if (look.pet) {
    const { shape, color, accent } = look.pet;
    pet = new Group();
    petKind = shape;
    pet.position.set(-9.5, 2.2, 2.5);
    switch (shape) {
      case "owl": {
        pet.add(at(new Mesh(new SphereGeometry(1.7, 10, 8), mat(color)), 0, 0, 0));
        pet.add(at(new Mesh(new SphereGeometry(1.3, 10, 8), mat(color)), 0, 1.8, 0));
        for (const side of [-1, 1]) {
          pet.add(at(new Mesh(new SphereGeometry(0.5, 8, 6), mat(accent, { emissive: accent, emissiveIntensity: 0.7 })), side * 0.6, 2, 1.1));
        }
        const beak = new Mesh(new ConeGeometry(0.3, 0.8, 4), mat("#ff9f43"));
        beak.rotation.x = Math.PI / 2;
        pet.add(at(beak, 0, 1.5, 1.5));
        break;
      }
      case "wisp": {
        pet.add(new Mesh(new SphereGeometry(1.2, 10, 8), mat(color, { emissive: color, emissiveIntensity: 1.6, opacity: 0.9 })));
        pet.add(new Mesh(new SphereGeometry(0.55, 8, 6), mat(accent, { emissive: accent, emissiveIntensity: 2 })));
        const light = new PointLight(color, 16, 24);
        pet.add(light);
        break;
      }
      case "dragon": {
        pet.add(at(new Mesh(new SphereGeometry(1.7, 10, 8), mat(color)), 0, 0, 0));
        pet.add(at(new Mesh(new SphereGeometry(1.1, 10, 8), mat(color)), 1.6, 1.1, 0.4));
        pet.add(at(new Mesh(new SphereGeometry(0.3, 6, 5), mat(accent, { emissive: accent, emissiveIntensity: 1 })), 2.2, 1.4, 1));
        const tail = new Mesh(new ConeGeometry(0.6, 3.2, 6), mat(color));
        tail.rotation.z = Math.PI / 2 + 0.3;
        pet.add(at(tail, -2.6, -0.3, 0));
        for (const side of [-1, 1]) {
          const w = new Group();
          w.position.set(-0.2, 0.8, side * 1.2);
          const membrane = box(2.8, 0.3, 2.6, accent, { opacity: 0.9 });
          membrane.position.set(0, 0.6, side * 1.5);
          w.add(membrane);
          petWings.push(w);
          pet.add(w);
        }
        break;
      }
      case "slime":
      default: {
        const blob = new Mesh(new SphereGeometry(1.9, 10, 8), mat(color, { emissive: color, emissiveIntensity: 0.25, opacity: 0.92 }));
        blob.scale.set(1, 0.75, 1);
        pet.add(blob);
        for (const side of [-1, 1]) {
          pet.add(at(new Mesh(new SphereGeometry(0.28, 6, 5), mat(accent)), side * 0.6, 0.4, 1.6));
        }
        break;
      }
    }
    root.add(track(pet));
  }

  // --- title halo -----------------------------------------------------------------
  let halo: Object3D | null = null;
  if (titled) {
    const ring = new Mesh(new TorusGeometry(3.2, 0.22, 6, 28), mat(gold, { emissive: gold, emissiveIntensity: 1.4 }));
    ring.rotation.x = Math.PI / 2;
    ring.position.y = py(0) + 1.6;
    halo = ring;
    body.add(track(ring));
  }

  return { root, body, eyes, weapon, cape, wings, auraRing, auraOrbiters, pet, petKind, petWings, halo, disposables };
}

// ---------------------------------------------------------------------------
// particles (celebrate / crit / purchase bursts)

const PARTICLES = 56;
interface Particle {
  pos: Vector3;
  vel: Vector3;
  life: number;
  ttl: number;
  spin: number;
}

class Burst {
  mesh: InstancedMesh;
  parts: Particle[] = [];
  private m4 = new Matrix4();
  private q = new Quaternion();
  private s = new Vector3();
  constructor(scene: Scene) {
    const material = mat("#ffffff", { rough: 0.4, emissive: "#ffffff", emissiveIntensity: 0.6 });
    this.mesh = new InstancedMesh(new BoxGeometry(0.7, 0.7, 0.7), material, PARTICLES);
    this.mesh.count = 0;
    this.mesh.frustumCulled = false;
    scene.add(this.mesh);
  }
  fire(colors: string[], strength = 1) {
    this.parts = [];
    for (let i = 0; i < PARTICLES; i++) {
      const a = Math.random() * Math.PI * 2;
      const up = 6 + Math.random() * 9 * strength;
      const out = (3 + Math.random() * 7) * strength;
      this.parts.push({
        pos: new Vector3(0, 7, 0),
        vel: new Vector3(Math.cos(a) * out, up, Math.sin(a) * out),
        life: 0,
        ttl: 1.1 + Math.random() * 0.7,
        spin: Math.random() * 6,
      });
      this.mesh.setColorAt(i, new Color(colors[i % colors.length]));
    }
    if (this.mesh.instanceColor) this.mesh.instanceColor.needsUpdate = true;
    this.mesh.count = PARTICLES;
  }
  step(dt: number) {
    if (this.parts.length === 0) return;
    let alive = 0;
    this.parts.forEach((p, i) => {
      p.life += dt;
      if (p.life > p.ttl) {
        this.s.set(0, 0, 0);
      } else {
        alive++;
        p.vel.y -= 28 * dt;
        p.pos.addScaledVector(p.vel, dt);
        const k = 1 - p.life / p.ttl;
        this.s.set(k, k, k);
      }
      this.q.setFromAxisAngle(new Vector3(1, 1, 0).normalize(), p.life * p.spin);
      this.m4.compose(p.pos, this.q, this.s);
      this.mesh.setMatrixAt(i, this.m4);
    });
    this.mesh.instanceMatrix.needsUpdate = true;
    if (alive === 0) {
      this.parts = [];
      this.mesh.count = 0;
    }
  }
  dispose() {
    this.mesh.geometry.dispose();
    (this.mesh.material as Material).dispose();
    this.mesh.dispose();
  }
}

// ---------------------------------------------------------------------------
// the component

export default function Hero3D({
  level,
  titled = false,
  mood = "idle",
  size = 96,
  loadout,
  preview = null,
  interactive = false,
  frame = "tight",
  burst = 0,
  burstColor,
  className,
}: Hero3DProps) {
  const host = useRef<HTMLDivElement>(null);
  const scene = useRef<{
    renderer: WebGLRenderer;
    scene: Scene;
    camera: PerspectiveCamera;
    key: DirectionalLight;
    hemi: HemisphereLight;
    burst: Burst;
    rig: Rig | null;
    mood: HeroMood;
    moodStart: number;
    dragYaw: number;
    dragging: boolean;
    lastPointer: number;
    lastInteraction: number;
    running: boolean;
    visible: boolean;
    still: boolean; // reduced motion
    raf: number;
    t0: number;
    last: number;
    blinkAt: number;
    blinkUntil: number;
    dispose(): void;
  } | null>(null);

  const look = resolveLook(level, loadout);
  if (preview) look[preview.slot] = preview;
  const lookSig = `${level}|${titled}|${Object.values(look)
    .map((p) => `${p.slot}:${p.key}:${p.color}`)
    .sort()
    .join(",")}`;
  const bodyMood = mood;
  const gold = palette().gold;
  const phos = palette().phos;

  // 1) scene lifetime -----------------------------------------------------
  useEffect(() => {
    const el = host.current;
    if (!el) return;
    const renderer = new WebGLRenderer({ alpha: true, antialias: true, powerPreference: "low-power" });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
    renderer.toneMapping = ACESFilmicToneMapping;
    renderer.toneMappingExposure = 1.05;
    renderer.domElement.style.width = "100%";
    renderer.domElement.style.height = "100%";
    renderer.domElement.style.display = "block";
    renderer.domElement.style.touchAction = interactive ? "pan-y" : "auto"; // drag rotates, swipe still scrolls
    el.appendChild(renderer.domElement);

    const sc = new Scene();
    const camera = new PerspectiveCamera(30, 1, 1, 200);
    const dist = frame === "wide" ? 44 : 34;
    camera.position.set(0, 10, dist);
    camera.lookAt(0, 7.2, 0);

    const hemi = new HemisphereLight("#ffffff", "#1a2030", 1.1);
    sc.add(hemi);
    const key = new DirectionalLight("#ffffff", 2.2);
    key.position.set(6, 12, 9);
    sc.add(key);
    const rim = new DirectionalLight(phos, 1.1);
    rim.position.set(-7, 5, -8);
    sc.add(rim);
    const fill = new DirectionalLight(gold, 0.35);
    fill.position.set(-4, 2, 8);
    sc.add(fill);

    // ground shadow blob
    const blob = new Mesh(new CircleGeometry(6.2, 24), mat("#000000", { opacity: 0.38 }));
    blob.rotation.x = -Math.PI / 2;
    blob.position.y = 0.02;
    sc.add(blob);

    const bur = new Burst(sc);

    const state = {
      renderer,
      scene: sc,
      camera,
      key,
      hemi,
      burst: bur,
      rig: null as Rig | null,
      mood: "idle" as HeroMood,
      moodStart: 0,
      dragYaw: 0,
      dragging: false,
      lastPointer: 0,
      lastInteraction: -100,
      running: true,
      visible: true,
      still: window.matchMedia("(prefers-reduced-motion: reduce)").matches,
      raf: 0,
      t0: performance.now(),
      last: performance.now(),
      blinkAt: 2 + Math.random() * 3,
      blinkUntil: 0,
      dispose() {},
    };
    scene.current = state;

    const resize = () => {
      const w = el.clientWidth || size;
      const h = el.clientHeight || size;
      renderer.setSize(w, h, false);
      camera.aspect = w / h;
      camera.updateProjectionMatrix();
    };
    resize();
    const ro = new ResizeObserver(resize);
    ro.observe(el);

    const io = new IntersectionObserver((entries) => {
      state.visible = entries.some((e) => e.isIntersecting);
    });
    io.observe(el);
    const onVis = () => {
      state.running = !document.hidden;
    };
    document.addEventListener("visibilitychange", onVis);

    // drag to rotate
    const onDown = (e: PointerEvent) => {
      if (!interactive) return;
      state.dragging = true;
      state.lastPointer = e.clientX;
      state.lastInteraction = (performance.now() - state.t0) / 1000;
      renderer.domElement.setPointerCapture(e.pointerId);
    };
    const onMove = (e: PointerEvent) => {
      if (!state.dragging) return;
      state.dragYaw += (e.clientX - state.lastPointer) * 0.012;
      state.lastPointer = e.clientX;
      state.lastInteraction = (performance.now() - state.t0) / 1000;
    };
    const onUp = () => {
      state.dragging = false;
    };
    renderer.domElement.addEventListener("pointerdown", onDown);
    renderer.domElement.addEventListener("pointermove", onMove);
    renderer.domElement.addEventListener("pointerup", onUp);
    renderer.domElement.addEventListener("pointercancel", onUp);

    const keyColor = new Color("#ffffff");
    const red = new Color("#ff3b3b");
    const warm = new Color("#ffb070");
    const tmp = new Color();

    const tick = (now: number) => {
      state.raf = requestAnimationFrame(tick);
      if (!state.running || !state.visible) return;
      const dt = Math.min(0.05, (now - state.last) / 1000);
      state.last = now;
      const time = (now - state.t0) / 1000;
      const rig = state.rig;
      const m = state.mood;
      const since = time - state.moodStart;

      if (rig && state.still) {
        rig.eyes.forEach((e) => e.scale.set(0.9, 0.9, 0.5));
        rig.root.position.set(0, 0, 0);
        rig.root.rotation.set(0, -0.45 + state.dragYaw + (m === "focus" ? -0.35 : 0), 0);
        rig.body.rotation.x = m === "focus" ? -0.12 : 0;
        if (rig.cape) rig.cape.rotation.x = 0.25;
        rig.wings.forEach((w, i) => (w.rotation.z = (i === 0 ? -1 : 1) * 0.3));
        rig.auraOrbiters.forEach((o, i) => {
          const ph = (o.userData.phase as number) + i;
          if (o.userData.tilt !== undefined) o.position.set(Math.cos(ph) * 7, 8, Math.sin(ph) * 7);
          else o.position.set(Math.cos(ph) * 8, 0.8, Math.sin(ph) * 8);
        });
        if (rig.pet) rig.pet.position.y = rig.petKind === "slime" ? 1.9 : 8;
        if (rig.halo) rig.halo.position.y = 15.4;
      } else if (rig) {
        // blink
        if (time > state.blinkAt) {
          state.blinkUntil = time + 0.13;
          state.blinkAt = time + 2.5 + Math.random() * 3.5;
        }
        const blinking = time < state.blinkUntil;
        rig.eyes.forEach((e) => e.scale.set(0.9, blinking ? 0.12 : 0.9, 0.5));

        let y = 0;
        let yaw = 0;
        let lean = 0;
        let roll = 0;
        let x = 0;
        let scaleY = 1;
        switch (m) {
          case "celebrate": {
            if (since < 2.4) {
              y = Math.abs(Math.sin((since / 0.8) * Math.PI)) * 3.2;
              yaw = since < 1.6 ? (since / 1.6) * Math.PI * 4 : 0;
              scaleY = 1 + Math.sin((since / 0.8) * Math.PI * 2) * 0.06;
            } else {
              y = Math.sin(time * 2.4) * 0.25;
              yaw = Math.sin(time * 0.6) * 0.25;
            }
            break;
          }
          case "crit": {
            if (since < 1.4) {
              x = Math.sin(time * 70) * 0.45 * (1 - since / 1.4);
              roll = Math.sin(time * 55) * 0.1 * (1 - since / 1.4);
            }
            y = Math.sin(time * 2.4) * 0.25;
            break;
          }
          case "focus": {
            y = Math.sin(time * 5.5) * 0.35;
            lean = -0.18;
            yaw = -0.35 + Math.sin(time * 1.1) * 0.08;
            break;
          }
          case "camp": {
            y = Math.sin(time * 1.2) * 0.18;
            scaleY = 1 + Math.sin(time * 1.2) * 0.015;
            yaw = Math.sin(time * 0.35) * 0.2;
            break;
          }
          default: {
            y = Math.sin(time * 2.4) * 0.25;
            yaw = Math.sin(time * 0.6) * 0.25;
          }
        }
        const idleTurn = interactive && time - state.lastInteraction > 4 ? (time - state.lastInteraction - 4) * 0.25 : 0;
        const restYaw = -0.35; // three-quarter view so the depth reads
        rig.root.position.set(x, y, 0);
        rig.root.rotation.set(0, restYaw + yaw + state.dragYaw + idleTurn, roll);
        rig.body.rotation.x = lean;
        rig.body.scale.set(1, scaleY, 1);

        if (rig.weapon) {
          rig.weapon.rotation.z = m === "focus" ? Math.sin(time * 7) * 0.3 - 0.2 : m === "celebrate" && since < 2.4 ? -0.9 : Math.sin(time * 2.4) * 0.05;
        }
        if (rig.cape) rig.cape.rotation.x = 0.25 + Math.sin(time * 2.2) * 0.12 + (m === "focus" ? 0.25 : 0);
        rig.wings.forEach((w, i) => {
          const side = i === 0 ? -1 : 1;
          w.rotation.z = side * (0.15 + Math.sin(time * 4 + i) * 0.35);
          w.rotation.y = side * Math.sin(time * 4 + i) * 0.25;
        });
        if (rig.auraRing) rig.auraRing.rotation.z = time * 0.6;
        rig.auraOrbiters.forEach((o) => {
          const ph = (o.userData.phase as number) + time * 1.4;
          if (o.userData.tilt !== undefined) {
            const tilt = o.userData.tilt as number;
            o.position.set(Math.cos(ph) * 7, 8 + Math.sin(ph * 1.3 + tilt) * 3, Math.sin(ph) * 7);
          } else {
            o.position.set(Math.cos(ph) * 8, 0.8 + Math.abs(Math.sin(ph * 2)) * 1.2, Math.sin(ph) * 8);
            o.rotation.set(ph, ph * 0.7, 0);
          }
        });
        if (rig.pet) {
          const p = rig.pet;
          switch (rig.petKind) {
            case "slime": {
              const hop = Math.abs(Math.sin(time * 3.2));
              p.position.y = 1.9 + hop * 1.6;
              p.scale.set(1 + (1 - hop) * 0.15, 0.85 + hop * 0.25, 1 + (1 - hop) * 0.15);
              break;
            }
            case "wisp":
              p.position.set(-8.5 + Math.sin(time * 0.9) * 2.5, 9 + Math.sin(time * 1.7) * 1.8, 2 + Math.cos(time * 0.9) * 2.5);
              break;
            case "dragon":
              p.position.y = 8.5 + Math.sin(time * 2.2) * 0.9;
              p.rotation.z = Math.sin(time * 2.2) * 0.08;
              rig.petWings.forEach((w, i) => {
                w.rotation.x = (i === 0 ? -1 : 1) * Math.sin(time * 9) * 0.7;
              });
              break;
            default:
              p.position.y = 7.5 + Math.sin(time * 1.6) * 0.8;
              p.rotation.y = Math.sin(time * 0.8) * 0.4;
          }
        }
        if (rig.halo) {
          rig.halo.rotation.z = time * 0.8;
          rig.halo.position.y = 15.4 + Math.sin(time * 2) * 0.25;
        }
      }

      // lighting per mood
      if (m === "crit" && since < 1.2 && !state.still) {
        tmp.copy(keyColor).lerp(red, Math.max(0, 1 - since / 1.2));
        state.key.color.copy(tmp);
        state.key.intensity = 2.2 + (1 - since / 1.2) * 2.5;
      } else if (m === "camp") {
        state.key.color.copy(tmp.copy(keyColor).lerp(warm, 0.55));
        state.key.intensity = 1.4 + Math.sin(time * 6) * 0.08;
        state.hemi.intensity = 0.7;
      } else {
        state.key.color.copy(keyColor);
        state.key.intensity = 2.2;
        state.hemi.intensity = 1.1;
      }

      bur.step(dt);
      renderer.render(sc, camera);
    };
    state.raf = requestAnimationFrame(tick);

    state.dispose = () => {
      cancelAnimationFrame(state.raf);
      ro.disconnect();
      io.disconnect();
      document.removeEventListener("visibilitychange", onVis);
      renderer.domElement.removeEventListener("pointerdown", onDown);
      renderer.domElement.removeEventListener("pointermove", onMove);
      renderer.domElement.removeEventListener("pointerup", onUp);
      renderer.domElement.removeEventListener("pointercancel", onUp);
      if (state.rig) {
        sc.remove(state.rig.root);
        state.rig.disposables.forEach((d) => d.dispose());
      }
      bur.dispose();
      blob.geometry.dispose();
      (blob.material as Material).dispose();
      renderer.dispose();
      renderer.forceContextLoss();
      if (renderer.domElement.parentNode === el) el.removeChild(renderer.domElement);
      scene.current = null;
    };
    return () => state.dispose();
    // The scene is created once per mount; props that change the hero are
    // handled by the effects below (rebuild / mood / burst).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 2) (re)build the hero when level / gear / title change ------------------
  useEffect(() => {
    const st = scene.current;
    if (!st) return;
    if (st.rig) {
      st.scene.remove(st.rig.root);
      st.rig.disposables.forEach((d) => d.dispose());
    }
    const rig = buildHero(level, look, titled);
    st.scene.add(rig.root);
    st.rig = rig;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lookSig]);

  // 3) mood --------------------------------------------------------------------
  useEffect(() => {
    const st = scene.current;
    if (!st) return;
    st.mood = bodyMood;
    st.moodStart = (performance.now() - st.t0) / 1000;
    if (st.still) return;
    if (bodyMood === "celebrate") st.burst.fire([gold, phos, "#ffffff"], 1);
    if (bodyMood === "crit") st.burst.fire(["#ff4747", gold, "#ffffff"], 1.4);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bodyMood]);

  // 4) explicit bursts (purchase / equip) ----------------------------------------
  useEffect(() => {
    const st = scene.current;
    if (!st || burst === 0 || st.still) return;
    st.burst.fire([burstColor ?? gold, "#ffffff", phos], 0.9);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [burst]);

  return (
    <div
      ref={host}
      className={className}
      style={{ width: size, height: size, position: "relative", cursor: interactive ? "grab" : undefined }}
      role="img"
      aria-label={t("hero.alt", { level })}
      data-testid="hero-3d"
      data-mood={mood}
    />
  );
}
