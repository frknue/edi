import { useState } from "react";
import { Plus, Scroll } from "lucide-react";
import {
  useQuests,
  useCreateQuest,
  useUpdateQuest,
  useCompleteQuest,
  useSkipQuest,
  useArchiveQuest,
} from "../lib/queries";
import { useReward } from "../lib/reward";
import { QuestCard } from "../components/QuestCard";
import { QuestFormModal } from "../components/QuestFormModal";
import { Btn, EmptyState, SectionTitle, Spinner } from "../components/ui";
import { typeMeta, getType } from "../lib/theme";
import type { Quest, QuestInput, QuestType } from "../lib/types";

const TYPE_FILTERS: ("all" | QuestType)[] = ["all", ...(Object.keys(typeMeta) as QuestType[])];
const STATUS_FILTERS = ["active", "completed", "skipped", "archived", "all"] as const;

export function QuestsPage() {
  const [typeFilter, setTypeFilter] = useState<(typeof TYPE_FILTERS)[number]>("all");
  const [statusFilter, setStatusFilter] = useState<(typeof STATUS_FILTERS)[number]>("active");
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Quest | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const filters = {
    type: typeFilter === "all" ? undefined : typeFilter,
    status: statusFilter === "all" ? undefined : statusFilter,
  };
  const { data: quests, isLoading } = useQuests(filters);

  const create = useCreateQuest();
  const update = useUpdateQuest();
  const complete = useCompleteQuest();
  const skip = useSkipQuest();
  const archive = useArchiveQuest();
  const { celebrate } = useReward();

  const busy = create.isPending || update.isPending || complete.isPending || skip.isPending || archive.isPending;

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
          label: "Quest Complete",
        }),
    });

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-display text-xl font-bold tracking-tight text-ink">Quest Log</h1>
          <p className="text-sm text-faint">Create, edit, and manage your real-life quests.</p>
        </div>
        <Btn variant="primary" onClick={openCreate} data-testid="new-quest">
          <Plus size={16} /> New quest
        </Btn>
      </div>

      {/* Filters */}
      <div className="hud-panel space-y-3 p-3.5">
        <FilterRow label="Type">
          {TYPE_FILTERS.map((t) => (
            <Chip
              key={t}
              active={typeFilter === t}
              color={t === "all" ? "var(--color-gold)" : getType(t as QuestType).color}
              onClick={() => setTypeFilter(t)}
            >
              {t === "all" ? "All" : getType(t as QuestType).label}
            </Chip>
          ))}
        </FilterRow>
        <FilterRow label="Status">
          {STATUS_FILTERS.map((s) => (
            <Chip key={s} active={statusFilter === s} color="var(--color-focus)" onClick={() => setStatusFilter(s)}>
              {s[0].toUpperCase() + s.slice(1)}
            </Chip>
          ))}
        </FilterRow>
      </div>

      {isLoading ? (
        <Spinner />
      ) : !quests || quests.length === 0 ? (
        <EmptyState
          icon={<Scroll size={22} />}
          title="No quests match these filters"
          hint="Try a different filter or create a new quest."
        />
      ) : (
        <>
          <SectionTitle hint={`${quests.length} quest${quests.length === 1 ? "" : "s"}`}>Results</SectionTitle>
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
