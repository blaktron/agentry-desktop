// A knowledge pack's pages, previewed (plan knowledge.md §3, §4.8;
// agentry-desktop#273). The pages are what a model wrote from the person's
// documents, so they render through the run-output reader (AgentMarkdown):
// no raw HTML, no image, and a link only to a web address, opened in the
// system browser. Each page is opened from the store when it is asked for
// and nothing of it is kept on disk in the clear (KnowledgeAPI.ReadPage).
//
// A pack a build left short may hold no page besides its index (the apply
// step drops the pages it cannot take): the preview says so rather than
// assuming a list (agentry-desktop#351, where a null list blanked the app).
import { useEffect, useState } from "react";
import * as KnowledgeAPI from "../../wailsjs/go/main/KnowledgeAPI";
import { knowledge } from "../../wailsjs/go/models";
import { AgentMarkdown } from "../AgentMarkdown";
import { RevealText } from "../reveal";
import { Button, ErrorBox, Modal, Spinner } from "../ui";

export default function KnowledgePreview({ registry, name, onClose }: { registry: string; name: string; onClose: () => void }) {
  const [pv, setPv] = useState<knowledge.Preview | null>(null);
  const [page, setPage] = useState<knowledge.Page | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    KnowledgeAPI.Preview(registry, name)
      .then((v) => {
        setPv(v);
        setPage(v.index ?? null);
      })
      .catch((e) => setError(String(e)));
  }, [registry, name]);

  const pages = pv?.pages ?? [];
  async function open(path: string) {
    if (!pv) return;
    if (pv.index && path === pv.index.path) {
      setPage(pv.index);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      setPage(await KnowledgeAPI.ReadPage(registry, name, path));
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }

  return (
    <Modal title={`Preview: ${name}`} onClose={onClose} wide>
      <p className="muted small">
        The pages as the Knowledge Creator wrote them from your documents. They are text: nothing in them runs, no image
        loads, and a link opens in your browser only when it is a web address.
      </p>
      <ErrorBox error={error} />
      {!pv && !error && <Spinner />}
      {pv && (
        <div className="knowledge-preview">
          <nav className="knowledge-pages" aria-label="Pages">
            {pv.index && (
              <button className={`nav-item ${page?.path === pv.index.path ? "active" : ""}`} onClick={() => open(pv.index.path)}>Index</button>
            )}
            {pages.length === 0 && <p className="muted small">No pages yet.</p>}
            {pages.map((p) => (
              <button key={p.path} className={`nav-item ${page?.path === p.path ? "active" : ""}`} title={p.path} onClick={() => open(p.path)}>
                <RevealText text={p.title} />
              </button>
            ))}
          </nav>
          <article className="knowledge-page" aria-label={page ? `Page ${page.path}` : "Page"}>
            {pages.length === 0 && (
              <p className="muted small">
                The Knowledge Creator kept no page in this pack besides its index: the build stopped, or the pages it wrote
                were dropped. Add documents with the same documents builds it again.
              </p>
            )}
            {loading && <Spinner />}
            {page && !loading && (
              <>
                <div className="mono small muted"><RevealText text={page.path} /></div>
                {page.summary && <p className="muted small"><RevealText text={page.summary} /></p>}
                <AgentMarkdown text={page.markdown} />
                {page.truncated && <p className="muted small">The page is longer than a preview shows; export the pack to read all of it.</p>}
              </>
            )}
          </article>
        </div>
      )}
      <div className="row" style={{ justifyContent: "flex-end", marginTop: 12 }}>
        <Button onClick={onClose}>Close</Button>
      </div>
    </Modal>
  );
}
