import { Component, type ErrorInfo, type ReactNode } from "react";

interface Props {
  children: ReactNode;
}
interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Surface for debugging; in production this would go to a logger.
    console.error("UI error:", error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex min-h-screen items-center justify-center p-6">
          <div className="hud-panel max-w-md p-6" data-testid="error-boundary">
            <h1 className="font-display text-lg font-bold text-[#ff7d9d]">Something broke</h1>
            <p className="mt-2 text-sm text-muted">{this.state.error.message}</p>
            <pre className="mt-3 max-h-40 overflow-auto rounded-lg bg-black/40 p-3 text-[11px] text-faint">
              {this.state.error.stack}
            </pre>
            <button
              className="mt-4 rounded-lg border border-edge px-3 py-2 text-sm text-ink hover:bg-white/5"
              onClick={() => window.location.reload()}
            >
              Reload
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
