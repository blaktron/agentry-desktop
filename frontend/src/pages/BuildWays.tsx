// The Build tab's start page: two ways in (agentry-desktop#598, the
// operator's request of 2026-10-04; DEV-201), the same two cards as the
// site's /build (agentry-web#1050) with the same words. Describe it to AI
// opens Describe it; Build it by hand opens Step by step, and offers
// Advanced beside it. Each card is pressable over its whole area: the
// title's button covers the card (.build-way-main::after in style.css),
// and the card's own Advanced button sits above that cover.
//
// The figures are the site's (agentry-web components/build-ways.tsx) in the
// desktop's tokens: light lines with a spark running each, the site's
// convergence language. They picture the card's own sentence and claim
// nothing beyond it, so they are hidden from the accessibility tree. CSS
// keyframes only, and still under prefers-reduced-motion.
import type { CSSProperties, ReactNode } from "react";
import { CardFrame } from "../cards";

// The operator's words, on both clients.
export const BUILD_TAGLINE = "Use our AI Powered QuickAgent Drafter or Build one to your exact requirements.";
export const DESCRIBE_COPY =
  "Build a QuickAgent using the AI Drafter - just tell it what you want an agent to do and it will figure out what tools and servers it needs to do that job and build it out in the correct format.";
export const BY_HAND_COPY =
  "Build a QuickAgent manually so that it does exactly what you want with exactly the tools you want. This is useful if you already have a workflow to bring over, or if you know precisely how you want the agent to behave.";

export function BuildWays({ onDescribe, onSteps, onAdvanced }: { onDescribe: () => void; onSteps: () => void; onAdvanced: () => void }) {
  return (
    <div className="build-ways">
      <WayCard title="Describe it to AI" blurb={DESCRIBE_COPY} figure={<DrafterFigure />} onOpen={onDescribe} />
      <WayCard title="Build it by hand" blurb={BY_HAND_COPY} figure={<ByHandFigure />} onOpen={onSteps}>
        Opens step by step, one section at a time.{" "}
        <button type="button" className="link build-way-part" onClick={onAdvanced}>Switch to Advanced</button>{" "}
        for every field on one page.
      </WayCard>
    </div>
  );
}

// WayCard is one way in, shared with the Voice and Knowledge pages' cards
// (Voice.tsx, agentry-desktop#610; Knowledge.tsx): the card frame Home's
// cards use too (cards.tsx), with a figure and a sentence.
export function WayCard({ title, blurb, figure, onOpen, children }: { title: string; blurb: string; figure: ReactNode; onOpen: () => void; children?: ReactNode }) {
  return <CardFrame title={title} blurb={blurb} figure={figure} onOpen={onOpen} foot={children} />;
}

const W = 320;
const H = 150;
// The figures' lights and parts, shared with the Voice page's figures
// (VoiceFigures.tsx).
export const ACCENT = "var(--color-accent)";
export const SKY = "#38bdf8";
export const VIOLET = "#a78bfa";
export const AMBER = "#fbbf24";

export function Figure({ glow, children }: { glow: string; children: ReactNode }) {
  return (
    <svg viewBox={`0 0 ${W} ${H}`} aria-hidden="true" focusable="false" className="convergence">
      <defs>
        <filter id={glow} filterUnits="userSpaceOnUse" x="0" y="0" width={W} height={H}>
          <feGaussianBlur stdDeviation="4" />
        </filter>
      </defs>
      {children}
    </svg>
  );
}

// One light line: its glow, the stroke, and a spark fitted to its length.
export function Beam({ d, color, glow, length, delay = 0 }: { d: string; color: string; glow: string; length: number; delay?: number }) {
  const timing = { animationDelay: `${delay}s` } as CSSProperties;
  return (
    <>
      <path d={d} className="cv-beam cv-beam-glow" style={{ ...timing, stroke: color }} filter={`url(#${glow})`} />
      <path d={d} className="cv-beam" style={{ ...timing, stroke: color }} />
      <path d={d} className="cv-spark" style={{ ...timing, stroke: color, "--cv-len": length } as CSSProperties} />
    </>
  );
}

export function Part({ x, y, w, h, color, label, glow, delay }: { x: number; y: number; w: number; h: number; color: string; label: string; glow: string; delay: number }) {
  return (
    <g style={{ "--cv-c": color } as CSSProperties}>
      <rect x={x} y={y} width={w} height={h} rx="7" className="cv-box-glow" style={{ animationDelay: `${delay}s` }} filter={`url(#${glow})`} />
      <rect x={x} y={y} width={w} height={h} rx="7" className="cv-box" />
      <text x={x + w / 2} y={y + h / 2 + 3.5} textAnchor="middle" className="cv-label">{label}</text>
    </g>
  );
}

