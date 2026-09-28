// A render fault's way back (agentry-desktop#351). React unmounts the whole
// tree when a component throws while rendering and nothing catches it, and
// in the app that left the window's bare background: no sidebar, no dialog,
// nothing to click, only a quit and a reopen. Recover catches it and shows
// what went wrong with a way back instead. App puts one around the page
// (the sidebar stays usable, and moving to another page starts it afresh)
// and main.tsx one around App itself, whose way back is a reload.
import { Component, type ErrorInfo, type ReactNode } from "react";
import { Button, ErrorBox } from "./ui";

type Props = {
  children: ReactNode;
  /** Where the fault happened, for the heading ("This page", "Agentry Desktop"). */
  what: string;
  /** A change of key (the page moved) starts the children afresh. */
  resetKey?: string;
  /** The way back; unset, Try again renders the children afresh. */
  onRecover?: () => void;
  recoverLabel?: string;
  /** What else the person can do, after "Nothing you kept was changed by this." */
  hint?: string;
};

type State = { error: Error | null };

export class Recover extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: unknown): State {
    return { error: error instanceof Error ? error : new Error(String(error)) };
  }

  componentDidCatch(error: unknown, info: ErrorInfo) {
    // The webview's console is the only log a render fault has.
    console.error("render fault:", error, info.componentStack);
  }

  componentDidUpdate(prev: Props) {
    if (this.state.error && prev.resetKey !== this.props.resetKey) this.setState({ error: null });
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    const recover = () => {
      this.setState({ error: null });
      this.props.onRecover?.();
    };
    return (
      <div className="recover" role="alert">
        <h2>{this.props.what} stopped with an error</h2>
        <p className="muted">
          Something on it could not be shown. Nothing you kept was changed by this. {this.props.hint ?? "Try again, or choose another page on the left."}
        </p>
        <ErrorBox error={error.message || String(error)} />
        <div className="row">
          <Button variant="primary" onClick={recover}>{this.props.recoverLabel ?? "Try again"}</Button>
        </div>
      </div>
    );
  }
}
