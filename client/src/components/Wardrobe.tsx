// Wardrobe: the gear shop for the hero. A big interactive 3D hero up top
// (drag to rotate; hovering / selecting a piece tries it on), the seven
// slots, then the catalog grid. Buying is arm-then-confirm like the reward
// shop; the server loadout is the truth, the try-on is client state only.
import { useEffect, useMemo, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { Check, Coins, Crown, Lock, PawPrint, Shield, Shirt, Sparkles, Sword, Wind, X } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { useBuyCosmetic, useCosmetics, useEquipCosmetic, useUnequipCosmetic } from "../lib/queries";
import { rarityColor } from "../lib/theme";
import { pushToast } from "../lib/toast";
import { useI18n } from "../lib/i18n";
import type { MessageKey } from "../lib/locales/en";
import type { CosmeticItem, CosmeticSlot, EquippedCosmetic } from "../lib/types";
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

function GearCard({
  item,
  balance,
  selected,
  onSelect,
  onBought,
}: {
  item: CosmeticItem;
  balance: number;
  selected: boolean;
  onSelect: (item: CosmeticItem | null) => void;
  onBought: (item: CosmeticItem) => void;
}) {
  const { t } = useI18n();
  const buy = useBuyCosmetic();
  const equip = useEquipCosmetic();
  const [arming, setArming] = useState(false);
  const color = rarityColor[item.rarity] ?? rarityColor.common;
  const affordable = balance >= item.price;
  const rarityLabel = t(`rarity.${item.rarity}` as MessageKey);

  useEffect(() => {
    if (!arming) return;
    const id = window.setTimeout(() => setArming(false), 3500);
    return () => window.clearTimeout(id);
  }, [arming]);

  const doBuy = () => {
    if (!arming) {
      setArming(true);
      return;
    }
    setArming(false);
    buy.mutate(item.key, {
      onSuccess: (res) => {
        pushToast(t("wardrobe.bought", { name: res.item.name, price: res.item.price }), "success");
        onBought(res.item);
      },
    });
  };

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
    action = (
      <Btn
        variant={affordable ? "primary" : "ghost"}
        disabled={!affordable || buy.isPending}
        onClick={doBuy}
        title={affordable ? undefined : t("wardrobe.needGold", { n: item.price - balance })}
        data-testid={`gear-buy-${item.key}`}
      >
        {arming ? t("wardrobe.confirm", { price: item.price }) : affordable ? t("wardrobe.buy") : t("wardrobe.tooCostly")}
      </Btn>
    );
  }

  const Icon = SLOT_ICON[item.slot];
  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      className="hud-panel hud-panel-hover relative flex flex-col gap-2.5 p-3.5"
      style={{
        borderColor: selected ? color : item.equipped ? "rgba(var(--phos-rgb),0.5)" : undefined,
        boxShadow: selected ? `0 0 22px -8px ${color}` : undefined,
        opacity: item.unlocked || item.owned ? 1 : 0.7,
      }}
      data-testid={`gear-${item.key}`}
      data-rarity={item.rarity}
    >
      <button
        type="button"
        className="flex items-start gap-3 text-left focus:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-phos)]"
        onClick={() => onSelect(item)}
        onMouseEnter={() => onSelect(item)}
        aria-pressed={selected}
        aria-label={t("wardrobe.tryOn", { name: item.name })}
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
          </span>
          <span className="mt-0.5 block text-[11px] leading-snug text-muted">{item.flavor}</span>
        </span>
      </button>
      <div className="mt-auto flex items-center justify-between gap-2">
        {item.owned ? (
          <span className="font-display text-[10px] uppercase tracking-wider text-faint">{t("wardrobe.owned")}</span>
        ) : (
          <span className="tabnum inline-flex items-center gap-1 text-sm font-bold" style={{ color: "var(--color-gold)" }}>
            <Coins size={13} /> {item.price}g
          </span>
        )}
        {action}
      </div>
    </motion.div>
  );
}

