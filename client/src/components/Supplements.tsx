import { useMemo, useState } from "react";
import type { FormEvent } from "react";
import { Check, Loader2, Pencil, Pill, Plus, Star, Trash2, X } from "lucide-react";
import {
  useAddSupplement,
  useArchiveSupplement,
  useSupplementHistory,
  useSupplements,
  useTakeSupplement,
  useUpdateSupplement,
} from "../lib/queries";
import { useReward } from "../lib/reward";
import { pushToast } from "../lib/toast";
import { formatTime } from "../lib/format";
import { useI18n } from "../lib/i18n";
import type { Supplement, SupplementDay } from "../lib/types";
import { Btn, EmptyState, RewardChips, SectionTitle, Spinner } from "./ui";

// Supplements: the daily stack. Tick what you took; every tick pays a small
// reward and the tick that completes the stack pays the once-a-day bonus.
// Taking is final (no un-take, no clawback) — same rule as quests and journal.
export function Supplements() {
  const { t } = useI18n();
  const { data: today, isLoading, isError } = useSupplements();

  if (isLoading) return <Spinner />;
  if (isError || !today) {
    return <EmptyState title={t("common.backendUnreachable")} hint={t("common.backendHint")} />;
  }

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-1 font-display text-[11px] uppercase tracking-[0.24em] text-[var(--color-health)]">
        {t("supps.overline")}
      </div>
      <h1 className="mb-1 font-display text-3xl leading-tight text-ink">{t("supps.title")}</h1>
      <p className="mb-5 text-sm text-muted">{t("supps.subtitle")}</p>

      <ProgressPanel today={today} />

      <div className="mt-5 space-y-2" data-testid="supplement-list">
        {today.supplements.length === 0 ? (
          <EmptyState icon={<Pill size={22} />} title={t("supps.empty")} hint={t("supps.emptyHint")} />
        ) : (
          today.supplements.map((sp) => <SupplementRow key={sp.id} supplement={sp} />)
        )}
      </div>

      <AddForm />

      <HistoryPanel />
    </div>
  );
}

// --- today's progress -----------------------------------------------------------

function ProgressPanel({
  today,
}: {
  today: {
    taken: number;
    total: number;
    bonus_awarded: boolean;
    item_rewards: Record<string, number>;
    bonus_rewards: Record<string, number>;
  };
}) {
  const { t } = useI18n();
  const ratio = today.total > 0 ? today.taken / today.total : 0;
  return (
    <div className="hud-panel clip-corner px-4 py-3" data-testid="supplement-progress">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="tabnum font-display text-lg text-ink">
          {t("supps.progress", { taken: today.taken, total: today.total })}
        </div>
        {today.bonus_awarded ? (
          <span
            className="inline-flex items-center gap-1 rounded-sm px-2 py-0.5 text-[11px] font-medium uppercase tracking-wider"
            style={{ background: "rgba(255,176,0,0.12)", color: "var(--color-goldhi)" }}
            data-testid="supplement-bonus-paid"
          >
            <Star size={12} /> {t("supps.bonusPaid")}
          </span>
        ) : (
          today.total > 0 && <span className="text-[11px] text-faint">{t("supps.bonusPending")}</span>
        )}
      </div>
      <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-white/[0.06]">
        <div
          className="h-full rounded-full transition-[width] duration-300"
          style={{
            width: `${Math.round(ratio * 100)}%`,
            background: today.bonus_awarded ? "var(--color-gold)" : "var(--color-health)",
          }}
        />
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-2 text-[11px] text-faint">
        <div className="flex items-center gap-2">
          <span className="uppercase tracking-wider">{t("supps.perItem")}</span>
          <RewardChips rewards={today.item_rewards} />
        </div>
        <div className="flex items-center gap-2">
          <span className="uppercase tracking-wider">{t("supps.fullStack")}</span>
          <RewardChips rewards={today.bonus_rewards} />
        </div>
      </div>
    </div>
  );
}

// --- one supplement -------------------------------------------------------------

