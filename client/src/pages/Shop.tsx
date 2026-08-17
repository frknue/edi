import { useEffect, useRef, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { Archive, CalendarDays, Coins, Plus, ShoppingCart, X } from "lucide-react";
import {
  useArchiveShopItem,
  useCreateShopItem,
  useDashboard,
  useGoldEvents,
  usePurchaseShopItem,
  useShopItems,
} from "../lib/queries";
import { pushToast } from "../lib/toast";
import { Btn, EmptyState, SectionTitle, Spinner } from "../components/ui";
import { formatDateTime, relativeTime } from "../lib/format";
import type { ShopItem } from "../lib/types";
import { useI18n } from "../lib/i18n";

function ShopItemModal({
  item,
  balance,
  onClose,
}: {
  item: ShopItem;
  balance: number;
  onClose: () => void;
}) {
  const { t } = useI18n();
  const purchase = usePurchaseShopItem();
  const [arming, setArming] = useState(false);
  const closeButton = useRef<HTMLButtonElement>(null);
  const dialog = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setArming(false);
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const frame = window.requestAnimationFrame(() => closeButton.current?.focus());

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
        return;
      }
      if (event.key !== "Tab" || !dialog.current) return;

      const focusable = Array.from(
        dialog.current.querySelectorAll<HTMLElement>("button:not(:disabled), [href], input, select, textarea"),
      );
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (!first || !last) return;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.cancelAnimationFrame(frame);
      window.removeEventListener("keydown", onKeyDown);
      previousFocus?.focus();
    };
  }, [item, onClose]);

  const affordable = balance >= item.price;
  const shortfall = Math.max(0, item.price - balance);

  const buy = () => {
    if (!arming) {
      setArming(true);
      return;
    }
    setArming(false);
    purchase.mutate(item.id, {
      onSuccess: (res) => {
        pushToast(t("shop.purchased", { name: res.item.name, price: res.item.price }), "success");
        onClose();
      },
    });
  };

  return (
    <motion.div
      className="sheet-safe fixed inset-0 z-50 flex items-end justify-center p-0 sm:items-center sm:p-6"
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      style={{ background: "rgba(4,5,9,0.76)", backdropFilter: "blur(4px)" }}
      onClick={onClose}
      data-testid="shop-item-modal"
    >
      <motion.div
        ref={dialog}
        role="dialog"
        aria-modal="true"
        aria-labelledby="shop-item-modal-title"
        aria-describedby="shop-item-modal-status"
        className="hud-panel w-full max-w-md overflow-hidden"
        initial={{ y: 30, opacity: 0, scale: 0.98 }}
        animate={{ y: 0, opacity: 1, scale: 1 }}
        exit={{ y: 20, opacity: 0 }}
        transition={{ type: "spring", stiffness: 300, damping: 28 }}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-edge px-5 py-3.5">
          <h2
            id="shop-item-modal-title"
            className="font-display text-sm font-semibold uppercase tracking-[0.18em] text-ink"
          >
            {t("shop.details")}
          </h2>
          <button
            ref={closeButton}
            type="button"
            onClick={onClose}
            className="rounded-sm text-faint hover:text-ink focus:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-phos)]"
            aria-label={t("shop.closeDetails")}
            data-testid="shop-item-modal-close"
          >
            <X size={18} />
          </button>
        </div>

        <div className="space-y-5 px-5 py-5">
          <div>
            <p className="mb-1 font-display text-xs uppercase tracking-[0.14em] text-faint">{t("shop.reward")}</p>
            <p className="break-words text-lg font-semibold leading-snug text-ink">{item.name}</p>
          </div>

          <dl className="grid grid-cols-2 gap-3">
            <div className="rounded-sm border border-edge bg-white/[0.02] p-3">
              <dt className="mb-1 flex items-center gap-1.5 text-[11px] uppercase tracking-wide text-faint">
                <Coins size={13} />
                {t("shop.price")}
              </dt>
              <dd className="tabnum text-lg font-bold text-[var(--color-goldhi)]">{item.price}g</dd>
            </div>
            <div className="rounded-sm border border-edge bg-white/[0.02] p-3">
              <dt className="mb-1 flex items-center gap-1.5 text-[11px] uppercase tracking-wide text-faint">
                <Coins size={13} />
                {t("shop.balance")}
              </dt>
              <dd className="tabnum text-lg font-bold text-ink">{balance}g</dd>
            </div>
          </dl>

          <div className="flex items-start gap-2.5 text-xs text-muted">
            <CalendarDays size={15} className="mt-0.5 shrink-0 text-faint" />
            <div>
              <p className="text-faint">{t("shop.addedOn")}</p>
              <p>
                {formatDateTime(item.created_at)} · {relativeTime(item.created_at)}
              </p>
            </div>
          </div>

          <div
            id="shop-item-modal-status"
            className="rounded-sm border px-3 py-2.5 text-xs"
            style={{
              borderColor: affordable ? "var(--color-edge2)" : "rgba(255,176,0,0.3)",
              background: affordable ? "rgba(75,255,126,0.04)" : "rgba(255,176,0,0.04)",
              color: affordable ? "var(--color-muted)" : "var(--color-gold)",
            }}
          >
            {affordable ? t("shop.enough") : t("shop.short", { n: shortfall })}
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-edge px-5 py-3.5">
          <Btn variant="ghost" onClick={onClose}>
            {t("common.close")}
          </Btn>
          <Btn
            variant={affordable ? "primary" : "ghost"}
            disabled={!affordable || purchase.isPending}
            onClick={buy}
            data-testid="shop-item-modal-buy"
          >
            <ShoppingCart size={14} />
            {arming ? t("shop.confirm") : affordable ? t("shop.buyReward") : t("shop.tooCostly")}
          </Btn>
        </div>
      </motion.div>
    </motion.div>
  );
}

