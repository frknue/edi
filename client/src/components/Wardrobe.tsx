// Wardrobe: the gear shop for the hero, built around the hooks a good game
// shop uses honestly — a chosen GOAL with visible gold progress, a DAILY
// DEAL as a reason to come back, the NEXT UNLOCK tier as a level carrot,
// COLLECTION completion (sets), and a purchase CEREMONY. No countdown
// pressure, no gacha, no real money: gold is earned by quests only.
//
// Layout: fitting room (interactive 3D hero, goal bar, slots) → featured
// strip (deal / next unlock / collection) → filters → the catalog grid.
// Try-on: CLICKING a piece pins it on the hero (one per slot, so a whole
// outfit can be assembled and judged); HOVERING peeks at a piece for as
// long as the pointer rests on it. Pins survive scrolling and filtering,
// clear with ×/Escape, and drop for a slot once that piece is bought.
// Buying is arm-then-confirm; the server loadout is the truth, the try-on
// is client state only.
import { useEffect, useMemo, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { Check, Coins, Crown, Eye, Lock, PawPrint, Shield, Shirt, Sparkles, Star, Sword, Wind, X } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import {
  useBuyCosmetic,
  useClearGearGoal,
  useCosmetics,
  useEquipCosmetic,
  useSetGearGoal,
  useUnequipCosmetic,
} from "../lib/queries";
import { rarityColor } from "../lib/theme";
import { pushToast } from "../lib/toast";
import { useI18n } from "../lib/i18n";
import type { MessageKey } from "../lib/locales/en";
import type { CosmeticCatalog, CosmeticItem, CosmeticPurchaseResult, CosmeticSlot, EquippedCosmetic } from "../lib/types";
import { Hero } from "./Hero";
import { Btn, EmptyState, Spinner } from "./ui";

const SLOT_ICON: Record<CosmeticSlot, LucideIcon> = {
  head: Crown,
  body: Shirt,
  weapon: Sword,
  offhand: Shield,
  back: Wind,
  aura: Sparkles,
  pet: PawPrint,
};

function asEquipped(it: CosmeticItem): EquippedCosmetic {
  return { slot: it.slot, key: it.key, name: it.name, rarity: it.rarity, shape: it.shape, color: it.color, accent: it.accent };
}

const rc = (rarity: string) => rarityColor[rarity] ?? rarityColor.common;

// "26g short" is a wall; "≈ 3 quests away" is a plan.
function questsAway(missing: number, avgGold: number): number {
  return Math.max(1, Math.ceil(missing / Math.max(1, avgGold)));
}

// --- one purchase flow shared by cards, the deal and the goal --------------

function useBuyFlow(onBought: (res: CosmeticPurchaseResult) => void) {
  const { t } = useI18n();
  const buy = useBuyCosmetic();
  const [arming, setArming] = useState<string | null>(null);
  useEffect(() => {
    if (!arming) return;
    const id = window.setTimeout(() => setArming(null), 3500);
    return () => window.clearTimeout(id);
  }, [arming]);
  const trigger = (item: CosmeticItem) => {
    if (arming !== item.key) {
      setArming(item.key);
      return;
    }
    setArming(null);
    buy.mutate(item.key, {
      onSuccess: (res) => {
        pushToast(t("wardrobe.bought", { name: res.item.name, price: res.item.price }), "success");
        onBought(res);
      },
    });
  };
  return { trigger, arming, pending: buy.isPending };
}

function BuyButton({
  item,
  balance,
  flow,
  testId,
}: {
  item: CosmeticItem;
  balance: number;
  flow: ReturnType<typeof useBuyFlow>;
  testId: string;
}) {
  const { t } = useI18n();
  const affordable = balance >= item.price;
  return (
    <Btn
      variant={affordable ? "primary" : "ghost"}
      disabled={!affordable || flow.pending}
      onClick={() => flow.trigger(item)}
      title={affordable ? undefined : t("wardrobe.needGold", { n: item.price - balance })}
      data-testid={testId}
    >
      {flow.arming === item.key ? t("wardrobe.confirm", { price: item.price }) : affordable ? t("wardrobe.buy") : t("wardrobe.tooCostly")}
    </Btn>
  );
}

// --- catalog card ----------------------------------------------------------------

function GearCard({
  item,
  data,
  pinned,
  onPin,
  onHover,
  flow,
}: {
  item: CosmeticItem;
  data: CosmeticCatalog;
  pinned: boolean; // this piece is on the hero right now (clicked)
  onPin: (item: CosmeticItem) => void;
  onHover: (item: CosmeticItem | null) => void;
  flow: ReturnType<typeof useBuyFlow>;
}) {
  const { t, tp } = useI18n();
  const equip = useEquipCosmetic();
  const setGoal = useSetGearGoal();
  const clearGoal = useClearGearGoal();
  const color = rc(item.rarity);
  const balance = data.balance;
  const affordable = balance >= item.price;
  const isGoal = data.goal?.item.key === item.key;
  const isNew = item.unlocked && !item.owned && item.min_level === data.level && data.level > 1;
  const rarityLabel = t(`rarity.${item.rarity}` as MessageKey);
  const shiny = item.rarity === "epic" || item.rarity === "legendary";

  let action: React.ReactNode;
  if (item.equipped) {
    action = (
      <span className="inline-flex items-center gap-1 font-display text-[11px] uppercase tracking-wider" style={{ color: "var(--color-phos)" }} data-testid={`gear-equipped-${item.key}`}>
        <Check size={13} /> {t("wardrobe.equipped")}
      </span>
    );
  } else if (item.owned) {
    action = (
      <Btn variant="ghost" disabled={equip.isPending} onClick={() => equip.mutate(item.key)} data-testid={`gear-equip-${item.key}`}>
        {t("wardrobe.equip")}
      </Btn>
    );
  } else if (!item.unlocked) {
    action = (
      <span className="inline-flex items-center gap-1 font-display text-[11px] uppercase tracking-wider text-faint" data-testid={`gear-locked-${item.key}`}>
        <Lock size={12} /> {t("wardrobe.locked", { level: item.min_level })}
      </span>
    );
  } else {
    action = <BuyButton item={item} balance={balance} flow={flow} testId={`gear-buy-${item.key}`} />;
  }

  const Icon = SLOT_ICON[item.slot];
  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      className={`hud-panel hud-panel-hover relative flex flex-col gap-2.5 p-3.5 ${shiny ? "rarity-sheen" : ""}`}
      style={{
        borderColor: pinned ? color : item.equipped ? "rgba(var(--phos-rgb),0.5)" : `${color}40`,
        borderStyle: pinned ? "dashed" : undefined,
        boxShadow: pinned ? `0 0 22px -8px ${color}` : shiny ? `inset 0 0 40px -30px ${color}` : undefined,
        opacity: item.unlocked || item.owned ? 1 : 0.72,
      }}
      data-testid={`gear-${item.key}`}
      data-rarity={item.rarity}
    >
      {/* badges + the goal star */}
      <div className="absolute right-2 top-2 flex items-center gap-1">
        {item.deal && !item.owned && (
          <span className="deal-tag rounded-sm px-1.5 py-0.5 font-display text-[9px] uppercase tracking-wider" style={{ background: "var(--color-gold)", color: "#1a1200" }} data-testid={`badge-deal-${item.key}`}>
            {t("wardrobe.dealOff", { percent: data.deal?.percent ?? 0 })}
          </span>
        )}
        {isNew && (
          <span className="rounded-sm px-1.5 py-0.5 font-display text-[9px] uppercase tracking-wider" style={{ background: "rgba(var(--phos-rgb),0.18)", color: "var(--color-phos)" }} data-testid={`badge-new-${item.key}`}>
            {t("wardrobe.new")}
          </span>
        )}
        {!item.owned && (
          <button
            type="button"
            className="icon-action"
            style={isGoal ? { color: "var(--color-goldhi)" } : undefined}
            onClick={() => (isGoal ? clearGoal.mutate() : setGoal.mutate(item.key))}
            disabled={setGoal.isPending || clearGoal.isPending}
            aria-pressed={isGoal}
            aria-label={isGoal ? t("wardrobe.clearGoal") : t("wardrobe.setGoal")}
            title={isGoal ? t("wardrobe.clearGoal") : t("wardrobe.setGoal")}
            data-testid={`goal-${item.key}`}
          >
            <Star size={14} fill={isGoal ? "currentColor" : "none"} />
          </button>
        )}
      </div>

      <button
        type="button"
        className="flex items-start gap-3 pr-16 text-left focus:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-phos)]"
        onClick={() => onPin(item)}
        onMouseEnter={() => onHover(item)}
        onMouseLeave={() => onHover(null)}
        aria-pressed={pinned}
        aria-label={pinned ? t("wardrobe.takeOffPreview", { name: item.name }) : t("wardrobe.tryOn", { name: item.name })}
        data-testid={`tryon-${item.key}`}
      >
        <span
          className="grid h-11 w-11 shrink-0 place-items-center rounded-sm border"
          style={{
            borderColor: `${color}80`,
            background: `linear-gradient(135deg, ${item.color}, ${item.accent})`,
            boxShadow: `inset 0 0 12px rgba(0,0,0,0.35), 0 0 14px -6px ${color}`,
          }}
        >
          <Icon size={18} color="#0b0d12" strokeWidth={2.4} />
        </span>
        <span className="min-w-0">
          <span className="block truncate text-sm font-semibold text-ink">{item.name}</span>
          <span className="block font-display text-[10px] uppercase tracking-[0.2em]" style={{ color }}>
            {rarityLabel} · {t(`slot.${item.slot}` as MessageKey)}
            {item.set && <span className="text-faint"> · {t(`set.${item.set}` as MessageKey)}</span>}
          </span>
          <span className="mt-0.5 block text-[11px] leading-snug text-muted">{item.flavor}</span>
          {pinned && !item.equipped && (
            <span className="mt-1 inline-flex items-center gap-1 font-display text-[9px] uppercase tracking-wider" style={{ color }} data-testid={`pinned-${item.key}`}>
              <Eye size={11} /> {t("wardrobe.onHero")}
            </span>
          )}
        </span>
      </button>

      <div className="mt-auto flex items-center justify-between gap-2">
        {item.owned ? (
          <span className="font-display text-[10px] uppercase tracking-wider text-faint">{t("wardrobe.owned")}</span>
        ) : (
          <span className="min-w-0">
            <span className="tabnum inline-flex items-center gap-1 text-sm font-bold" style={{ color: "var(--color-gold)" }}>
              <Coins size={13} /> {item.price}g
              {item.deal && <s className="text-[11px] font-normal text-faint">{item.list_price}g</s>}
            </span>
            {item.unlocked && !affordable && (
              <span className="block text-[10px] text-faint" data-testid={`quests-away-${item.key}`}>
                {tp("wardrobe.questsAway", questsAway(item.price - balance, data.avg_quest_gold))}
              </span>
            )}
          </span>
        )}
        {action}
      </div>
    </motion.div>
  );
}

