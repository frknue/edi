import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { CheckCircle2, TriangleAlert, X } from "lucide-react";

export type ToastType = "error" | "success" | "info";

interface Toast {
  id: number;
  message: string;
  type: ToastType;
}

// Module-level emitter so non-React code (e.g. the React Query MutationCache) can
// push toasts without a hook. A single Toaster mounted at the root renders them.
type Listener = (t: Toast) => void;
let listeners: Listener[] = [];
let counter = 0;

export function pushToast(message: string, type: ToastType = "info") {
  counter += 1;
  const toast: Toast = { id: counter, message, type };
  listeners.forEach((l) => l(toast));
}

function subscribe(l: Listener): () => void {
  listeners.push(l);
  return () => {
    listeners = listeners.filter((x) => x !== l);
  };
}

const styles: Record<ToastType, { color: string; Icon: typeof CheckCircle2 }> = {
  error: { color: "#ff7d9d", Icon: TriangleAlert },
  success: { color: "#3ee594", Icon: CheckCircle2 },
  info: { color: "#2dd4ff", Icon: CheckCircle2 },
};

export function Toaster() {
  const [toasts, setToasts] = useState<Toast[]>([]);

  useEffect(
    () =>
      subscribe((t) => {
        setToasts((cur) => [...cur, t]);
        window.setTimeout(() => {
          setToasts((cur) => cur.filter((x) => x.id !== t.id));
        }, 4500);
      }),
    [],
  );

  const dismiss = (id: number) => setToasts((cur) => cur.filter((x) => x.id !== id));

  return (
    <div className="pointer-events-none fixed bottom-20 right-4 z-[60] flex w-[min(92vw,360px)] flex-col gap-2 lg:bottom-4">
      <AnimatePresence>
        {toasts.map((t) => {
          const s = styles[t.type];
          const Icon = s.Icon;
          return (
            <motion.div
              key={t.id}
              layout
              initial={{ opacity: 0, x: 40, scale: 0.96 }}
              animate={{ opacity: 1, x: 0, scale: 1 }}
              exit={{ opacity: 0, x: 40, scale: 0.96 }}
              transition={{ type: "spring", stiffness: 380, damping: 30 }}
              className="hud-panel pointer-events-auto flex items-start gap-2.5 p-3"
              style={{ borderColor: `${s.color}55` }}
              role="status"
            >
              <Icon size={16} style={{ color: s.color, marginTop: 1 }} className="shrink-0" />
              <p className="flex-1 text-[13px] leading-snug text-ink">{t.message}</p>
              <button onClick={() => dismiss(t.id)} className="text-faint hover:text-ink" aria-label="Dismiss">
                <X size={14} />
              </button>
            </motion.div>
          );
        })}
      </AnimatePresence>
    </div>
  );
}
