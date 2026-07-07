import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { MutationCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RewardProvider } from "./lib/reward";
import { AiConsentProvider } from "./lib/aiConsent";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { Toaster, pushToast } from "./lib/toast";
import App from "./App";
import "./index.css";

const queryClient = new QueryClient({
  // Global safety net: surface ANY failed mutation so an action can never fail
  // silently (the backend returns a human-readable {error} that api.ts unwraps).
  mutationCache: new MutationCache({
    onError: (err) => pushToast((err as Error)?.message || "Something went wrong", "error"),
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
        <RewardProvider>
          <AiConsentProvider>
            <App />
          </AiConsentProvider>
          <Toaster />
        </RewardProvider>
      </ErrorBoundary>
    </QueryClientProvider>
  </StrictMode>,
);