// --- the goal bar (fitting room) --------------------------------------------------

function GoalPanel({ data, flow }: { data: CosmeticCatalog; flow: ReturnType<typeof useBuyFlow> }) {
  const { t, tp } = useI18n();
  const clearGoal = useClearGearGoal();
  const goal = data.goal;
  if (!goal) {
    return (
      <div className="rounded-sm border border-dashed border-edge px-3 py-2.5" data-testid="goal-empty">
        <div className="flex items-center gap-2 font-display text-[11px] uppercase tracking-wider text-muted">
          <Star size={13} /> {t("wardrobe.goalNone")}
        </div>
        <div className="mt-0.5 text-[11px] text-faint">{t("wardrobe.goalNoneHint")}</div>
      </div>
    );
  }
  const color = rc(goal.item.rarity);
  const ready = goal.missing === 0;
  return (
    <div className="rounded-sm border px-3 py-2.5" style={{ borderColor: `${color}66`, background: `${color}0d` }} data-testid="goal-panel">
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0">
          <div className="font-display text-[10px] uppercase tracking-[0.2em] text-faint">{t("wardrobe.savingFor")}</div>
          <div className="truncate text-sm font-semibold" style={{ color }}>
            {goal.item.name}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {ready && goal.item.unlocked ? (
            <Btn variant="primary" disabled={flow.pending} onClick={() => flow.trigger(goal.item)} data-testid="goal-claim">
              {flow.arming === goal.item.key ? t("wardrobe.confirm", { price: goal.item.price }) : t("wardrobe.claim")}
            </Btn>
          ) : ready ? (
            <span className="inline-flex items-center gap-1 font-display text-[11px] uppercase tracking-wider text-faint" data-testid="goal-locked">
              <Lock size={12} /> {t("wardrobe.locked", { level: goal.item.min_level })}
            </span>
          ) : (
            <span className="tabnum text-xs text-muted">{tp("wardrobe.questsAway", questsAway(goal.missing, data.avg_quest_gold))}</span>
          )}
          <button type="button" className="icon-action" onClick={() => clearGoal.mutate()} aria-label={t("wardrobe.clearGoal")} title={t("wardrobe.clearGoal")} data-testid="goal-clear">
            <X size={13} />
          </button>
        </div>
      </div>
      <div className="goal-bar mt-2">
        <span style={{ width: `${Math.round(goal.progress * 100)}%`, background: color, boxShadow: `0 0 8px ${color}` }} />
      </div>
      <div className="mt-1 flex items-center justify-between text-[11px]">
        <span className="tabnum text-muted">{t("wardrobe.goalProgress", { balance: goal.balance, price: goal.item.price })}</span>
        {ready && <span style={{ color }}>{t("wardrobe.goalReady")}</span>}
      </div>
    </div>
  );
}