function ItemRow({
  item,
  balance,
  onOpen,
}: {
  item: ShopItem;
  balance: number;
  onOpen: (item: ShopItem) => void;
}) {
  const { t } = useI18n();
  const purchase = usePurchaseShopItem();
  const archive = useArchiveShopItem();
  const [arming, setArming] = useState(false);
  const affordable = balance >= item.price;

  const buy = () => {
    if (!arming) {
      setArming(true);
      window.setTimeout(() => setArming(false), 3000);
      return;
    }
    setArming(false);
    purchase.mutate(item.id, {
      onSuccess: (res) =>
        pushToast(t("shop.purchased", { name: res.item.name, price: res.item.price }), "success"),
    });
  };

  return (
    <div className="hud-panel hud-panel-hover flex items-center gap-3 p-3.5" data-testid={`shop-item-${item.id}`}>
      <button
        type="button"
        onClick={() => onOpen(item)}
        className="min-w-0 flex-1 cursor-pointer rounded-sm text-left focus:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-phos)]"
        aria-label={t("shop.viewDetails", { name: item.name })}
        aria-haspopup="dialog"
        data-testid={`shop-item-details-${item.id}`}
      >
        <div className="truncate text-sm font-medium text-ink">{item.name}</div>
        <div className="tabnum text-xs" style={{ color: "var(--color-gold)" }}>
          {item.price}g
        </div>
      </button>
      <Btn
        variant={affordable ? "primary" : "ghost"}
        disabled={!affordable || purchase.isPending}
        onClick={buy}
        data-testid={`buy-${item.id}`}
      >
        <ShoppingCart size={14} />
        {arming ? t("shop.confirm") : affordable ? t("shop.buy") : t("shop.tooCostly")}
      </Btn>
      <button
        onClick={() => archive.mutate(item.id)}
        className="text-faint transition-colors hover:text-ink"
        aria-label={t("shop.archiveItem", { name: item.name })}
        title={t("shop.archiveTitle")}
      >
        <Archive size={16} />
      </button>
    </div>
  );
}