export function Wardrobe() {
  const { t } = useI18n();
  const cat = useCosmetics();
  const unequip = useUnequipCosmetic();
  const [filter, setFilter] = useState<CosmeticSlot | "all">("all");
  const [selected, setSelected] = useState<CosmeticItem | null>(null);
  const [burst, setBurst] = useState(0);
  const [burstColor, setBurstColor] = useState<string | undefined>(undefined);
  const [mood, setMood] = useState<"idle" | "celebrate">("idle");

  const items = useMemo(() => (cat.data?.items ?? []).filter((it) => filter === "all" || it.slot === filter), [cat.data, filter]);

  if (cat.isLoading) return <Spinner label={t("wardrobe.loading")} />;
  if (cat.isError || !cat.data) {
    return <EmptyState title={t("common.backendUnreachable")} hint={(cat.error as Error)?.message ?? t("common.backendHint")} />;
  }
  const data = cat.data;
  const worn = new Map(data.loadout.map((e) => [e.slot, e]));
  const preview = selected && !selected.equipped ? asEquipped(selected) : null;
  const previewColor = selected ? rarityColor[selected.rarity] ?? rarityColor.common : "var(--color-gold)";

  const onBought = (item: CosmeticItem) => {
    setSelected(null);
    setBurstColor(item.color);
    setBurst((n) => n + 1);
    setMood("celebrate");
    window.setTimeout(() => setMood("idle"), 2600);
  };

  return (
    <div className="space-y-5" data-testid="wardrobe">
      {/* the fitting room */}
      <div
        className="hud-panel clip-corner relative overflow-hidden p-4 sm:p-5"
        style={{ background: `radial-gradient(ellipse at 50% 90%, ${previewColor}22, transparent 60%), var(--color-panel)` }}
      >
        <div className="flex flex-col items-center gap-4 lg:flex-row lg:items-stretch">
          <div className="relative shrink-0" data-testid="wardrobe-stage">
            <Hero
              level={data.level}
              loadout={data.loadout}
              preview={preview}
              mood={mood}
              size={260}
              interactive
              frame="wide"
              burst={burst}
              burstColor={burstColor}
            />
            <div className="pointer-events-none absolute inset-x-0 bottom-1 text-center font-display text-[10px] uppercase tracking-[0.25em] text-faint">
              {t("wardrobe.dragHint")}
            </div>
            <AnimatePresence>
              {preview && (
                <motion.div
                  initial={{ opacity: 0, y: -6 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0 }}
                  className="pointer-events-none absolute left-1/2 top-2 -translate-x-1/2 whitespace-nowrap rounded-sm border px-2 py-0.5 font-display text-[10px] uppercase tracking-wider"
                  style={{ borderColor: previewColor, color: previewColor, background: "rgba(0,0,0,0.45)" }}
                  data-testid="wardrobe-preview"
                >
                  {t("wardrobe.previewing", { name: preview.name })}
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
            <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-4 lg:grid-cols-2 xl:grid-cols-4" data-testid="wardrobe-slots">
              {data.slots.map((slot) => {
                const piece = worn.get(slot);
                const Icon = SLOT_ICON[slot];
                const color = piece ? rarityColor[piece.rarity] ?? rarityColor.common : "var(--color-faint)";
                return (
                  <div
                    key={slot}
                    className="flex items-center gap-2 rounded-sm border border-edge px-2 py-1.5"
                    style={{ borderColor: piece ? `${color}66` : undefined }}
                    data-testid={`slot-${slot}`}
                  >
                    <Icon size={14} style={{ color }} />
                    <div className="min-w-0 flex-1">
                      <div className="font-display text-[9px] uppercase tracking-wider text-faint">{t(`slot.${slot}` as MessageKey)}</div>
                      <div className="truncate text-xs text-ink">{piece ? piece.name : <span className="text-faint">{t("wardrobe.levelLook")}</span>}</div>
                    </div>
                    {piece && (
                      <button
                        type="button"
                        onClick={() => unequip.mutate(slot)}
                        className="text-faint hover:text-ink"
                        title={t("wardrobe.unequip")}
                        aria-label={t("wardrobe.unequipItem", { name: piece.name })}
                        data-testid={`unequip-${slot}`}
                      >
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

      {/* slot filter */}
      <div className="flex flex-wrap gap-1.5" role="tablist" aria-label={t("wardrobe.filter")}>
        {(["all", ...data.slots] as const).map((slot) => {
          const active = filter === slot;
          return (
            <button
              key={slot}
              role="tab"
              aria-selected={active}
              onClick={() => setFilter(slot)}
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
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3" onMouseLeave={() => setSelected(null)}>
        {items.map((it) => (
          <GearCard key={it.key} item={it} balance={data.balance} selected={selected?.key === it.key} onSelect={setSelected} onBought={onBought} />
        ))}
      </div>
    </div>
  );
}
