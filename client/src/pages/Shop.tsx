import { useState } from "react";
import { Archive, Coins, Plus, ShoppingCart } from "lucide-react";
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
import { relativeTime } from "../lib/format";
import type { ShopItem } from "../lib/types";

function ItemRow({ item, balance }: { item: ShopItem; balance: number }) {
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
        pushToast(`Purchased "${res.item.name}" for ${res.item.price}g — enjoy it, you earned it.`, "success"),
    });
  };

  return (
    <div className="hud-panel flex items-center gap-3 p-3.5" data-testid={`shop-item-${item.id}`}>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium text-ink">{item.name}</div>
        <div className="tabnum text-xs" style={{ color: "var(--color-gold)" }}>
          {item.price}g
        </div>
      </div>
      <Btn
        variant={affordable ? "primary" : "ghost"}
        disabled={!affordable || purchase.isPending}
        onClick={buy}
        data-testid={`buy-${item.id}`}
      >
        <ShoppingCart size={14} />
        {arming ? "Confirm?" : affordable ? "Buy" : "Too costly"}
      </Btn>
      <button
        onClick={() => archive.mutate(item.id)}
        className="text-faint transition-colors hover:text-ink"
        aria-label={`Archive ${item.name}`}
        title="Archive (remove from shop)"
      >
        <Archive size={16} />
      </button>
    </div>
  );
}

function AddItemForm() {
  const create = useCreateShopItem();
  const [name, setName] = useState("");
  const [price, setPrice] = useState("");

  const submit = () => {
    const p = parseInt(price, 10);
    if (!name.trim() || !p || p <= 0) {
      pushToast("A reward needs a name and a price above 0.", "error");
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
        placeholder='Reward, e.g. "Guilt-free gaming evening"'
        className="flex-1 rounded-md border border-edge bg-transparent px-3 py-2 text-sm text-ink outline-none placeholder:text-faint"
        data-testid="shop-name"
      />
      <input
        value={price}
        onChange={(e) => setPrice(e.target.value.replace(/\D/g, ""))}
        placeholder="Price (g)"
        inputMode="numeric"
        className="w-full rounded-md border border-edge bg-transparent px-3 py-2 text-sm text-ink outline-none placeholder:text-faint sm:w-28"
        data-testid="shop-price"
      />
      <Btn variant="primary" onClick={submit} disabled={create.isPending} data-testid="shop-add">
        <Plus size={14} />
        Add
      </Btn>
    </div>
  );
}

export function ShopPage() {
  const dashboard = useDashboard();
  const items = useShopItems();
  const ledger = useGoldEvents(30);

  if (dashboard.isLoading || items.isLoading) return <Spinner label="Opening the shop…" />;
  if (dashboard.isError || items.isError || !dashboard.data || !items.data) {
    return (
      <EmptyState
        title="Couldn't reach the backend"
        hint={((dashboard.error ?? items.error) as Error)?.message ?? "Is the Go server running on :8080?"}
      />
    );
  }

  const balance = dashboard.data.gold_balance;
  const purchases = (ledger.data ?? []).filter((e) => e.source === "purchase");

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-display text-xl font-bold tracking-tight text-ink">Reward Shop</h1>
          <p className="text-sm text-faint">XP is progress. Gold is permission — spend it guilt-free.</p>
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
        <SectionTitle hint="Define your own rewards; buy them with earned gold.">Wares</SectionTitle>
        {items.data.length === 0 ? (
          <EmptyState
            icon={<ShoppingCart size={20} />}
            title="The shop is empty"
            hint="Add real-life rewards above — a takeout night, a lazy morning, that gadget."
          />
        ) : (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {items.data.map((it) => (
              <ItemRow key={it.id} item={it} balance={balance} />
            ))}
          </div>
        )}
      </section>

      <section>
        <SectionTitle hint="Every purchase is a ledger entry — same audit trail as XP.">
          Purchase history
        </SectionTitle>
        {purchases.length === 0 ? (
          <EmptyState title="No purchases yet" hint="Earn gold by completing quests, then treat yourself." />
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
    </div>
  );
}