function SupplementRow({ supplement: sp }: { supplement: Supplement }) {
  const { t } = useI18n();
  const { celebrate } = useReward();
  const take = useTakeSupplement();
  const archive = useArchiveSupplement();
  const [editing, setEditing] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);

  const onTake = () => {
    take.mutate(sp.id, {
      onSuccess: (res) => {
        celebrate({
          title: res.bonus_awarded ? t("supps.rewardFullStack") : sp.name,
          xp_events: res.xp_events,
          level_ups: res.level_ups,
          label: res.bonus_awarded ? sp.name : t("supps.rewardLabel"),
          gold: res.gold,
          level: res.dashboard.character.level,
        });
      },
    });
  };

  if (editing) {
    return <EditForm supplement={sp} onDone={() => setEditing(false)} />;
  }

  return (
    <div
      className={`hud-panel clip-corner group flex items-center gap-3 px-3 py-2.5 ${sp.taken ? "opacity-80" : "hud-panel-hover"}`}
      data-testid={`supplement-${sp.id}`}
      data-taken={sp.taken ? "1" : "0"}
    >
      <button
        onClick={onTake}
        disabled={sp.taken || take.isPending}
        aria-label={sp.taken ? t("supps.takenAt", { time: sp.taken_at ? formatTime(sp.taken_at) : "" }) : t("supps.take")}
        data-testid={`take-${sp.id}`}
        className="grid h-9 w-9 shrink-0 place-items-center rounded-sm border transition-colors disabled:cursor-default"
        style={{
          borderColor: sp.taken ? "var(--color-health)" : "var(--color-edge2)",
          background: sp.taken ? "rgba(75,255,126,0.14)" : "transparent",
          color: sp.taken ? "var(--color-health)" : "var(--color-faint)",
        }}
      >
        {take.isPending ? <Loader2 size={16} className="animate-spin" /> : sp.taken ? <Check size={18} /> : <Pill size={16} />}
      </button>

      <div className="min-w-0 flex-1">
        <div className={`truncate text-sm font-medium ${sp.taken ? "text-muted line-through decoration-white/20" : "text-ink"}`}>
          {sp.name}
        </div>
        <div className="truncate text-[11px] text-faint">
          {sp.taken && sp.taken_at ? t("supps.takenAt", { time: formatTime(sp.taken_at) }) : sp.dose}
        </div>
      </div>

      {confirmRemove ? (
        <div className="flex items-center gap-1.5 text-[11px]">
          <span className="text-muted">{t("supps.removeConfirm")}</span>
          <Btn
            variant="danger"
            className="!px-2 !py-1 !text-[11px]"
            disabled={archive.isPending}
            data-testid={`remove-confirm-${sp.id}`}
            onClick={() =>
              archive.mutate(sp.id, {
                onSuccess: () => pushToast(t("supps.removed", { name: sp.name }), "info"),
              })
            }
          >
            {t("supps.removeYes")}
          </Btn>
          <button onClick={() => setConfirmRemove(false)} className="text-faint hover:text-ink" aria-label={t("common.cancel")}>
            <X size={14} />
          </button>
        </div>
      ) : (
        <div className="flex items-center gap-1 opacity-60 transition-opacity group-hover:opacity-100">
          {!sp.taken && (
            <Btn variant="primary" className="!px-2.5 !py-1 !text-[11px]" onClick={onTake} disabled={take.isPending}>
              {t("supps.take")}
            </Btn>
          )}
          <button
            onClick={() => setEditing(true)}
            className="rounded-sm p-1.5 text-faint hover:text-ink"
            aria-label={t("supps.edit")}
            title={t("supps.edit")}
            data-testid={`edit-${sp.id}`}
          >
            <Pencil size={14} />
          </button>
          <button
            onClick={() => setConfirmRemove(true)}
            className="rounded-sm p-1.5 text-faint hover:text-[#ff8a80]"
            aria-label={t("supps.remove")}
            title={t("supps.remove")}
            data-testid={`remove-${sp.id}`}
          >
            <Trash2 size={14} />
          </button>
        </div>
      )}
    </div>
  );
}

const inputClass =
  "w-full rounded-sm border border-edge bg-white/[0.03] px-3 py-2 text-sm text-ink placeholder:text-faint focus:border-edge2 focus:outline-none";

function EditForm({ supplement: sp, onDone }: { supplement: Supplement; onDone: () => void }) {
  const { t } = useI18n();
  const update = useUpdateSupplement();
  const [name, setName] = useState(sp.name);
  const [dose, setDose] = useState(sp.dose);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    update.mutate(
      { id: sp.id, input: { name, dose } },
      {
        onSuccess: () => {
          pushToast(t("supps.updated"), "success");
          onDone();
        },
      },
    );
  };

  return (
    <form onSubmit={submit} className="hud-panel clip-corner flex flex-wrap items-center gap-2 px-3 py-2.5" data-testid={`edit-form-${sp.id}`}>
      <input
        className={`${inputClass} min-w-0 flex-1`}
        value={name}
        onChange={(e) => setName(e.target.value)}
        maxLength={60}
        autoFocus
        aria-label={t("supps.namePlaceholder")}
      />
      <input
        className={`${inputClass} w-32`}
        value={dose}
        onChange={(e) => setDose(e.target.value)}
        maxLength={40}
        placeholder={t("supps.dosePlaceholder")}
        aria-label={t("supps.dosePlaceholder")}
      />
      <Btn type="submit" variant="primary" className="!px-2.5 !py-1.5 !text-[11px]" disabled={update.isPending || !name.trim()}>
        {t("common.saveChanges")}
      </Btn>
      <button type="button" onClick={onDone} className="text-faint hover:text-ink" aria-label={t("common.cancel")}>
        <X size={16} />
      </button>
    </form>
  );
}

