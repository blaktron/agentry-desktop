// A persistent agent's chat blocks, in a terminal's manner, as an agent's
// page and Home's agent card both draw them: the person's lines after a
// ">", the agent's as markdown, a turn's tool calls as one folded dim
// line, the runner's notes dim, and what the agent did on its own folded
// to a line per run, or to one line for runs in a row. A folded line
// opens on the whole of it. With bubbles (the page's Chat, Focus and
// Workspace views) the person's and the agent's lines are the app's chat
// bubbles instead, under "You" and the agent's name; the rest is the same.
import type { ReactNode } from "react";
import { AgentMarkdown } from "../AgentMarkdown";
import { backgroundSummary, runSummary, toolsSummary, type BackgroundRun, type TermBlock } from "../persistentAgents";

/** How the person's and the agent's lines are drawn: in a terminal's
 *  manner when absent, as chat bubbles under these names when given. */
export type Bubbles = { agent: string } | undefined;

export function AgentTermBlocks({ blocks, bubbles }: { blocks: TermBlock[]; bubbles?: Bubbles }) {
  return <>{blocks.map((b, i) => <Block key={i} b={b} bubbles={bubbles} />)}</>;
}

/** The agent's line as a bubble, for its answer and for what it streams. */
export function AgentBubble({ who, streaming, children }: { who: string; streaming?: boolean; children: ReactNode }) {
  return (
    <div className={`chat-msg assistant agent-chat-msg${streaming ? " streaming" : ""}`}>
      <div className="chat-bubble"><div className="chat-who">{who}</div>{children}</div>
    </div>
  );
}

function Block({ b, bubbles }: { b: TermBlock; bubbles: Bubbles }) {
  switch (b.kind) {
    case "person":
      if (bubbles) return <div className="chat-msg user agent-chat-msg"><div className="chat-bubble"><div className="chat-who">You</div>{b.text}</div></div>;
      return <div className="agent-term-person"><span aria-hidden="true">&gt; </span>{b.text}</div>;
    case "agent":
      if (bubbles) return <AgentBubble who={bubbles.agent}><AgentMarkdown text={b.text} /></AgentBubble>;
      return <div className="agent-term-agent"><AgentMarkdown text={b.text} /></div>;
    case "tools":
      return (
        <details className="agent-term-tools" data-tool-calls>
          <summary>{toolsSummary(b.names)}</summary>
          <ol>{b.names.map((n, i) => <li key={i}>{n}</li>)}</ol>
        </details>
      );
    case "warn":
      return <div className="agent-term-warn">{b.text}</div>;
    case "note":
      return <div className="agent-term-note">{b.text}</div>;
    case "background":
      if (b.runs.length === 1) return <Run r={b.runs[0]} bubbles={bubbles} />;
      return (
        <details className="agent-term-task" data-background-runs={b.runs.length}>
          <summary>{backgroundSummary(b.runs)}</summary>
          <div className="agent-term-task-body">{b.runs.map((r, i) => <Run key={i} r={r} bubbles={bubbles} />)}</div>
        </details>
      );
  }
}

function Run({ r, bubbles }: { r: BackgroundRun; bubbles: Bubbles }) {
  return (
    <details className="agent-term-task" data-task-turn={r.kind === "task" ? "" : undefined} data-background-run={r.kind}>
      <summary>{runSummary(r)}</summary>
      <div className="agent-term-task-body">
        {r.instruction && <div className="agent-term-run-instruction">{r.instruction}</div>}
        {r.blocks.map((b, i) => <Block key={i} b={b} bubbles={bubbles} />)}
      </div>
    </details>
  );
}