// --- featured strip ----------------------------------------------------------------

function DealCard({ data, flow, onPin, onHover }: { data: CosmeticCatalog; flow: ReturnType<typeof useBuyFlow>; onPin: (i: CosmeticItem) => void; onHover: (i: CosmeticItem | null) => void }) {
  const { t } = useI18n();
  const deal = data.deal;
  const item = deal ? data.items.find((i) => i.key === deal.key) : undefined;
  if (!deal || !item) return null;
  const color = rc(item.rarity);
  const Icon = SLOT_ICON[item.slot];
  return (
    <div
      className="hud-panel rarity-sheen relative flex flex-col gap-2 p-3.5"
      style={{ borderColor: "rgba(var(--gold-rgb),0.55)", background: `linear-gradient(160deg, rgba(var(--gold-rgb),0.10), transparent 60%), var(--color-panel)` }}
      data-testid="deal-card"
    >
      <div className="flex items-center justify-between">
        <span className="font-display text-[10px] uppercase tracking-[0.25em]" style={{ color: "var(--color-goldhi)" }}>
          {t("wardrobe.dealTitle")}
        </span>
        <span className="deal-tag rounded-sm px-1.5 py-0.5 font-display text-[10px] uppercase tracking-wider" style={{ background: "var(--color-gold)", color: "#1a1200" }}>
          {t("wardrobe.dealOff", { percent: deal.percent })} · {t("wardrobe.today")}
        </span>
      </div>
      <button type="button" className="flex items-center gap-3 text-left" onClick={() => onPin(item)} onMouseEnter={() => onHover(item)} onMouseLeave={() => onHover(null)} aria-label={t("wardrobe.tryOn", { name: item.name })} data-testid="deal-tryon">
        <span className="grid h-12 w-12 shrink-0 place-items-center rounded-sm border" style={{ borderColor: `${color}80`, background: `linear-gradient(135deg, ${item.color}, ${item.accent})`, boxShadow: `0 0 16px -6px ${color}` }}>
          <Icon size={20} color="#0b0d12" strokeWidth={2.4} />
        </span>
        <span className="min-w-0">
          <span className="block truncate text-base font-semibold text-ink">{item.name}</span>
          <span className="block font-display text-[10px] uppercase tracking-[0.2em]" style={{ color }}>
            {t(`rarity.${item.rarity}` as MessageKey)} · {t(`slot.${item.slot}` as MessageKey)}
          </span>
        </span>
      </button>
      <div className="mt-auto flex items-center justify-between gap-2">
        <span className="tabnum inline-flex items-baseline gap-1.5">
          <span className="text-lg font-bold" style={{ color: "var(--color-goldhi)" }}>
            {deal.price}g
          </span>
          <s className="text-xs text-faint">{deal.list_price}g</s>
        </span>
        {item.unlocked ? (
          <BuyButton item={item} balance={data.balance} flow={flow} testId="deal-buy" />
        ) : (
          <span className="inline-flex items-center gap-1 font-display text-[11px] uppercase tracking-wider text-faint">
            <Lock size={12} /> {t("wardrobe.locked", { level: item.min_level })}
          </span>
        )}
      </div>
      <div className="text-[11px] text-faint">{t("wardrobe.dealHint", { percent: deal.percent })}</div>
    </div>
  );
}

