import { useEffect, useState } from "react";
import { Copy, KeyRound, UserPlus } from "lucide-react";
import { api, saveToken, setUnauthorizedHandler } from "../lib/api";
import { Btn } from "./ui";
import { Logo } from "./Sidebar";

/**
 * Blocks the app when the server rejects our credentials.
 *
 * On a tokenless server this never renders — nothing 401s. It exists for the
 * remote/self-hosted case, and above all for iOS: an app installed to the home
 * screen gets its own storage partition (the token captured in Safari isn't
 * there) and has no address bar to re-open `/#token=…` with.
 *
 * Two ways in: paste an existing access token, or — when the server has
 * EDI_INVITE_CODE set — create a fresh account with the invite code. The new
 * token is shown exactly once (the server stores only its hash), so the gate
 * makes the user acknowledge saving it before entering.
 */
export function TokenGate({ children }: { children: React.ReactNode }) {
  const [locked, setLocked] = useState(false);
  const [mode, setMode] = useState<"token" | "register">("token");
  const [registrationOpen, setRegistrationOpen] = useState(false);

  const [value, setValue] = useState("");
  const [name, setName] = useState("");
  const [invite, setInvite] = useState("");
  const [minted, setMinted] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    setUnauthorizedHandler(() => setLocked(true));
    return () => setUnauthorizedHandler(null);
  }, []);

  useEffect(() => {
    if (!locked) return;
    api
      .authConfig()
      .then((cfg) => setRegistrationOpen(cfg.registration_open))
      .catch(() => setRegistrationOpen(false));
  }, [locked]);

  if (!locked) return <>{children}</>;

  const unlock = () => {
    if (value.trim() === "") return;
    saveToken(value);
    // Full reload: every cached query was rejected, so refetch from scratch.
    window.location.reload();
  };

  const register = async () => {
    setBusy(true);
    setError(null);
    try {
      const created = await api.register({ name: name.trim(), invite_code: invite.trim() });
      setMinted(created.token);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const copyMinted = async () => {
    if (!minted) return;
    try {
      await navigator.clipboard.writeText(minted);
      setCopied(true);
    } catch {
      /* clipboard unavailable — the token is selectable below */
    }
  };

  const enterWithMinted = () => {
    if (!minted) return;
    saveToken(minted);
    window.location.reload();
  };

  const inputCls =
    "w-full rounded-lg border border-edge bg-white/[0.03] px-3 py-2 text-sm text-ink placeholder:text-faint focus:border-[var(--color-gold)] focus:outline-none";

  return (
    <div
      className="flex min-h-screen flex-col items-center justify-center gap-6 px-6"
      style={{ paddingTop: "env(safe-area-inset-top)", paddingBottom: "env(safe-area-inset-bottom)" }}
    >
      <Logo />
      <div className="hud-panel w-full max-w-sm space-y-4 p-5">
        {minted ? (
          <>
            <h1 className="font-display text-sm font-semibold uppercase tracking-[0.18em] text-ink">
              Account created
            </h1>
            <p className="text-xs leading-relaxed text-faint">
              This is your access token — it is shown <span className="text-ink">only once</span>. Save it somewhere
              safe (password manager); you need it to sign in on other devices.
            </p>
            <div className="flex items-center gap-2 rounded-lg border border-[var(--color-gold)]/40 bg-[var(--color-gold)]/[0.06] p-2.5">
              <code className="tabnum min-w-0 flex-1 break-all text-xs text-ink" data-testid="minted-token">
                {minted}
              </code>
              <button onClick={copyMinted} className="shrink-0 text-faint hover:text-ink" aria-label="Copy token">
                <Copy size={14} />
              </button>
            </div>
            {copied && <p className="text-[11px] text-[var(--color-phos)]">Copied.</p>}
            <Btn variant="primary" className="w-full" onClick={enterWithMinted} data-testid="enter-minted">
              I saved it — enter
            </Btn>
          </>
        ) : mode === "register" ? (
          <>
            <div className="flex items-center gap-2">
              <UserPlus size={16} style={{ color: "var(--color-gold)" }} />
              <h1 className="font-display text-sm font-semibold uppercase tracking-[0.18em] text-ink">
                New character
              </h1>
            </div>
            <p className="text-xs leading-relaxed text-faint">
              Create your own character on this server. You need the invite code from whoever runs it.
            </p>
            <input
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="your name"
              maxLength={40}
              data-testid="register-name"
              className={inputCls}
            />
            <input
              type="password"
              value={invite}
              onChange={(e) => setInvite(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && register()}
              placeholder="invite code"
              autoCapitalize="off"
              autoCorrect="off"
              data-testid="register-invite"
              className={inputCls}
            />
            {error && <p className="text-xs text-[#ff8a80]">{error}</p>}
            <Btn
              variant="primary"
              className="w-full"
              disabled={busy || name.trim() === "" || invite.trim() === ""}
              onClick={register}
              data-testid="register-submit"
            >
              Create character
            </Btn>
            <button onClick={() => setMode("token")} className="w-full text-center text-[11px] text-faint hover:text-ink">
              I already have a token
            </button>
          </>
        ) : (
          <>
            <div className="flex items-center gap-2">
              <KeyRound size={16} style={{ color: "var(--color-gold)" }} />
              <h1 className="font-display text-sm font-semibold uppercase tracking-[0.18em] text-ink">
                Access token
              </h1>
            </div>
            <p className="text-xs leading-relaxed text-faint">
              This server is protected. Paste your access token to unlock this device — it is stored on the device and
              sent with every request.
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
              className={inputCls}
            />
            <Btn variant="primary" className="w-full" disabled={value.trim() === ""} onClick={unlock}>
              Unlock
            </Btn>
            {registrationOpen && (
              <button
                onClick={() => setMode("register")}
                data-testid="goto-register"
                className="w-full text-center text-[11px] text-faint hover:text-ink"
              >
                No account yet? Create a character
              </button>
            )}
          </>
        )}
      </div>
    </div>
  );
}
