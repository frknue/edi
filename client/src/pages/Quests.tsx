import { useState } from "react";
import { Copy, Link, Plus, Scroll, Sparkles, UserPlus, Users } from "lucide-react";
import {
  useQuests,
  useCreateQuest,
  useRecordSpontaneousQuest,
  useUpdateQuest,
  useCompleteQuest,
  useSkipQuest,
  useArchiveQuest,
  useCreateQuestBoard,
  useCreateQuestBoardInvite,
  useJoinQuestBoard,
  useMultiplayerStatus,
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
  const { data: multiplayer } = useMultiplayerStatus();

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

      <MultiplayerPanel />

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
                onComplete={q.status === "active" && q.assigned_to_me && q.my_status === "active" ? handleComplete : undefined}
                onEdit={openEdit}
                onSkip={q.status === "active" && q.assigned_to_me && q.my_status === "active" ? (id) => skip.mutate(id) : undefined}
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
        board={multiplayer?.board}
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

function MultiplayerPanel() {
  const { t } = useI18n();
  const { data, isLoading } = useMultiplayerStatus();
  const create = useCreateQuestBoard();
  const invite = useCreateQuestBoardInvite();
  const join = useJoinQuestBoard();
  const [joinCode, setJoinCode] = useState("");
  const [copied, setCopied] = useState(false);

  if (isLoading) return null;
  const board = data?.board;
  const inputCls = "min-w-0 flex-1 rounded-lg border border-edge bg-white/[0.03] px-3 py-2 text-xs text-ink placeholder:text-faint focus:border-[var(--color-relationships)] focus:outline-none";

  const copyInvite = async () => {
    if (!invite.data) return;
    try {
      await navigator.clipboard.writeText(invite.data.code);
      setCopied(true);
    } catch {
      // The selectable code remains visible when clipboard access is unavailable.
    }
  };

  return (
    <div className="hud-panel p-3.5" data-testid="multiplayer-panel">
      <div className="flex items-start gap-3">
        <div className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-[color-mix(in_srgb,var(--color-relationships)_12%,transparent)] text-[var(--color-relationships)]">
          <Users size={16} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <h2 className="font-display text-xs font-semibold uppercase tracking-[0.16em] text-ink">
                {board?.name ?? t("multi.title")}
              </h2>
              <p className="mt-0.5 text-[11px] text-faint">
                {board ? t("multi.boardHint") : t("multi.setupHint")}
              </p>
            </div>
            {board && (
              <div className="flex flex-wrap gap-1">
                {board.members.map((member) => (
                  <span key={member.user_id} className="rounded-full border border-edge px-2 py-0.5 text-[10px] text-muted">
                    {member.name}
                  </span>
                ))}
              </div>
            )}
          </div>

          {!board ? (
            <div className="mt-3 flex flex-col gap-2 sm:flex-row">
              <Btn variant="soft" disabled={create.isPending} onClick={() => create.mutate(t("multi.defaultName"))} data-testid="create-board">
                <UserPlus size={14} /> {t("multi.create")}
              </Btn>
              <div className="flex min-w-0 flex-1 gap-2">
                <input
                  value={joinCode}
                  onChange={(event) => setJoinCode(event.target.value)}
                  onKeyDown={(event) => event.key === "Enter" && joinCode.trim() && join.mutate(joinCode)}
                  placeholder={t("multi.codePlaceholder")}
                  data-testid="join-code"
                  className={inputCls}
                />
                <Btn variant="ghost" disabled={join.isPending || joinCode.trim() === ""} onClick={() => join.mutate(joinCode)} data-testid="join-board">
                  <Link size={14} /> {t("multi.join")}
                </Btn>
              </div>
            </div>
          ) : board.members.length < 2 ? (
            <div className="mt-3">
              {invite.data ? (
                <div className="flex flex-wrap items-center gap-2 rounded-lg border border-[color-mix(in_srgb,var(--color-relationships)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-relationships)_6%,transparent)] p-2.5">
                  <span className="text-[10px] text-faint">{t("multi.inviteCode")}</span>
                  <code className="tabnum select-all text-sm font-semibold tracking-wider text-ink" data-testid="board-invite-code">{invite.data.code}</code>
                  <button onClick={copyInvite} className="text-faint hover:text-ink" aria-label={t("multi.copyCode")}>
                    <Copy size={13} />
                  </button>
                  <span className="text-[10px] text-faint">{copied ? t("multi.copied") : t("multi.expires")}</span>
                </div>
              ) : (
                <Btn variant="ghost" disabled={invite.isPending} onClick={() => invite.mutate()} data-testid="invite-member">
                  <UserPlus size={14} /> {t("multi.invite")}
                </Btn>
              )}
            </div>
          ) : null}
        </div>
      </div>
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
