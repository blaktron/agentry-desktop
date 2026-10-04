// The Knowledge page's two figures (plan knowledge-cards.md §4.1,
// agentry-desktop#619), drawn as the Build and Voice pages' are (DEV-201,
// VoiceFigures.tsx) and in the language of the site's Knowledge cards
// (agentry-web components/knowledge-ways.tsx): light lines with a spark
// running each. They picture the card's own sentence and claim nothing
// beyond it, so they are hidden from the accessibility tree; AI Review is a
// ring a pack passes through, never a check or a shield, which would read
// as "checked". CSS keyframes only, and still under prefers-reduced-motion.
import { ACCENT, AMBER, Beam, Figure, Part, SKY, Star, VIOLET } from "./BuildWays";

// LocalKnowledgeFigure: documents on this computer run into the Knowledge
// Creator, a point of light, which writes one pack, sealed here by a small
// padlock.
const FOCUS = { x: 166, y: 75 };
const DOCS = [
  { y: 16, label: "notes.md", color: SKY, delay: 0, length: 100 },
  { y: 62, label: "spec.pdf", color: VIOLET, delay: 1.2, length: 86 },
  { y: 108, label: "faq.docx", color: AMBER, delay: 2.4, length: 100 },
];
const DOC = { x: 14, w: 70, h: 26 };
const PACK = { x: 218, w: 90, h: 26 };
const LOCK = { x: 253, y: 98 };

export function LocalKnowledgeFigure() {
  const glow = "kf-local-glow";
  return (
    <Figure glow={glow}>
      {DOCS.map((d) => (
        <Beam
          key={d.label}
          d={`M ${DOC.x + DOC.w} ${d.y + DOC.h / 2} C 120 ${d.y + DOC.h / 2}, 130 ${FOCUS.y}, ${FOCUS.x} ${FOCUS.y}`}
          color={d.color}
          glow={glow}
          length={d.length}
          delay={d.delay}
        />
      ))}
      <Beam d={`M ${FOCUS.x} ${FOCUS.y} L ${PACK.x} ${FOCUS.y}`} color={ACCENT} glow={glow} length={52} delay={3.4} />
      {DOCS.map((d) => (
        <Part key={d.label} x={DOC.x} y={d.y} w={DOC.w} h={DOC.h} color={d.color} label={d.label} glow={glow} delay={d.delay} />
      ))}
      {/* The Knowledge Creator: a point of light with four rays. */}
      <Star x={FOCUS.x} y={FOCUS.y} glow={glow} />
      <Part x={PACK.x} y={FOCUS.y - PACK.h / 2} w={PACK.w} h={PACK.h} color={ACCENT} label="wiki pack" glow={glow} delay={3.8} />
      {/* Sealed on this computer. */}
      <g className="cv-lock">
        <path d={`M ${LOCK.x + 4} ${LOCK.y + 8} V ${LOCK.y + 3} a 6 6 0 0 1 12 0 V ${LOCK.y + 8}`} className="cv-lock-shackle" />
        <rect x={LOCK.x} y={LOCK.y + 8} width="20" height="16" rx="3" className="cv-lock-body" />
        <circle cx={LOCK.x + 10} cy={LOCK.y + 15} r="2" className="cv-lock-hole" />
      </g>
    </Figure>
  );
}

// CloudKnowledgeFigure: a pack beams up through AI Review, which reads
// every pack the account keeps, into the account's cloud.
const UP = { x: 14, y: 104, w: 92, h: 26 };
const MARK = { x: 162, y: 80 };
const CLOUD = "M 232 70 Q 214 70 216 55 Q 218 42 232 43 Q 236 26 256 26 Q 275 26 280 40 Q 298 39 300 55 Q 300 70 285 70 Z";

export function CloudKnowledgeFigure() {
  const glow = "kf-cloud-glow";
  const { x, y } = MARK;
  return (
    <Figure glow={glow}>
      <Beam d={`M ${UP.x + UP.w} ${UP.y + UP.h / 2} C 130 ${UP.y + UP.h / 2}, 136 ${y}, ${x - 15} ${y}`} color={SKY} glow={glow} length={62} delay={0} />
      <Beam d={`M ${x + 15} ${y} C 196 ${y}, 204 62, 222 58`} color={VIOLET} glow={glow} length={50} delay={1.6} />
      <Part x={UP.x} y={UP.y} w={UP.w} h={UP.h} color={SKY} label="wiki pack" glow={glow} delay={0} />
      {/* AI Review: a ring every pack passes through. */}
      <circle cx={x} cy={y} r="15" className="cv-node-glow" style={{ fill: ACCENT }} filter={`url(#${glow})`} />
      <circle cx={x} cy={y} r="13" className="cv-mark" />
      <circle cx={x} cy={y} r="5" className="cv-mark" />
      <text x={x} y={y + 30} textAnchor="middle" className="cv-label">AI Review</text>
      <path d={CLOUD} className="cv-bubble" />
      {[[240, 56, 30], [244, 48, 40]].map(([x1, ly, len]) => (
        <line key={ly} x1={x1} x2={x1 + len} y1={ly} y2={ly} className="cv-ink" />
      ))}
    </Figure>
  );
}
