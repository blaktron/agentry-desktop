// A persistent agent's chat blocks, in a terminal's manner, as an agent's
// page and Home's agent card both draw them: the person's lines after a
// ">", the agent's as markdown, a turn's tool calls as one folded dim
// line, the runner's notes dim, and what the agent did on its own folded
// to a line per run, or to one line for runs in a row. A folded line
// opens on the whole of it.
import { AgentMarkdown } from "../AgentMarkdown";
import { backgroundSummary, runSummary, toolsSummary, type BackgroundRun, type TermBlock } from "../persistentAgents";

export function AgentTermBlocks({ blocks }: { blocks: TermBlock[] }) {
  return <>{blocks.map((b, i) => <Block key={i} b={b} />)}</>;
}

function Block({ b }: { b: TermBlock }) {
  switch (b.kind) {
    case "person":
      return <div className="agent-term-person"><span aria-hidden="true">&gt; </span>{b.text}</div>;
    case "agent":
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
      if (b.runs.length === 1) return <Run r={b.runs[0]} />;
      return (
        <details className="agent-term-task" data-background-runs={b.runs.length}>
          <summary>{backgroundSummary(b.runs)}</summary>
          <div className="agent-term-task-body">{b.runs.map((r, i) => <Run key={i} r={r} />)}</div>
        </details>
      );
  }
}

function Run({ r }: { r: BackgroundRun }) {
  return (
    <details className="agent-term-task" data-task-turn={r.kind === "task" ? "" : undefined} data-background-run={r.kind}>
      <summary>{runSummary(r)}</summary>
      <div className="agent-term-task-body">
        {r.instruction && <div className="agent-term-run-instruction">{r.instruction}</div>}
        {r.blocks.map((b, i) => <Block key={i} b={b} />)}
      </div>
    </details>
  );
}
