import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { ShieldCheck } from "lucide-react";

// One-time opt-in before any AI assist touches private content (e.g. mood-log
// text). Consent is remembered in localStorage; requestConsent() resolves true
// once granted, and shows a modal the first time.

const KEY = "edi_ai_assist_consent";

interface AiConsentValue {
  requestConsent: () => Promise<boolean>;
}
const Ctx = createContext<AiConsentValue>({ requestConsent: async () => true });

export function useAiConsent() {
  return useContext(Ctx);
}

export function AiConsentProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const resolver = useRef<(v: boolean) => void>(() => {});

  const requestConsent = useCallback(() => {
    if (localStorage.getItem(KEY) === "1") return Promise.resolve(true);
    setOpen(true);
    return new Promise<boolean>((resolve) => {
      resolver.current = resolve;
    });
  }, []);

  const decide = (ok: boolean) => {
    if (ok) localStorage.setItem(KEY, "1");
    setOpen(false);
    resolver.current(ok);
  };

  return (
    <Ctx.Provider value={{ requestConsent }}>
      {children}
      <AnimatePresence>
        {open && (
          <motion.div
            className="fixed inset-0 z-[55] flex items-center justify-center p-6"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            style={{ background: "rgba(4,5,9,0.7)", backdropFilter: "blur(4px)" }}
            onClick={() => decide(false)}
          >
            <motion.div
              className="hud-panel w-full max-w-sm p-6"
              initial={{ scale: 0.92, y: 16, opacity: 0 }}
              animate={{ scale: 1, y: 0, opacity: 1 }}
              exit={{ scale: 0.95, opacity: 0 }}
              onClick={(e) => e.stopPropagation()}
              data-testid="ai-consent"
            >
              <div
                className="mb-3 grid h-10 w-10 place-items-center rounded-full"
                style={{ background: "rgba(69,224,208,0.14)", color: "var(--color-spirituality)" }}
              >
                <ShieldCheck size={20} />
              </div>
              <h2 className="font-display text-base font-bold text-ink">Use AI help?</h2>
              <p className="mt-2 text-sm text-muted">
                AI help sends this thought to <strong className="text-ink">your own</strong> ChatGPT account to
                suggest distortions and responses. It's a supportive coach, <strong className="text-ink">not
                therapy</strong>. Your entries otherwise stay on your device.
              </p>
              <div className="mt-5 flex justify-end gap-2">
                <button
                  onClick={() => decide(false)}
                  className="rounded-lg px-3 py-2 text-sm text-muted hover:text-ink"
                >
                  Not now
                </button>
                <button
                  onClick={() => decide(true)}
                  data-testid="ai-consent-accept"
                  className="rounded-lg px-3.5 py-2 text-sm font-medium text-[#0c1a17]"
                  style={{ background: "var(--color-spirituality)" }}
                >
                  Allow AI help
                </button>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </Ctx.Provider>
  );
}
