// Design harness. Not shipped in the app bundle: it lives outside
// tsconfig's `include` and the app's Vite entry. It renders the shell chrome
// and the primitive layer against the real style.css and fonts so the UI can
// be screenshotted (Playwright, light and dark) without webkit or the Go
// backend.
import { createRoot } from "react-dom/client";
import "../src/style.css";
import {
  Button, Card, PageHeader, Field, Input, Select, Textarea, Toolbar,
  EmptyState, Spinner, ErrorBox, Notice, Modal,
  Mark, Wordmark, Sev, ModeBadges, SignedBadge, CapabilityBadges, QuarantineBadge,
} from "../src/ui";
import { SidebarNav } from "../src/nav";
import type { trust } from "../wailsjs/go/models";

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="shell" style={{ height: "auto", minHeight: "100vh" }}>
      <div className="sidebar">
        <div className="brand"><Wordmark sub="Desktop" /></div>
        <SidebarNav page="browse" />
        <div className="foot">
          <div>v0.1.0</div>
          <div className="nowrap">secureagentry.net</div>
        </div>
      </div>
      <div className="content">{children}</div>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{ marginBottom: 26 }}>
      <h2>{title}</h2>
      {children}
    </div>
  );
}

const verified: trust.AttestationResult = { state: "verified", keyId: "a1b2c3d4", detail: "DSSE verified" } as trust.AttestationResult;
const unsigned: trust.AttestationResult = { state: "unsigned", keyId: "", detail: "no attestation" } as trust.AttestationResult;

function Gallery() {
  return (
    <Shell>
      <PageHeader
        title="Design harness"
        sub="The shell chrome and the shared primitives, in the website's design language."
        actions={<><Button variant="ghost">Secondary</Button><Button variant="primary">Primary action</Button></>}
      />

      <Section title="Buttons">
        <div className="row">
          <Button variant="primary">Primary</Button>
          <Button>Default</Button>
          <Button variant="ghost">Ghost</Button>
          <Button variant="danger">Danger</Button>
          <Button variant="link">Link</Button>
          <Button variant="primary" busy busyLabel="Working…">Busy</Button>
          <Button disabled>Disabled</Button>
        </div>
      </Section>

      <Section title="Badges">
        <div className="row" style={{ marginBottom: 8 }}>
          <span className="badge kind">kind</span>
          <span className="badge ok">ok</span>
          <span className="badge warn">warn</span>
          <span className="badge bad">bad</span>
          <span className="badge">neutral</span>
          <QuarantineBadge state="held" />
        </div>
        <div className="row" style={{ marginBottom: 8 }}>
          {["critical", "high", "medium", "low", "info"].map((s) => <Sev key={s} severity={s} />)}
        </div>
        <div className="row" style={{ marginBottom: 8 }}>
          <ModeBadges modes={["autonomous", "interactive"]} />
          <SignedBadge att={verified} />
          <SignedBadge att={unsigned} />
        </div>
        <div className="row">
          <CapabilityBadges caps={{ network: { egress: ["api.example.com"] }, filesystem: { read: "workspace", write: { paths: ["out"] } }, exec: ["jq"], secrets: ["API_TOKEN"] }} />
        </div>
      </Section>

      <Section title="Cards & report">
        <Card>
          <div className="spread">
            <div className="grow">
              <div className="row"><strong className="mono">demo1/incident-summarizer</strong> <ModeBadges modes={["autonomous"]} /> <SignedBadge att={verified} /></div>
              <p className="muted small" style={{ margin: "6px 0 0" }}>Summarises an incident channel into a report card. Scanned, 0 findings, signed.</p>
            </div>
            <Button variant="primary">Install</Button>
          </div>
        </Card>
        <Card>
          <table className="plain">
            <thead><tr><th>version</th><th>digest</th><th>scanned</th></tr></thead>
            <tbody>
              <tr><td className="mono">1.2.0</td><td className="mono">sha256:9c35…</td><td><span className="badge ok">0 findings</span></td></tr>
              <tr><td className="mono">1.1.0</td><td className="mono">sha256:d20f…</td><td><span className="badge warn">2 medium</span></td></tr>
            </tbody>
          </table>
        </Card>
      </Section>

      <Section title="Forms">
        <Card>
          <Field label="Registry" hint="The registry this app talks to.">
            <Input placeholder="https://secureagentry.net" defaultValue="https://secureagentry.net" />
          </Field>
          <Field label="Provider">
            <Select defaultValue="anthropic"><option>anthropic</option><option>openai</option><option>custom</option></Select>
          </Field>
          <Field label="Task" hint="What the QuickAgent should do.">
            <Textarea rows={3} defaultValue="Summarise the last 24h of the incident channel." />
          </Field>
        </Card>
        <Toolbar>
          <Input type="search" className="grow" placeholder="Search agents…" />
          <Button variant="primary">Search</Button>
        </Toolbar>
      </Section>

      <Section title="Messages">
        <Notice tone="info">A neutral status line — waiting for your browser to authorize this app.</Notice>
        <Notice tone="ok">Signed in as @dupres — token stored in the OS keychain.</Notice>
        <Notice tone="warn">No container engine answers. Install one to run QuickAgents.</Notice>
        <ErrorBox error="verify failed: digest mismatch (exit 10)" />
      </Section>

      <Section title="Empty & loading">
        <EmptyState title="No installed QuickAgents yet" action={<Button variant="primary">Browse the registry</Button>}>
          Install one from the registry to see it here.
        </EmptyState>
        <Spinner label="Resolving…" />
      </Section>

      <Section title="Type">
        <p>Prose is set in IBM Plex Sans. Values — <code>demo1/incident-summarizer@1.2.0</code>, <code>sha256:9c35df…</code>, <code>ANTHROPIC_API_KEY</code> — are set in IBM Plex Mono, one visual register for “this is a value”.</p>
      </Section>
    </Shell>
  );
}

createRoot(document.getElementById("root")!).render(<Gallery />);