function NextUnlockCard({ data, onPin, onHover }: { data: CosmeticCatalog; onPin: (i: CosmeticItem) => void; onHover: (i: CosmeticItem | null) => void }) {
  const { t, tp } = useI18n();
  const nu = data.next_unlock;
  const opening = nu ? data.items.filter((i) => nu.keys.includes(i.key)) : [];
  return (
    <div className="hud-panel flex flex-col gap-2 p-3.5" data-testid="next-unlock">
      <span className="font-display text-[10px] uppercase tracking-[0.25em] text-muted">{t("wardrobe.nextUnlock")}</span>
      {!nu ? (
        <span className="text-sm text-muted">{t("wardrobe.allUnlocked")}</span>
      ) : (
        <>
          <span className="text-sm font-semibold text-ink">{tp("wardrobe.nextUnlockAt", opening.length, { level: nu.level })}</span>
          <div className="flex flex-wrap gap-1.5">
            {opening.map((i) => {
              const Icon = SLOT_ICON[i.slot];
              return (
                <button
                  key={i.key}
                  type="button"
                  className="grid h-8 w-8 place-items-center rounded-sm border"
                  style={{ borderColor: `${rc(i.rarity)}80`, background: `linear-gradient(135deg, ${i.color}, ${i.accent})` }}
                  onClick={() => onPin(i)}
                  onMouseEnter={() => onHover(i)}
                  onMouseLeave={() => onHover(null)}
                  title={i.name}
                  aria-label={t("wardrobe.tryOn", { name: i.name })}
                >
                  <Icon size={14} color="#0b0d12" strokeWidth={2.4} />
                </button>
              );
            })}
          </div>
          <span className="tabnum mt-auto text-[11px]" style={{ color: "var(--color-goldhi)" }} data-testid="next-unlock-xp">
            {t("wardrobe.xpToGo", { xp: nu.xp_to_go })}
          </span>
        </>
      )}
    </div>
  );
}

