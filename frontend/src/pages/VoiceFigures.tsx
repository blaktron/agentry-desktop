// The Voice page's two figures (plan voice-page-redesign.md §4.1,
// agentry-desktop#610), drawn as the Build page's are (DEV-201) and in the
// language of the site's (agentry-web components/voice-ways.tsx): light
// lines with a spark running each. They picture the card's own sentence and
// claim nothing beyond it, so they are hidden from the accessibility tree.
// CSS keyframes only, and still under prefers-reduced-motion.
import { ACCENT, AMBER, Beam, Figure, Part, SKY, VIOLET } from "./BuildWays";

// LocalVoiceFigure: lines of writing on this computer's screen run into one
// VOICE.md, which a small padlock seals here.
const SCREEN = { x: 14, y: 24, w: 116, h: 82 };
const INK: { y: number; len: number }[] = [
  { y: 46, len: 64 },
  { y: 62, len: 84 },
  { y: 78, len: 72 },
  { y: 94, len: 44 },
];
const FILE = { x: 212, y: 62, w: 92, h: 26 };
const LOCAL_BEAMS = [
  { y: 46, color: SKY, delay: 0, length: 90 },
  { y: 65, color: VIOLET, delay: 1.2, length: 82 },
  { y: 84, color: AMBER, delay: 2.4, length: 84 },
];
const LOCK = { x: 248, y: 104 };

export function LocalVoiceFigure() {
  const glow = "vf-local-glow";
  const toY = FILE.y + FILE.h / 2;
  return (
    <Figure glow={glow}>
      <rect x={SCREEN.x} y={SCREEN.y} width={SCREEN.w} height={SCREEN.h} rx="7" className="cv-bubble" />
      <line x1={SCREEN.x + 30} x2={SCREEN.x + SCREEN.w - 30} y1={SCREEN.y + SCREEN.h + 12} y2={SCREEN.y + SCREEN.h + 12} className="cv-ink" />
      {INK.map((l) => (
        <line key={l.y} x1={SCREEN.x + 12} x2={SCREEN.x + 12 + l.len} y1={l.y} y2={l.y} className="cv-ink" />
      ))}
      {LOCAL_BEAMS.map((b) => (
        <Beam
          key={b.y}
          d={`M ${SCREEN.x + SCREEN.w} ${b.y} C 170 ${b.y}, 176 ${toY}, ${FILE.x} ${toY}`}
          color={b.color}
          glow={glow}
          length={b.length}
          delay={b.delay}
        />
      ))}
      <Part x={FILE.x} y={FILE.y} w={FILE.w} h={FILE.h} color={ACCENT} label="VOICE.md" glow={glow} delay={3.4} />
      {/* Sealed on this computer. */}
      <g className="cv-lock">
        <path d={`M ${LOCK.x + 4} ${LOCK.y + 8} V ${LOCK.y + 3} a 6 6 0 0 1 12 0 V ${LOCK.y + 8}`} className="cv-lock-shackle" />
        <rect x={LOCK.x} y={LOCK.y + 8} width="20" height="16" rx="3" className="cv-lock-body" />
        <circle cx={LOCK.x + 10} cy={LOCK.y + 15} r="2" className="cv-lock-hole" />
      </g>
    </Figure>
  );
}

// CloudVoiceFigure: a VOICE.md beams up through AI Review's mark, which
// reads every version, into the account's cloud.
const UP = { x: 14, y: 104, w: 92, h: 26 };
const MARK = { x: 162, y: 80 };
const CLOUD = "M 232 70 Q 214 70 216 55 Q 218 42 232 43 Q 236 26 256 26 Q 275 26 280 40 Q 298 39 300 55 Q 300 70 285 70 Z";

export function CloudVoiceFigure() {
  const glow = "vf-cloud-glow";
  const { x, y } = MARK;
  return (
    <Figure glow={glow}>
      <Beam d={`M ${UP.x + UP.w} ${UP.y + UP.h / 2} C 130 ${UP.y + UP.h / 2}, 136 ${y}, ${x - 15} ${y}`} color={SKY} glow={glow} length={62} delay={0} />
      <Beam d={`M ${x + 15} ${y} C 196 ${y}, 204 62, 222 58`} color={VIOLET} glow={glow} length={50} delay={1.6} />
      <Part x={UP.x} y={UP.y} w={UP.w} h={UP.h} color={SKY} label="VOICE.md" glow={glow} delay={0} />
      {/* AI Review: a ring with a check, which every version passes through. */}
      <circle cx={x} cy={y} r="15" className="cv-node-glow" style={{ fill: ACCENT }} filter={`url(#${glow})`} />
      <circle cx={x} cy={y} r="13" className="cv-mark" />
      <path d={`M ${x - 6} ${y} L ${x - 1.5} ${y + 4.5} L ${x + 6} ${y - 4}`} className="cv-check" />
      <text x={x} y={y + 30} textAnchor="middle" className="cv-label">AI Review</text>
      <path d={CLOUD} className="cv-bubble" />
      {[[240, 56, 30], [244, 48, 40]].map(([x1, ly, len]) => (
        <line key={ly} x1={x1} x2={x1 + len} y1={ly} y2={ly} className="cv-ink" />
      ))}
    </Figure>
  );
}
