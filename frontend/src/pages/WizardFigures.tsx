// The first-run wizard's Local and Cloud figures (agentry-desktop#842), in
// the light-line language of the Build, Voice and Knowledge cards
// (BuildWays.tsx): held lines with a spark running each, CSS keyframes only,
// still under prefers-reduced-motion. They picture the card's title and
// claim nothing beyond it, so they are hidden from the accessibility tree.
import type { CSSProperties } from "react";
import { ACCENT, AMBER, Beam, Figure, SKY, VIOLET } from "./BuildWays";

// A desk, a widescreen monitor on it and a desktop computer beside it,
// joined by a cable: this computer.
const DESK_Y = 118;
const SCREEN = { x: 52, y: 18, w: 168, h: 84 };
const TOWER = { x: 238, y: 48, w: 36, h: DESK_Y - 48 };
const STAND_X = SCREEN.x + SCREEN.w / 2;

export function LocalFigure() {
  const glow = "wz-local-glow";
  const cable = `M ${TOWER.x} 104 C ${TOWER.x - 22} 104, ${STAND_X + 46} ${DESK_Y - 3}, ${STAND_X + 24} ${DESK_Y - 3}`;
  return (
    <Figure glow={glow}>
      {/* The desk: its top and two legs. */}
      <Beam d={`M 22 ${DESK_Y} H 298`} color={AMBER} glow={glow} length={276} delay={0} />
      <Beam d={`M 36 ${DESK_Y} V 146`} color={AMBER} glow={glow} length={28} delay={1.4} />
      <Beam d={`M 284 ${DESK_Y} V 146`} color={AMBER} glow={glow} length={28} delay={2.1} />

      {/* The monitor: a wide screen on a stand. */}
      <g style={{ "--cv-c": ACCENT } as CSSProperties}>
        <rect x={SCREEN.x} y={SCREEN.y} width={SCREEN.w} height={SCREEN.h} rx="7" className="cv-box-glow" filter={`url(#${glow})`} />
        <rect x={SCREEN.x} y={SCREEN.y} width={SCREEN.w} height={SCREEN.h} rx="7" className="cv-box" />
        <rect x={SCREEN.x + 8} y={SCREEN.y + 8} width={SCREEN.w - 16} height={SCREEN.h - 16} rx="3" className="cv-field" />
        <path d={`M ${STAND_X} ${SCREEN.y + SCREEN.h} V ${DESK_Y - 5}`} className="cv-stand" />
        <rect x={STAND_X - 26} y={DESK_Y - 6} width={52} height={5} rx="2.5" className="cv-box" />
      </g>
      {/* What is on the screen: a few lines, the caret on the last. */}
      {[[86, 40], [60, 54], [104, 68]].map(([len, ly]) => (
        <line key={ly} x1={SCREEN.x + 20} x2={SCREEN.x + 20 + len} y1={ly} y2={ly} className="cv-ink" />
      ))}
      <line x1={SCREEN.x + 132} x2={SCREEN.x + 132} y1={62} y2={74} className="cv-caret" />

      {/* The computer: a tower, its power light and two drive bays. */}
      <g style={{ "--cv-c": VIOLET } as CSSProperties}>
        <rect x={TOWER.x} y={TOWER.y} width={TOWER.w} height={TOWER.h} rx="5" className="cv-box-glow" style={{ animationDelay: "1s" }} filter={`url(#${glow})`} />
        <rect x={TOWER.x} y={TOWER.y} width={TOWER.w} height={TOWER.h} rx="5" className="cv-box" />
      </g>
      {[62, 72].map((ly) => (
        <line key={ly} x1={TOWER.x + 8} x2={TOWER.x + TOWER.w - 8} y1={ly} y2={ly} className="cv-ink" />
      ))}
      <circle cx={TOWER.x + TOWER.w / 2} cy={92} r="6" className="cv-node-glow" style={{ fill: SKY }} filter={`url(#${glow})`} />
      <circle cx={TOWER.x + TOWER.w / 2} cy={92} r="2.6" className="cv-node" style={{ fill: SKY }} />

      {/* The cable from the computer to the screen's stand. */}
      <Beam d={cable} color={SKY} glow={glow} length={52} delay={3} />
    </Figure>
  );
}

// A datacenter rack in a cloud: Agentry's cloud. The cloud is one held
// line with a spark running round it; the rack's units each have a light.
const CLOUD =
  "M 74 130 C 40 130, 30 94, 60 86 C 54 56, 92 42, 114 58 C 124 22, 194 16, 206 52 C 228 36, 268 50, 260 82 C 292 88, 290 130, 252 130 Z";
const RACK = { x: 130, y: 46, w: 60, h: 78 };
const UNITS = [54, 70, 86, 102];
const LIGHTS = [ACCENT, SKY, ACCENT, AMBER];

export function CloudFigure() {
  const glow = "wz-cloud-glow";
  return (
    <Figure glow={glow}>
      <Beam d={CLOUD} color={SKY} glow={glow} length={610} delay={0} />

      {/* The rack: its frame and four units, each with a light. */}
      <g style={{ "--cv-c": ACCENT } as CSSProperties}>
        <rect x={RACK.x} y={RACK.y} width={RACK.w} height={RACK.h} rx="5" className="cv-box-glow" filter={`url(#${glow})`} />
        <rect x={RACK.x} y={RACK.y} width={RACK.w} height={RACK.h} rx="5" className="cv-box" />
      </g>
      {UNITS.map((uy, i) => (
        <g key={uy}>
          <rect x={RACK.x + 6} y={uy} width={RACK.w - 12} height={12} rx="2.5" className="cv-field" />
          <line x1={RACK.x + 12} x2={RACK.x + 30} y1={uy + 6} y2={uy + 6} className="cv-ink" />
          <circle cx={RACK.x + RACK.w - 13} cy={uy + 6} r="5" className="cv-node-glow" style={{ fill: LIGHTS[i], animationDelay: `${i * 0.9}s` }} filter={`url(#${glow})`} />
          <circle cx={RACK.x + RACK.w - 13} cy={uy + 6} r="2.2" className="cv-node" style={{ fill: LIGHTS[i], animationDelay: `${i * 0.9}s` }} />
        </g>
      ))}
      {/* Its feet, standing on the cloud's floor. */}
      <Beam d={`M ${RACK.x + 8} ${RACK.y + RACK.h} V 130`} color={VIOLET} glow={glow} length={6} delay={2} />
      <Beam d={`M ${RACK.x + RACK.w - 8} ${RACK.y + RACK.h} V 130`} color={VIOLET} glow={glow} length={6} delay={2.6} />
    </Figure>
  );
}