// What the person says, as a speech bubble, runs into the Drafter, a point
// of light, which sends it out as the tools, the servers and the package
// format the draft is written in.
const FOCUS = { x: 172, y: 75 };
const IN = [
  { y: 58, color: SKY, delay: 0, length: 67 },
  { y: 75, color: VIOLET, delay: 1.2, length: 64 },
  { y: 92, color: ACCENT, delay: 2.4, length: 67 },
];
const OUT = [
  { y: 20, color: VIOLET, label: "tools", delay: 3.2, length: 72 },
  { y: 62, color: AMBER, label: "servers", delay: 3.8, length: 54 },
  { y: 104, color: ACCENT, label: "agent.yaml", delay: 4.4, length: 72 },
];
const BOX = { x: 226, w: 84, h: 26 };

export function DrafterFigure() {
  const glow = "bw-drafter-glow";
  const { x, y } = FOCUS;
  return (
    <Figure glow={glow}>
      <path
        d="M 24 36 H 98 Q 108 36 108 46 V 104 Q 108 114 98 114 H 44 L 30 128 L 32 114 H 24 Q 14 114 14 104 V 46 Q 14 36 24 36 Z"
        className="cv-bubble"
      />
      {[[90, 58], [82, 75], [70, 92]].map(([x2, ly]) => (
        <line key={ly} x1={28} x2={x2} y1={ly} y2={ly} className="cv-ink" />
      ))}
      {IN.map((b) => (
        <Beam key={b.y} d={`M 108 ${b.y} C 140 ${b.y}, 146 ${y}, ${x} ${y}`} color={b.color} glow={glow} length={b.length} delay={b.delay} />
      ))}
      {OUT.map((b) => (
        <Beam key={b.y} d={`M ${x} ${y} C 198 ${y}, 200 ${b.y + BOX.h / 2}, ${BOX.x} ${b.y + BOX.h / 2}`} color={b.color} glow={glow} length={b.length} delay={b.delay} />
      ))}
      <Star x={x} y={y} glow={glow} />
      {OUT.map((b) => (
        <Part key={b.label} x={BOX.x} y={b.y} w={BOX.w} h={BOX.h} color={b.color} label={b.label} glow={glow} delay={b.delay} />
      ))}
    </Figure>
  );
}

// A figure's point of light with four rays: the Drafter here, the Voice
// Writer on the Voice page (the site's WayStar).
export function Star({ x, y, glow }: { x: number; y: number; glow: string }) {
  return (
    <>
      <circle cx={x} cy={y} r="15" className="cv-node-glow" style={{ fill: ACCENT }} filter={`url(#${glow})`} />
      <path
        d={`M ${x} ${y - 13} Q ${x + 2} ${y - 2} ${x + 13} ${y} Q ${x + 2} ${y + 2} ${x} ${y + 13} Q ${x - 2} ${y + 2} ${x - 13} ${y} Q ${x - 2} ${y - 2} ${x} ${y - 13} Z`}
        className="cv-node"
        style={{ fill: ACCENT }}
      />
    </>
  );
}

// The form, one section at a time: a rail of steps, each with its field;
// the first two written, the third being written (its caret blinks), the
// last still empty.
const STEPS: { y: number; label: string; state: "done" | "now" | "next"; lines: number[] }[] = [
  { y: 24, label: "basics", state: "done", lines: [96, 58] },
  { y: 58, label: "instructions", state: "done", lines: [120] },
  { y: 92, label: "tools", state: "now", lines: [44] },
  { y: 126, label: "review", state: "next", lines: [] },
];
const RAIL_X = 22;
const FIELD = { x: 128, w: 180, h: 22 };

export function ByHandFigure() {
  const glow = "bw-hand-glow";
  return (
    <Figure glow={glow}>
      <Beam d={`M ${RAIL_X} ${STEPS[0].y} L ${RAIL_X} ${STEPS[STEPS.length - 1].y}`} color={ACCENT} glow={glow} length={102} />
      {STEPS.map((s, i) => (
        <g key={s.label} className={`cv-step cv-step-${s.state}`}>
          {s.state === "now" && (
            <circle cx={RAIL_X} cy={s.y} r="10" className="cv-node-glow" style={{ fill: AMBER, animationDelay: `${i * 1.1}s` }} filter={`url(#${glow})`} />
          )}
          <circle cx={RAIL_X} cy={s.y} r="5.5" className="cv-step-dot" />
          <text x={RAIL_X + 14} y={s.y + 3.5} className="cv-label">{s.label}</text>
          <rect x={FIELD.x} y={s.y - FIELD.h / 2} width={FIELD.w} height={FIELD.h} rx="5" className="cv-field" />
          {s.lines.map((len, j) => {
            const lx = FIELD.x + 10 + s.lines.slice(0, j).reduce((a, b) => a + b + 8, 0);
            return <line key={j} x1={lx} x2={lx + len} y1={s.y} y2={s.y} className="cv-ink" />;
          })}
          {s.state === "now" && (
            <line x1={FIELD.x + 16 + s.lines[0]} x2={FIELD.x + 16 + s.lines[0]} y1={s.y - 6} y2={s.y + 6} className="cv-caret" />
          )}
        </g>
      ))}
    </Figure>
  );
}
