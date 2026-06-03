import { Bot, RefreshCw } from "lucide-react";
import {
  useSuggestions,
  useGenerateSuggestions,
  useAcceptSuggestion,
  useDismissSuggestion,
} from "../lib/queries";
import { SuggestionCard } from "../components/SuggestionCard";
import { Btn, EmptyState, SectionTitle, Spinner } from "../components/ui";
import { pushToast } from "../lib/toast";

export function SuggestionsPage() {
  const { data: suggestions, isLoading } = useSuggestions();
  const generate = useGenerateSuggestions();
  const accept = useAcceptSuggestion();
  const dismiss = useDismissSuggestion();

  const pending = suggestions?.filter((s) => s.status === "pending") ?? [];
  const resolved = suggestions?.filter((s) => s.status !== "pending") ?? [];
  const busy = accept.isPending || dismiss.isPending;

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="flex items-center gap-2 font-display text-xl font-bold tracking-tight text-ink">
            <Bot size={20} style={{ color: "#b18bff" }} /> Agent Suggestions
          </h1>
          <p className="text-sm text-faint">
            Rule-based for now — same API a future LLM agent will call.
          </p>
        </div>
        <Btn
          variant="primary"
          disabled={generate.isPending}
          onClick={() => generate.mutate()}
          data-testid="generate-suggestions"
        >
          <RefreshCw size={15} className={generate.isPending ? "animate-spin" : ""} /> Generate
        </Btn>
      </div>

      {isLoading ? (
        <Spinner />
      ) : (
        <>
          <section>
            <SectionTitle hint="Accept to turn into a real quest.">Pending</SectionTitle>
            {pending.length === 0 ? (
              <EmptyState
                icon={<Bot size={22} />}
                title="No pending suggestions"
                hint="Hit Generate to analyze your recent activity."
              />
            ) : (
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                {pending.map((s, i) => (
                  <SuggestionCard
                    key={s.id}
                    suggestion={s}
                    index={i}
                    busy={busy}
                    onAccept={(id) =>
                      accept.mutate(id, {
                        onSuccess: (q) => pushToast(`Added quest: ${q.title}`, "success"),
                      })
                    }
                    onDismiss={(id) => dismiss.mutate(id)}
                  />
                ))}
              </div>
            )}
          </section>

          {resolved.length > 0 && (
            <section>
              <SectionTitle hint="Accepted or dismissed.">History</SectionTitle>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                {resolved.map((s, i) => (
                  <SuggestionCard key={s.id} suggestion={s} index={i} />
                ))}
              </div>
            </section>
          )}
        </>
      )}
    </div>
  );
}
