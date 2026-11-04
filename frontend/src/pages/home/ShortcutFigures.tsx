// The shortcut cards' figures: the Build page's light-line language (the
// Drafter's figure in BuildWays.tsx), drawn small enough for one cell of
// Home's grid. Each pictures its card's title and claims nothing beyond it,
// so it is hidden from the accessibility tree. CSS keyframes only, and still
// under prefers-reduced-motion.
import type { ReactNode } from "react";
import type { ShortcutKind } from "../../homeCards";
import { ACCENT, AMBER, Beam, Part, SKY, Star, VIOLET } from "../BuildWays";

const W = 200;
const H = 110;
// Where each figure's point of light sits, and its outputs' boxes.
const FX = 108;
const FY = 55;
const OUT_X = 142;
const OUT_W = 52;
const BOX_H = 22;

function Small({ glow, children }: { glow: string; children: ReactNode }) {
  return (
    <svg viewBox={`0 0 ${W} ${H}`} aria-hidden="true" focusable="false" className="convergence" preserveAspectRatio="xMidYMid meet">
      <defs>
        <filter id={glow} filterUnits="userSpaceOnUse" x="0" y="0" width={W} height={H}>
          <feGaussianBlur stdDeviation="3.5" />
        </filter>
      </defs>
      {children}
    </svg>
  );
}

// Lines from the left side's points into the point of light.
function Into({ ys, from, glow }: { ys: { y: number; color: string; delay: number }[]; from: number; glow: string }) {
  return (
    <>
      {ys.map((b) => (
        <Beam key={b.y} d={`M ${from} ${b.y} C ${from + 22} ${b.y}, ${FX - 22} ${FY}, ${FX} ${FY}`} color={b.color} glow={glow} length={FX - from + 6} delay={b.delay} />
      ))}
    </>
  );
}

// The point of light's outputs: a line to each box, and the boxes.
function Out({ boxes, glow }: { boxes: { y: number; color: string; label: string; delay: number }[]; glow: string }) {
  return (
    <>
      {boxes.map((b) => (
        <Beam key={b.label} d={`M ${FX} ${FY} C ${FX + 16} ${FY}, ${OUT_X - 16} ${b.y + BOX_H / 2}, ${OUT_X} ${b.y + BOX_H / 2}`} color={b.color} glow={glow} length={OUT_X - FX + 6} delay={b.delay} />
      ))}
      <Star x={FX} y={FY} glow={glow} />
      {boxes.map((b) => (
        <Part key={b.label} x={OUT_X} y={b.y} w={OUT_W} h={BOX_H} color={b.color} label={b.label} glow={glow} delay={b.delay} />
      ))}
    </>
  );
}

// A sheet with lines of writing on it.
function Sheet({ x, y, w, h, lines }: { x: number; y: number; w: number; h: number; lines: number[] }) {
  return (
    <>
      <rect x={x} y={y} width={w} height={h} rx="4" className="cv-field" />
      {lines.map((lw, i) => <line key={i} x1={x + 7} x2={x + 7 + lw} y1={y + 9 + i * 8} y2={y + 9 + i * 8} className="cv-ink" />)}
    </>
  );
}

// Run: a QuickAgent goes into the light and comes out as a report and files.
function RunFigure() {
  const glow = "hs-run-glow";
  return (
    <Small glow={glow}>
      <Into ys={[{ y: FY, color: SKY, delay: 0 }]} from={66} glow={glow} />
      <Part x={10} y={FY - 13} w={56} h={26} color={SKY} label="agent" glow={glow} delay={0} />
      <Out boxes={[{ y: 20, color: ACCENT, label: "report", delay: 1.4 }, { y: 68, color: AMBER, label: "files", delay: 2 }]} glow={glow} />
    </Small>
  );
}

// Draft: what the person says, in a bubble, becomes the QuickAgent's
// package.
function DraftFigure() {
  const glow = "hs-draft-glow";
  return (
    <Small glow={glow}>
      <path d="M 16 24 H 62 Q 70 24 70 32 V 74 Q 70 82 62 82 H 30 L 20 92 L 22 82 H 16 Q 8 82 8 74 V 32 Q 8 24 16 24 Z" className="cv-bubble" />
      {[[58, 40], [50, 53], [40, 66]].map(([x2, y]) => <line key={y} x1={18} x2={x2} y1={y} y2={y} className="cv-ink" />)}
      <Into ys={[{ y: 40, color: SKY, delay: 0 }, { y: 66, color: VIOLET, delay: 1 }]} from={70} glow={glow} />
      <Out boxes={[{ y: 44, color: ACCENT, label: "package", delay: 2 }]} glow={glow} />
    </Small>
  );
}

