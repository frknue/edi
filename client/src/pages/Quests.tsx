import { useState } from "react";
import { Plus, Scroll, Sparkles } from "lucide-react";
import {
  useQuests,
  useCreateQuest,
  useRecordSpontaneousQuest,
  useUpdateQuest,
  useCompleteQuest,
  useSkipQuest,
  useArchiveQuest,
} from "../lib/queries";
import { useReward } from "../lib/reward";
import { pushToast } from "../lib/toast";
import { QuestCard } from "../components/QuestCard";
import { QuestFormModal } from "../components/QuestFormModal";
import { Btn, EmptyState, SectionTitle, Spinner } from "../components/ui";
import { typeMeta, getType } from "../lib/theme";
import type { Quest, QuestInput, QuestType } from "../lib/types";
import { useI18n } from "../lib/i18n";
import type { MessageKey } from "../lib/locales/en";

const TYPE_FILTERS: ("all" | QuestType)[] = ["all", ...(Object.keys(typeMeta) as QuestType[])];
const STATUS_FILTERS = ["active", "completed", "skipped", "archived", "all"] as const;

export function QuestsPage() {
  const { t, tp } = useI18n();
  const [typeFilter, setTypeFilter] = useState<(typeof TYPE_FILTERS)[number]>("all");
  const [statusFilter, setStatusFilter] = useState<(typeof STATUS_FILTERS)[number]>("active");
  const [modalOpen, setModalOpen] = useState(false);
  const [winModalOpen, setWinModalOpen] = useState(false);
  const [editing, setEditing] = useState<Quest | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const filters = {
    type: typeFilter === "all" ? undefined : typeFilter,
    status: statusFilter === "all" ? undefined : statusFilter,
  };
  const { data: quests, isLoading } = useQuests(filters);

  const create = useCreateQuest();
  const recordWin = useRecordSpontaneousQuest();
  const update = useUpdateQuest();
  const complete = useCompleteQuest();
  const skip = useSkipQuest();
  const archive = useArchiveQuest();
  const { celebrate } = useReward();

  const busy = create.isPending || recordWin.isPending || update.isPending || complete.isPending || skip.isPending || archive.isPending;

  const openCreate = () => {
    setEditing(null);
    setFormError(null);
    setModalOpen(true);
  };
  const openEdit = (q: Quest) => {
    setEditing(q);
    setFormError(null);
    setModalOpen(true);
  };
  const openWin = () => {
    setFormError(null);
    setWinModalOpen(true);
  };

  const handleSubmit = (input: QuestInput, id?: number) => {
    setFormError(null);
    const onError = (e: unknown) => setFormError((e as Error).message);
    const onSuccess = () => setModalOpen(false);
    if (id) update.mutate({ id, patch: input }, { onSuccess, onError });
    else create.mutate(input, { onSuccess, onError });
  };

  const handleComplete = (id: number) =>
    complete.mutate(id, {
      onSuccess: (res) =>
        celebrate({
          title: res.completed_quest.title,
          xp_events: res.xp_events,
          level_ups: res.level_ups,
          label: t("reward.questComplete"),
          gold: res.gold,
          crit: res.crit,
          combo: res.combo_multiplier,
          drop: res.drop,
          achievements: res.achievements_unlocked,
          level: res.dashboard.character.level,
        }),
    });

  const handleWin = (input: QuestInput) => {
    setFormError(null);
    recordWin.mutate(input, {
      onError: (e) => setFormError((e as Error).message),
      onSuccess: (res) => {
        setWinModalOpen(false);
        celebrate({
          title: res.completed_quest.title,
          xp_events: res.xp_events,
          level_ups: res.level_ups,
          label: t("reward.spontaneousWin"),
          gold: res.gold,
          crit: res.crit,
          combo: res.combo_multiplier,
          drop: res.drop,
          achievements: res.achievements_unlocked,
          level: res.dashboard.character.level,
        });
      },
    });
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="font-display text-xl font-bold tracking-tight text-ink">{t("quests.title")}</h1>
          <p className="text-sm text-faint">{t("quests.subtitle")}</p>
        </div>
        <div className="flex items-center gap-2 sm:shrink-0">
          <Btn variant="ghost" className="flex-1 sm:flex-none" onClick={openWin} data-testid="spontaneous-win">
            <Sparkles size={16} /> {t("quests.logWin")}
          </Btn>
          <Btn variant="primary" className="flex-1 sm:flex-none" onClick={openCreate} data-testid="new-quest">
            <Plus size={16} /> {t("quests.new")}
          </Btn>
        </div>
      </div>

      {/* Filters */}
      <div className="hud-panel space-y-3 p-3.5">
        <FilterRow label={t("quests.filterType")}>
          {TYPE_FILTERS.map((ty) => (
            <Chip
              key={ty}
              active={typeFilter === ty}
              color={ty === "all" ? "var(--color-gold)" : getType(ty as QuestType).color}
              onClick={() => setTypeFilter(ty)}
            >
              {ty === "all" ? t("common.all") : getType(ty as QuestType).label}
            </Chip>
          ))}
        </FilterRow>
        <FilterRow label={t("quests.filterStatus")}>
          {STATUS_FILTERS.map((s) => (
            <Chip key={s} active={statusFilter === s} color="var(--color-focus)" onClick={() => setStatusFilter(s)}>
              {t(s === "all" ? "common.all" : (`status.${s}` as MessageKey))}
            </Chip>
          ))}
        </FilterRow>
      </div>

      {isLoading ? (
        <Spinner />
      ) : !quests || quests.length === 0 ? (
        <EmptyState
          icon={<Scroll size={22} />}
          title={t("quests.noMatch")}
          hint={t("quests.noMatchHint")}
        />
      ) : (
        <>
          <SectionTitle hint={tp("quests.count", quests.length)}>{t("quests.results")}</SectionTitle>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {quests.map((q, i) => (
              <QuestCard
                key={q.id}
                quest={q}
                index={i}
                busy={busy}
                onComplete={q.status === "active" ? handleComplete : undefined}
                onEdit={openEdit}
                onSkip={q.status === "active" ? (id) => skip.mutate(id) : undefined}
                onArchive={q.status !== "archived" ? (id) => archive.mutate(id) : undefined}
                onRestore={(id) =>
                  update.mutate(
                    { id, patch: { status: "active" } },
                    { onSuccess: (r) => pushToast(t("quests.restored", { title: r.title }), "success") },
                  )
                }
              />
            ))}
          </div>
        </>
      )}

      <QuestFormModal
        open={modalOpen}
        initial={editing}
        busy={create.isPending || update.isPending}
        error={formError}
        onClose={() => setModalOpen(false)}
        onSubmit={handleSubmit}
      />
      <QuestFormModal
        open={winModalOpen}
        mode="spontaneous"
        busy={recordWin.isPending}
        error={formError}
        onClose={() => setWinModalOpen(false)}
        onSubmit={handleWin}
      />
    </div>
  );
}

function FilterRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="w-14 shrink-0 font-display text-[10px] uppercase tracking-wider text-faint">{label}</span>
      {children}
    </div>
  );
}

function Chip({
  active,
  color,
  onClick,
  children,
}: {
  active: boolean;
  color: string;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className="rounded-full border px-3 py-1 text-xs font-medium transition-all"
      style={{
        borderColor: active ? color : "var(--color-edge)",
        background: active ? `${color}1f` : "transparent",
        color: active ? color : "var(--color-muted)",
      }}
    >
      {children}
    </button>
  );
}
