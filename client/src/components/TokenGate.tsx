import { useEffect, useState } from "react";
import { KeyRound } from "lucide-react";
import { saveToken, setUnauthorizedHandler } from "../lib/api";
import { Btn } from "./ui";
import { Logo } from "./Sidebar";

/**
 * Blocks the app when the server rejects our credentials.
 *
 * On a tokenless server this never renders — nothing 401s. It exists for the
 * remote/self-hosted case, and above all for iOS: an app installed to the home
 * screen gets its own storage partition (the token captured in Safari isn't
 * there) and has no address bar to re-open `/#token=…` with. Without this,
 * the first launch would toast 401s forever with no way in.
 */
export function TokenGate({ children }: { children: React.ReactNode }) {
  const [locked, setLocked] = useState(false);
  const [value, setValue] = useState("");

  useEffect(() => {
    setUnauthorizedHandler(() => setLocked(true));
    return () => setUnauthorizedHandler(null);
  }, []);

  if (!locked) return <>{children}</>;

  const unlock = () => {
    if (value.trim() === "") return;
    saveToken(value);
    // Full reload: every cached query was rejected, so refetch from scratch.
    window.location.reload();
  };

  return (
    <div
      className="flex min-h-screen flex-col items-center justify-center gap-6 px-6"
      style={{ paddingTop: "env(safe-area-inset-top)", paddingBottom: "env(safe-area-inset-bottom)" }}
    >
      <Logo />
      <div className="hud-panel w-full max-w-sm space-y-4 p-5">
        <div className="flex items-center gap-2">
          <KeyRound size={16} style={{ color: "var(--color-gold)" }} />
          <h1 className="font-display text-sm font-semibold uppercase tracking-[0.18em] text-ink">
            Access token
          </h1>
        </div>
        <p className="text-xs leading-relaxed text-faint">
          This server is protected. Paste the token it runs with (<code className="text-muted">EDI_TOKEN</code>) to
          unlock this device — it is stored on the device and sent with every request.
        </p>
        <input
          autoFocus
          type="password"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && unlock()}
          placeholder="paste token"
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
          data-testid="token-input"
          className="w-full rounded-lg border border-edge bg-white/[0.03] px-3 py-2 text-sm text-ink placeholder:text-faint focus:border-[var(--color-gold)] focus:outline-none"
        />
        <Btn variant="primary" className="w-full" disabled={value.trim() === ""} onClick={unlock}>
          Unlock
        </Btn>
      </div>
    </div>
  );
}