function AddForm() {
  const { t } = useI18n();
  const add = useAddSupplement();
  const [name, setName] = useState("");
  const [dose, setDose] = useState("");

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    add.mutate(
      { name, dose },
      {
        onSuccess: (sp) => {
          pushToast(t("supps.added", { name: sp.name }), "success");
          setName("");
          setDose("");
        },
      },
    );
  };

  return (
    <form onSubmit={submit} className="mt-4 flex flex-wrap items-center gap-2" data-testid="supplement-add-form">
      <input
        className={`${inputClass} min-w-0 flex-1`}
        value={name}
        onChange={(e) => setName(e.target.value)}
        maxLength={60}
        placeholder={t("supps.namePlaceholder")}
        aria-label={t("supps.namePlaceholder")}
        data-testid="supplement-name"
      />
      <input
        className={`${inputClass} w-36`}
        value={dose}
        onChange={(e) => setDose(e.target.value)}
        maxLength={40}
        placeholder={t("supps.dosePlaceholder")}
        aria-label={t("supps.dosePlaceholder")}
        data-testid="supplement-dose"
      />
      <Btn type="submit" variant="primary" disabled={add.isPending || !name.trim()} data-testid="supplement-add">
        {add.isPending ? <Loader2 size={14} className="animate-spin" /> : <Plus size={14} />} {t("common.add")}
      </Btn>
    </form>
  );
}

// --- history --------------------------------------------------------------------

function dayKey(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

// Consecutive full-stack days ending today or yesterday (today may be in progress).
function fullStackStreak(days: Map<string, SupplementDay>): number {
  const cursor = new Date();
  if (!days.get(dayKey(cursor))?.bonus) cursor.setDate(cursor.getDate() - 1);
  let n = 0;
  while (days.get(dayKey(cursor))?.bonus) {
    n++;
    cursor.setDate(cursor.getDate() - 1);
  }
  return n;
}

function HistoryPanel() {
  const { t, tp } = useI18n();
  const { data: history } = useSupplementHistory(70);
  const days = useMemo(() => new Map((history ?? []).map((d) => [d.day, d])), [history]);
  const streak = useMemo(() => fullStackStreak(days), [days]);
  const maxTaken = useMemo(() => Math.max(1, ...(history ?? []).map((d) => d.taken)), [history]);

  const weeks = 10;
  const today = new Date();
  const start = new Date(today);
  start.setDate(today.getDate() - ((today.getDay() + 6) % 7) - (weeks - 1) * 7);
  const cols: { date: Date; key: string }[][] = [];
  for (let w = 0; w < weeks; w++) {
    const col: { date: Date; key: string }[] = [];
    for (let d = 0; d < 7; d++) {
      const date = new Date(start);
      date.setDate(start.getDate() + w * 7 + d);
      col.push({ date, key: dayKey(date) });
    }
    cols.push(col);
  }

  return (
    <div className="mt-8">
      <SectionTitle hint={t("supps.historyHint")}>{t("supps.history")}</SectionTitle>
      <div className="hud-panel clip-corner px-4 py-3">
        <div className="mb-3 flex items-center gap-2 text-sm">
          <Star size={14} style={{ color: streak > 0 ? "var(--color-goldhi)" : "var(--color-faint)" }} />
          <span className={streak > 0 ? "text-ink" : "text-faint"} data-testid="fullstack-streak">
            {tp("supps.fullStackStreak", streak)}
          </span>
        </div>
        <div className="flex items-center gap-3 overflow-x-auto">
          <span className="w-14 shrink-0 text-[10px] uppercase tracking-wider text-faint">{t("supps.weeks", { n: weeks })}</span>
          <div className="flex gap-[3px]">
            {cols.map((col, w) => (
              <div key={w} className="flex flex-col gap-[3px]">
                {col.map(({ date, key }) => {
                  const d = days.get(key);
                  const future = date > today;
                  const bg = future
                    ? "transparent"
                    : d?.bonus
                      ? "var(--color-gold)"
                      : d
                        ? `rgba(75,255,126,${(0.25 + (d.taken / maxTaken) * 0.6).toFixed(2)})`
                        : "rgba(255,255,255,0.05)";
                  return (
                    <div
                      key={key}
                      className="h-[10px] w-[10px] rounded-[2px]"
                      style={{ background: bg }}
                      title={
                        d
                          ? t("supps.cellTitle", { date: key, taken: d.taken, bonus: d.bonus ? t("supps.cellBonus") : "" })
                          : future
                            ? ""
                            : t("supps.noEntry", { date: key })
                      }
                    />
                  );
                })}
              </div>
            ))}
          </div>
          <div className="flex items-center gap-1 text-[9px] text-faint">
            <span className="h-[8px] w-[8px] rounded-[2px]" style={{ background: "rgba(75,255,126,0.4)" }} />
            <span className="h-[8px] w-[8px] rounded-[2px]" style={{ background: "rgba(75,255,126,0.85)" }} />
            <span className="h-[8px] w-[8px] rounded-[2px]" style={{ background: "var(--color-gold)" }} />
            <span>★</span>
          </div>
        </div>
      </div>
    </div>
  );
}