function CollectionCard({ data, onFilterSet }: { data: CosmeticCatalog; onFilterSet: (set: string) => void }) {
  const { t } = useI18n();
  const sets = useMemo(() => {
    const m = new Map<string, { owned: number; total: number }>();
    for (const i of data.items) {
      if (!i.set) continue;
      const e = m.get(i.set) ?? { owned: 0, total: 0 };
      e.total++;
      if (i.owned) e.owned++;
      m.set(i.set, e);
    }
    return [...m.entries()].sort((a, b) => b[1].owned / b[1].total - a[1].owned / a[1].total);
  }, [data.items]);
  const almost = sets.find(([, s]) => s.total - s.owned === 1);
  const pct = data.total ? data.owned_count / data.total : 0;
  return (
    <div className="hud-panel flex flex-col gap-2 p-3.5" data-testid="collection-card">
      <span className="font-display text-[10px] uppercase tracking-[0.25em] text-muted">{t("wardrobe.collection")}</span>
      <span className="text-sm font-semibold text-ink">{t("wardrobe.collected", { n: data.owned_count, total: data.total })}</span>
      <div className="goal-bar">
        <span style={{ width: `${Math.round(pct * 100)}%`, background: "var(--color-phos)", boxShadow: "0 0 8px rgba(var(--phos-rgb),0.6)" }} />
      </div>
      <div className="flex flex-wrap gap-1">
        {sets.map(([name, s]) => (
          <button
            key={name}
            type="button"
            onClick={() => onFilterSet(name)}
            className="rounded-sm border px-1.5 py-0.5 font-display text-[9px] uppercase tracking-wider"
            style={{
              borderColor: s.owned === s.total ? "var(--color-gold)" : "var(--color-edge)",
              color: s.owned === s.total ? "var(--color-goldhi)" : s.owned > 0 ? "var(--color-ink)" : "var(--color-faint)",
            }}
            data-testid={`set-${name}`}
          >
            {t("wardrobe.setProgress", { name: t(`set.${name}` as MessageKey), owned: s.owned, total: s.total })}
          </button>
        ))}
      </div>
      {almost && (
        <span className="mt-auto text-[11px]" style={{ color: "var(--color-goldhi)" }}>
          {t("wardrobe.setAlmost", { name: t(`set.${almost[0]}` as MessageKey) })}
        </span>
      )}
    </div>
  );
}

// --- the ceremony -----------------------------------------------------------------

