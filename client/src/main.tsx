import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { MutationCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RewardProvider } from "./lib/reward";
import { AiConsentProvider } from "./lib/aiConsent";
import { I18nProvider, t } from "./lib/i18n";
import { ThemeProvider } from "./lib/themes";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { TokenGate } from "./components/TokenGate";
import { Toaster, pushToast } from "./lib/toast";
import App from "./App";
import "./index.css";

const queryClient = new QueryClient({
  // Global safety net: surface ANY failed mutation so an action can never fail
  // silently (the backend returns a human-readable {error} that api.ts unwraps).
  mutationCache: new MutationCache({
    onError: (err) => pushToast((err as Error)?.message || t("app.somethingWentWrong"), "error"),
  }),
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      staleTime: 5_000,
    },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ErrorBoundary>
        {/* key={locale:theme}: a language or theme switch remounts the whole
            tree so every label and getter-based color (including module-level
            lookups) repaints in one go. */}
        <ThemeProvider>
          {(theme) => (
            <I18nProvider>
              {(locale) => (
                <RewardProvider key={`${locale}:${theme}`}>
                  <AiConsentProvider>
                    <TokenGate>
                      <App />
                    </TokenGate>
                  </AiConsentProvider>
                  <Toaster />
                </RewardProvider>
              )}
            </I18nProvider>
          )}
        </ThemeProvider>
      </ErrorBoundary>
    </QueryClientProvider>
  </StrictMode>,
);
