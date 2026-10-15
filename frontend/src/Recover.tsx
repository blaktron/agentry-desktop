// A render fault's way back. React unmounts the whole
// tree when a component throws while rendering and nothing catches it, and
// in the app that left the window's bare background: no sidebar, no dialog,
// nothing to click, only a quit and a reopen. Recover catches it and shows
// what went wrong with a way back instead. App puts one around the page
// (the sidebar stays usable, and moving to another page starts it afresh)
// and main.tsx one around App itself, whose way back is a reload.
import { Component, type ErrorInfo, type ReactNode } from "react";
import { RevealText } from "./reveal";
import { Button } from "./ui";

type Props = {
  children: ReactNode;
  /** What stopped, for the heading ("This page", "Agentry Desktop"). */
  subject: string;
  /** A change of key (the page moved) starts the children afresh. */
  resetKey?: string;
  /** The way back; unset, Try again renders the children afresh. */
  onRecover?: () => void;
  recoverLabel?: string;
  /** What the person can do next, after the sentence that says what happened. */
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
        <h2>{this.props.subject} stopped with an error</h2>
        <p className="muted">
          The error happened while showing it. {this.props.hint ?? "Try again, or choose another page on the left."}
        </p>
        {/* The message can quote what was being shown, a model's page
            among it: text only, with hidden characters revealed. */}
        <div className="error-box selectable">
          <RevealText text={error.message || String(error)} />
        </div>
        <div className="row">
          <Button variant="primary" onClick={recover}>{this.props.recoverLabel ?? "Try again"}</Button>
        </div>
      </div>
    );
  }
}