// Knowledge: documents gather into one pack.
function KnowledgeFigure() {
  const glow = "hs-knowledge-glow";
  return (
    <Small glow={glow}>
      <Sheet x={10} y={12} w={52} h={26} lines={[34, 24]} />
      <Sheet x={10} y={42} w={52} h={26} lines={[30, 36]} />
      <Sheet x={10} y={72} w={52} h={26} lines={[38, 20]} />
      <Into ys={[{ y: 25, color: SKY, delay: 0 }, { y: 55, color: VIOLET, delay: 0.8 }, { y: 85, color: AMBER, delay: 1.6 }]} from={62} glow={glow} />
      <Out boxes={[{ y: 44, color: ACCENT, label: "pack", delay: 2.4 }]} glow={glow} />
    </Small>
  );
}

// Meeting notes: a voice's wave becomes notes.
function MeetingsFigure() {
  const glow = "hs-meetings-glow";
  const bars = [10, 22, 34, 18, 40, 26, 14, 30, 20, 8];
  return (
    <Small glow={glow}>
      {bars.map((h, i) => (
        <line key={i} x1={12 + i * 6} x2={12 + i * 6} y1={FY - h / 2} y2={FY + h / 2} className="cv-beam" style={{ stroke: i % 3 === 0 ? VIOLET : SKY, animationDelay: `${i * 0.3}s` }} />
      ))}
      <Into ys={[{ y: FY, color: SKY, delay: 0.6 }]} from={70} glow={glow} />
      <Out boxes={[{ y: 22, color: ACCENT, label: "notes", delay: 1.6 }, { y: 66, color: AMBER, label: "actions", delay: 2.2 }]} glow={glow} />
    </Small>
  );
}

// Writing Voice: what the person wrote becomes the voice runs write in.
function VoiceFigure() {
  const glow = "hs-voice-glow";
  return (
    <Small glow={glow}>
      <Sheet x={10} y={20} w={58} h={70} lines={[40, 32, 44, 28, 36, 22, 30]} />
      <Into ys={[{ y: 38, color: VIOLET, delay: 0 }, { y: 72, color: SKY, delay: 1 }]} from={68} glow={glow} />
      <Out boxes={[{ y: 44, color: ACCENT, label: "voice", delay: 2 }]} glow={glow} />
    </Small>
  );
}

// Persistent agent: a light that stays on, with its chat and its tools.
function AgentFigure() {
  const glow = "hs-agent-glow";
  return (
    <Small glow={glow}>
      <ellipse cx={FX} cy={FY} rx="34" ry="20" className="cv-beam" style={{ stroke: VIOLET }} />
      <Part x={8} y={FY - 11} w={52} h={BOX_H} color={SKY} label="chat" glow={glow} delay={0} />
      <Into ys={[{ y: FY, color: SKY, delay: 0 }]} from={60} glow={glow} />
      <Out boxes={[{ y: 16, color: AMBER, label: "tools", delay: 1.2 }, { y: 72, color: ACCENT, label: "files", delay: 1.8 }]} glow={glow} />
    </Small>
  );
}

// Setup: the model, the runtime and the key meet in one ready light.
function SetupFigure() {
  const glow = "hs-setup-glow";
  return (
    <Small glow={glow}>
      <Part x={8} y={10} w={54} h={BOX_H} color={SKY} label="model" glow={glow} delay={0} />
      <Part x={8} y={44} w={54} h={BOX_H} color={VIOLET} label="runtime" glow={glow} delay={0.8} />
      <Part x={8} y={78} w={54} h={BOX_H} color={AMBER} label="key" glow={glow} delay={1.6} />
      <Into ys={[{ y: 21, color: SKY, delay: 0 }, { y: 55, color: VIOLET, delay: 0.8 }, { y: 89, color: AMBER, delay: 1.6 }]} from={62} glow={glow} />
      <Out boxes={[{ y: 44, color: ACCENT, label: "ready", delay: 2.4 }]} glow={glow} />
    </Small>
  );
}

export const SHORTCUT_FIGURES: Record<ShortcutKind, () => ReactNode> = {
  "shortcut-run": RunFigure,
  "shortcut-draft": DraftFigure,
  "shortcut-knowledge": KnowledgeFigure,
  "shortcut-meetings": MeetingsFigure,
  "shortcut-voice": VoiceFigure,
  "shortcut-agent": AgentFigure,
  "shortcut-setup": SetupFigure,
};
