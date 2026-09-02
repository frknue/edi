import { useState } from "react";
import { createPortal } from "react-dom";
import { AnimatePresence, motion } from "framer-motion";
import { Copy, UserPlus, X } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { api, hasToken } from "../lib/api";
import { useCreateAccountInvite } from "../lib/queries";
import { useI18n } from "../lib/i18n";
import { Btn } from "./ui";

export function AccountInviteButton({ compact = false }: { compact?: boolean }) {
  const { t } = useI18n();
  const { data: me } = useQuery({ queryKey: ["me"], queryFn: api.me, enabled: hasToken(), retry: false });
  const create = useCreateAccountInvite();
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  if (!hasToken() || !me?.is_admin) return null;

  const link = create.data
    ? `${window.location.origin}/#invite=${encodeURIComponent(create.data.code)}`
    : "";

  const copyLink = async () => {
    if (!link) return;
    try {
      await navigator.clipboard.writeText(link);
      setCopied(true);
    } catch {
      // The selectable link remains visible when clipboard access is unavailable.
    }
  };

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        title={t("invite.button")}
        aria-label={t("invite.button")}
        data-testid={compact ? "account-invite-mobile" : "account-invite"}
        className={
          compact
            ? "flex h-9 w-9 items-center justify-center rounded-lg border border-edge text-faint transition-colors hover:text-ink"
            : "mt-2 flex w-full items-center justify-center gap-1.5 rounded-lg border border-edge py-1.5 text-[10px] font-medium text-faint transition-colors hover:border-edge2 hover:text-ink"
        }
      >
        <UserPlus size={compact ? 16 : 13} />
        {!compact && t("invite.button")}
      </button>

      {createPortal(
        <AnimatePresence>
          {open && (
          <motion.div
            className="sheet-safe fixed inset-0 z-50 flex items-end justify-center p-0 sm:items-center sm:p-6"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            style={{ background: "rgba(4,5,9,0.76)", backdropFilter: "blur(4px)" }}
            onClick={() => setOpen(false)}
            data-testid="account-invite-modal"
          >
            <motion.div
              role="dialog"
              aria-modal="true"
              aria-labelledby="account-invite-title"
              className="hud-panel w-full max-w-md overflow-hidden"
              initial={{ y: 24, opacity: 0 }}
              animate={{ y: 0, opacity: 1 }}
              exit={{ y: 16, opacity: 0 }}
              onClick={(event) => event.stopPropagation()}
            >
              <div className="flex items-center justify-between border-b border-edge px-5 py-3.5">
                <h2 id="account-invite-title" className="font-display text-sm font-semibold uppercase tracking-[0.18em] text-ink">
                  {t("invite.title")}
                </h2>
                <button type="button" onClick={() => setOpen(false)} className="text-faint hover:text-ink" aria-label={t("common.close")}>
                  <X size={18} />
                </button>
              </div>

              <div className="space-y-4 p-5">
                <p className="text-xs leading-relaxed text-faint">{t("invite.hint")}</p>
                {create.data ? (
                  <>
                    <div className="rounded-lg border border-[var(--color-gold)]/40 bg-[var(--color-gold)]/[0.06] p-3">
                      <div className="mb-1 text-[10px] uppercase tracking-wide text-faint">{t("invite.linkLabel")}</div>
                      <code className="block select-all break-all text-xs leading-relaxed text-ink" data-testid="account-invite-link">
                        {link}
                      </code>
                    </div>
                    <div className="flex items-center justify-between gap-3 text-[11px] text-faint">
                      <span>{t("invite.expires")}</span>
                      {copied && <span className="text-[var(--color-phos)]">{t("invite.copied")}</span>}
                    </div>
                    <div className="flex flex-col gap-2 sm:flex-row">
                      <Btn variant="primary" className="flex-1" onClick={copyLink} data-testid="copy-account-invite">
                        <Copy size={14} /> {t("invite.copy")}
                      </Btn>
                      <Btn
                        variant="ghost"
                        className="flex-1"
                        disabled={create.isPending}
                        onClick={() => {
                          setCopied(false);
                          create.mutate();
                        }}
                        data-testid="generate-another-account-invite"
                      >
                        <UserPlus size={14} /> {t("invite.generateAnother")}
                      </Btn>
                    </div>
                  </>
                ) : (
                  <Btn
                    variant="primary"
                    className="w-full"
                    disabled={create.isPending}
                    onClick={() => create.mutate()}
                    data-testid="generate-account-invite"
                  >
                    <UserPlus size={14} /> {t("invite.generate")}
                  </Btn>
                )}
                <p className="text-[11px] leading-relaxed text-faint">{t("invite.recipientHint")}</p>
              </div>
            </motion.div>
          </motion.div>
          )}
        </AnimatePresence>,
        document.body,
      )}
    </>
  );
}