function AcquiredModal({ result, level, hasGoal, onClose }: { result: CosmeticPurchaseResult; level: number; hasGoal: boolean; onClose: () => void }) {
  const { t } = useI18n();
  const item = result.item;
  const color = rc(item.rarity);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <motion.div
      className="fixed inset-0 z-50 flex items-center justify-center p-6"
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      style={{ background: "rgba(4,5,9,0.78)", backdropFilter: "blur(6px)" }}
      onClick={onClose}
      data-testid="acquired-modal"
    >
      <motion.div
        role="dialog"
        aria-modal="true"
        aria-label={t("wardrobe.acquired")}
        className="hud-panel relative w-full max-w-sm overflow-hidden p-6 text-center"
        style={{ borderColor: color, boxShadow: `0 0 60px -20px ${color}` }}
        initial={{ scale: 0.85, y: 24, opacity: 0 }}
        animate={{ scale: 1, y: 0, opacity: 1 }}
        exit={{ scale: 0.9, y: 10, opacity: 0 }}
        transition={{ type: "spring", stiffness: 300, damping: 22 }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="rays" style={{ color }} />
        <button onClick={onClose} className="absolute right-3 top-3 z-10 text-faint hover:text-ink" aria-label={t("common.close")}>
          <X size={18} />
        </button>
        <div className="relative mx-auto -mt-2 flex justify-center">
          <Hero level={level} loadout={result.loadout} mood="celebrate" size={190} frame="wide" />
        </div>
        <div className="relative font-display text-[11px] uppercase tracking-[0.35em] text-muted">*** {t("wardrobe.acquired")} ***</div>
        <motion.div
          className="relative mt-1 font-display text-2xl"
          style={{ color, textShadow: `0 0 18px ${color}` }}
          initial={{ scale: 0.7 }}
          animate={{ scale: 1 }}
          transition={{ type: "spring", stiffness: 360, damping: 14, delay: 0.1 }}
        >
          {item.name}
        </motion.div>
        <div className="relative font-display text-[10px] uppercase tracking-[0.25em]" style={{ color }}>
          {t(`rarity.${item.rarity}` as MessageKey)} · {t(`slot.${item.slot}` as MessageKey)}
        </div>
        <p className="relative mt-2 text-xs text-muted">{item.flavor}</p>
        <div className="relative mt-3 inline-flex items-center gap-1.5 font-display text-[11px] uppercase tracking-wider" style={{ color: "var(--color-phos)" }}>
          <Check size={13} /> {t("wardrobe.acquiredHint")}
        </div>
        {!hasGoal && <p className="relative mt-3 text-[11px] text-faint">{t("wardrobe.nextGoalHint")}</p>}
        <div className="relative mt-4">
          <Btn variant="primary" onClick={onClose} data-testid="acquired-close">
            {t("wardrobe.keepBrowsing")}
          </Btn>
        </div>
      </motion.div>
    </motion.div>
  );
}

// --- the page ------------------------------------------------------------------------

