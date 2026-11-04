import { CardFrame } from "../../cards";
import { BrowserOpenURL } from "../../host";

const DOCS = "https://docs.secureagentry.net";

// The docs pages a new person starts from, in the order to read them.
export const LEARN_PAGES: { title: string; path: string }[] = [
  { title: "Home and the Library", path: "/agentry/desktop/" },
  { title: "Running a QuickAgent", path: "/runner/" },
  { title: "Drafting one with AI", path: "/agentry/builder/" },
  { title: "Knowledge packs", path: "/agentry/knowledge/" },
  { title: "Running on Agentry", path: "/agentry/hosted-runs/" },
];

// Home's Learn card: a few docs pages, each opened in the browser. The card
// itself opens the docs' first page.
export function LearnCard() {
  return (
    <CardFrame title="Learn" onOpen={() => BrowserOpenURL(`${DOCS}/`)} className="home-card" status="The docs, for your first steps">
      <ul className="home-card-list">
        {LEARN_PAGES.map((p) => (
          <li key={p.path}>
            <button type="button" className="link build-way-part" onClick={() => BrowserOpenURL(`${DOCS}${p.path}`)}>{p.title}</button>
          </li>
        ))}
      </ul>
    </CardFrame>
  );
}