function AddItemForm() {
  const { t } = useI18n();
  const create = useCreateShopItem();
  const [name, setName] = useState("");
  const [price, setPrice] = useState("");

  const submit = () => {
    const p = parseInt(price, 10);
    if (!name.trim() || !p || p <= 0) {
      pushToast(t("shop.needNamePrice"), "error");
      return;
    }
    create.mutate(
      { name: name.trim(), price: p },
      {
        onSuccess: () => {
          setName("");
          setPrice("");
        },
      },
    );
  };

  return (
    <div className="hud-panel flex flex-col gap-2 p-3.5 sm:flex-row sm:items-center">
      <input
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder={t("shop.namePlaceholder")}
        className="flex-1 rounded-md border border-edge bg-transparent px-3 py-2 text-sm text-ink outline-none placeholder:text-faint"
        data-testid="shop-name"
      />
      <input
        value={price}
        onChange={(e) => setPrice(e.target.value.replace(/\D/g, ""))}
        placeholder={t("shop.pricePlaceholder")}
        inputMode="numeric"
        className="w-full rounded-md border border-edge bg-transparent px-3 py-2 text-sm text-ink outline-none placeholder:text-faint sm:w-28"
        data-testid="shop-price"
      />
      <Btn variant="primary" onClick={submit} disabled={create.isPending} data-testid="shop-add">
        <Plus size={14} />
        {t("common.add")}
      </Btn>
    </div>
  );
}

export function ShopPage() {
  const { t } = useI18n();
  const dashboard = useDashboard();
  const items = useShopItems();
  const ledger = useGoldEvents(30, "purchase");
  const [selectedItem, setSelectedItem] = useState<ShopItem | null>(null);

  if (dashboard.isLoading || items.isLoading) return <Spinner label={t("shop.opening")} />;
  if (dashboard.isError || items.isError || !dashboard.data || !items.data) {
    return (
      <EmptyState
        title={t("common.backendUnreachable")}
        hint={((dashboard.error ?? items.error) as Error)?.message ?? t("common.backendHint")}
      />
    );
  }

  const balance = dashboard.data.gold_balance;
  const purchases = ledger.data ?? [];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-display text-xl font-bold tracking-tight text-ink">{t("shop.title")}</h1>
          <p className="text-sm text-faint">{t("shop.subtitle")}</p>
        </div>
        <div className="flex items-center gap-2" data-testid="gold-balance">
          <Coins size={20} style={{ color: "var(--color-gold)" }} />
          <span className="tabnum text-2xl font-bold" style={{ color: "var(--color-goldhi)" }}>
            {balance}g
          </span>
        </div>
      </div>

      <AddItemForm />

      <section>
        <SectionTitle hint={t("shop.waresHint")}>{t("shop.wares")}</SectionTitle>
        {items.data.length === 0 ? (
          <EmptyState
            icon={<ShoppingCart size={20} />}
            title={t("shop.empty")}
            hint={t("shop.emptyHint")}
          />
        ) : (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {items.data.map((it) => (
              <ItemRow key={it.id} item={it} balance={balance} onOpen={setSelectedItem} />
            ))}
          </div>
        )}
      </section>

      <section>
        <SectionTitle hint={t("shop.ledgerHint")}>{t("shop.recentPurchases")}</SectionTitle>
        {ledger.isError ? (
          <EmptyState
            title={t("shop.ledgerError")}
            hint={(ledger.error as Error)?.message ?? t("common.backendHint")}
          />
        ) : purchases.length === 0 ? (
          <EmptyState title={t("shop.noPurchases")} hint={t("shop.noPurchasesHint")} />
        ) : (
          <div className="space-y-1.5">
            {purchases.map((e) => (
              <div key={e.id} className="flex items-center justify-between rounded-md border border-edge px-3 py-2 text-sm">
                <span className="text-ink">{e.label}</span>
                <span className="flex items-center gap-3">
                  <span className="tabnum" style={{ color: "var(--color-gold)" }}>
                    {e.amount}g
                  </span>
                  <span className="text-xs text-faint">{relativeTime(e.created_at)}</span>
                </span>
              </div>
            ))}
          </div>
        )}
      </section>

      <AnimatePresence>
        {selectedItem && (
          <ShopItemModal item={selectedItem} balance={balance} onClose={() => setSelectedItem(null)} />
        )}
      </AnimatePresence>
    </div>
  );
}