export function Wardrobe() {
  const { t } = useI18n();
  const cat = useCosmetics();
  const unequip = useUnequipCosmetic();
  const [filter, setFilter] = useState<CosmeticSlot | "all">("all");
  const [setFilterName, setSetFilterName] = useState<string | null>(null);
  const [affordableOnly, setAffordableOnly] = useState(false);
  // Try-on state: pins are per slot (an outfit), the hover is a peek that
  // overrides its slot only while the pointer rests on a piece.
  const [pins, setPins] = useState<Partial<Record<CosmeticSlot, CosmeticItem>>>({});
  const [hovered, setHovered] = useState<CosmeticItem | null>(null);
  const [burst, setBurst] = useState(0);
  const [burstColor, setBurstColor] = useState<string | undefined>(undefined);
  const [mood, setMood] = useState<"idle" | "celebrate">("idle");
  const [acquired, setAcquired] = useState<CosmeticPurchaseResult | null>(null);

  // Click = pin for its slot; clicking the pinned piece (or the piece that
  // is actually equipped) restores the slot to what the hero really wears.
  const togglePin = (item: CosmeticItem) =>
    setPins((cur) => {
      const next = { ...cur };
      if (item.equipped || next[item.slot]?.key === item.key) delete next[item.slot];
      else next[item.slot] = item;
      return next;
    });
  const unpinSlot = (slot: CosmeticSlot) =>
    setPins((cur) => {
      const next = { ...cur };
      delete next[slot];
      return next;
    });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setPins({});
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const flow = useBuyFlow((res) => {
    setHovered(null);
    unpinSlot(res.item.slot); // it is equipped now — the pin did its job
    setBurstColor(res.item.color);
    setBurst((n) => n + 1);
    setMood("celebrate");
    window.setTimeout(() => setMood("idle"), 2600);
    setAcquired(res);
  });

  const data = cat.data;
  const items = useMemo(() => {
    if (!data) return [];
    return data.items.filter(
      (it) =>
        (filter === "all" || it.slot === filter) &&
        (!setFilterName || it.set === setFilterName) &&
        (!affordableOnly || it.owned || (it.unlocked && it.price <= data.balance)),
    );
  }, [data, filter, setFilterName, affordableOnly]);

  if (cat.isLoading) return <Spinner label={t("wardrobe.loading")} />;
  if (cat.isError || !data) {
    return <EmptyState title={t("common.backendUnreachable")} hint={(cat.error as Error)?.message ?? t("common.backendHint")} />;
  }
  const worn = new Map(data.loadout.map((e) => [e.slot, e]));
  const pinnedList = Object.values(pins).filter((p): p is CosmeticItem => !!p);
  const peek = hovered && !hovered.equipped && pins[hovered.slot]?.key !== hovered.key ? hovered : null;
  const previewItems = [...pinnedList.filter((p) => !p.equipped), ...(peek ? [peek] : [])];
  const preview = previewItems.map(asEquipped);
  const spotlight = peek ?? pinnedList[pinnedList.length - 1] ?? null;
  const previewColor = spotlight ? rc(spotlight.rarity) : data.goal ? rc(data.goal.item.rarity) : "var(--color-gold)";

  return (
    <div className="space-y-5" data-testid="wardrobe">
      {/* the fitting room */}
      <div
        className="hud-panel clip-corner relative overflow-hidden p-4 sm:p-5"
        style={{ background: `radial-gradient(ellipse at 50% 90%, ${previewColor}22, transparent 60%), var(--color-panel)` }}
      >
        <div className="flex flex-col items-center gap-4 lg:flex-row lg:items-stretch">
          <div className="relative shrink-0" data-testid="wardrobe-stage">
            <Hero level={data.level} loadout={data.loadout} preview={preview} mood={mood} size={260} interactive frame="wide" burst={burst} burstColor={burstColor} />
            <div className="pointer-events-none absolute inset-x-0 bottom-1 text-center font-display text-[10px] uppercase tracking-[0.25em] text-faint">
              {t("wardrobe.dragHint")}
            </div>
            <AnimatePresence>
              {peek && (
                <motion.div
                  initial={{ opacity: 0, y: -6 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0 }}
                  className="pointer-events-none absolute left-1/2 top-2 -translate-x-1/2 whitespace-nowrap rounded-sm border px-2 py-0.5 font-display text-[10px] uppercase tracking-wider"
                  style={{ borderColor: previewColor, color: previewColor, background: "rgba(0,0,0,0.45)" }}
                  data-testid="wardrobe-peek"
                >
                  {t("wardrobe.peek", { name: peek.name })}
                </motion.div>
              )}
            </AnimatePresence>
          </div>

          <div className="flex min-w-0 flex-1 flex-col justify-center gap-3">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <div>
                <div className="font-display text-[10px] uppercase tracking-[0.3em] text-muted">{t("wardrobe.title")}</div>
                <div className="text-sm text-faint">{t("wardrobe.hint")}</div>
              </div>
              <div className="flex items-center gap-3 text-sm">
                <span className="font-display text-[11px] uppercase tracking-wider text-faint">{t("wardrobe.level", { level: data.level })}</span>
                <span className="tabnum inline-flex items-center gap-1 font-bold" style={{ color: "var(--color-goldhi)" }} data-testid="wardrobe-gold">
                  <Coins size={14} /> {data.balance}g
                </span>
              </div>
            </div>

            <AnimatePresence initial={false}>
              {pinnedList.length > 0 && (
                <motion.div
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: "auto" }}
                  exit={{ opacity: 0, height: 0 }}
                  className="overflow-hidden"
                  data-testid="tryon-tray"
                >
                  <div className="flex flex-wrap items-center gap-1.5 rounded-sm border border-dashed px-2.5 py-2" style={{ borderColor: `${previewColor}66` }}>
                    <span className="inline-flex items-center gap-1 font-display text-[10px] uppercase tracking-[0.2em] text-muted">
                      <Eye size={12} /> {t("wardrobe.tryingOn")}
                    </span>
                    {pinnedList.map((p) => {
                      const c = rc(p.rarity);
                      return (
                        <span key={p.key} className="inline-flex items-center gap-1 rounded-sm border px-1.5 py-0.5 text-[11px]" style={{ borderColor: `${c}80`, color: c }} data-testid={`tray-${p.key}`}>
                          {p.name}
                          {!p.owned && <span className="tabnum text-faint">{p.price}g</span>}
                          <button type="button" className="text-faint hover:text-ink" onClick={() => unpinSlot(p.slot)} aria-label={t("wardrobe.takeOffPreview", { name: p.name })} data-testid={`tray-remove-${p.key}`}>
                            <X size={11} />
                          </button>
                        </span>
                      );
                    })}
                    <button type="button" className="ml-auto font-display text-[10px] uppercase tracking-wider text-faint hover:text-ink" onClick={() => setPins({})} data-testid="tray-clear">
                      {t("wardrobe.clearPreview")}
                    </button>
                  </div>
                </motion.div>
              )}
            </AnimatePresence>

            <GoalPanel data={data} flow={flow} />

            <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-4 lg:grid-cols-2 xl:grid-cols-4" data-testid="wardrobe-slots">
              {data.slots.map((slot) => {
                const piece = worn.get(slot);
                const trying = pins[slot] && !pins[slot]?.equipped ? pins[slot] : undefined;
                const Icon = SLOT_ICON[slot];
                const color = trying ? rc(trying.rarity) : piece ? rc(piece.rarity) : "var(--color-faint)";
                return (
                  <div
                    key={slot}
                    className="flex items-center gap-2 rounded-sm border border-edge px-2 py-1.5"
                    style={{ borderColor: trying || piece ? `${color}66` : undefined, borderStyle: trying ? "dashed" : undefined }}
                    data-testid={`slot-${slot}`}
                    data-trying={trying ? trying.key : undefined}
                  >
                    <Icon size={14} style={{ color }} />
                    <div className="min-w-0 flex-1">
                      <div className="font-display text-[9px] uppercase tracking-wider text-faint">
                        {t(`slot.${slot}` as MessageKey)}
                        {trying && <span style={{ color }}> · {t("wardrobe.onHero")}</span>}
                      </div>
                      <div className="truncate text-xs text-ink">{trying ? trying.name : piece ? piece.name : <span className="text-faint">{t("wardrobe.levelLook")}</span>}</div>
                    </div>
                    {trying ? (
                      <button type="button" onClick={() => unpinSlot(slot)} className="text-faint hover:text-ink" title={t("wardrobe.takeOffPreview", { name: trying.name })} aria-label={t("wardrobe.takeOffPreview", { name: trying.name })} data-testid={`slot-unpin-${slot}`}>
                        <X size={13} />
                      </button>
                    ) : piece && (
                      <button type="button" onClick={() => unequip.mutate(slot)} className="text-faint hover:text-ink" title={t("wardrobe.unequip")} aria-label={t("wardrobe.unequipItem", { name: piece.name })} data-testid={`unequip-${slot}`}>
                        <X size={13} />
                      </button>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      </div>

      {/* featured strip */}
      <div className="grid grid-cols-1 gap-3 md:grid-cols-3" data-testid="featured-strip">
        <DealCard data={data} flow={flow} onPin={togglePin} onHover={setHovered} />
        <NextUnlockCard data={data} onPin={togglePin} onHover={setHovered} />
        <CollectionCard
          data={data}
          onFilterSet={(name) => {
            setSetFilterName((cur) => (cur === name ? null : name));
            setFilter("all");
          }}
        />
      </div>

      {/* filters */}
      <div className="flex flex-wrap items-center gap-1.5" role="tablist" aria-label={t("wardrobe.filter")}>
        {(["all", ...data.slots] as const).map((slot) => {
          const active = filter === slot && !setFilterName;
          return (
            <button
              key={slot}
              role="tab"
              aria-selected={active}
              onClick={() => {
                setFilter(slot);
                setSetFilterName(null);
              }}
              className="rounded-sm border px-2.5 py-1 font-display text-[11px] uppercase tracking-wider transition-colors"
              style={{
                borderColor: active ? "var(--color-gold)" : "var(--color-edge)",
                color: active ? "var(--color-goldhi)" : "var(--color-muted)",
                background: active ? "rgba(var(--gold-rgb),0.08)" : "transparent",
              }}
              data-testid={`gear-filter-${slot}`}
            >
              {slot === "all" ? t("wardrobe.all") : t(`slot.${slot}` as MessageKey)}
            </button>
          );
        })}
        {setFilterName && (
          <button
            type="button"
            onClick={() => setSetFilterName(null)}
            className="inline-flex items-center gap-1 rounded-sm border px-2.5 py-1 font-display text-[11px] uppercase tracking-wider"
            style={{ borderColor: "var(--color-gold)", color: "var(--color-goldhi)", background: "rgba(var(--gold-rgb),0.08)" }}
            data-testid="set-filter-active"
          >
            {t(`set.${setFilterName}` as MessageKey)} <X size={11} />
          </button>
        )}
        <label className="ml-auto inline-flex cursor-pointer items-center gap-1.5 font-display text-[11px] uppercase tracking-wider text-muted">
          <input type="checkbox" checked={affordableOnly} onChange={(e) => setAffordableOnly(e.target.checked)} className="accent-[var(--color-gold)]" data-testid="affordable-toggle" />
          {t("wardrobe.affordable")}
        </label>
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {items.map((it) => (
          <GearCard key={it.key} item={it} data={data} pinned={pins[it.slot]?.key === it.key} onPin={togglePin} onHover={setHovered} flow={flow} />
        ))}
      </div>

      <AnimatePresence>{acquired && <AcquiredModal result={acquired} level={data.level} hasGoal={!!data.goal} onClose={() => setAcquired(null)} />}</AnimatePresence>
    </div>
  );
}
