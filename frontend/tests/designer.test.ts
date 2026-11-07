// Design with AI's rules: the inputs checked before the start in each place,
// where a run goes first, the price before the start, and the words of a
// finished run.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  addressLines, briefBytes, designerPriceLine, designerProblem, documentLine, firstWhere, hostedOutcome, lookedLine, maxImages, savePrompt, takesDocument,
  type DesignerInputs, type DesignerLimits,
} from "../src/designer.ts";

const limits: DesignerLimits = {
  maxBriefBytes: 4096, maxReferences: 4, maxCoverImages: 6, maxUrls: 2, reviewReady: true,
  offered: true, available: true, hostedMaxImages: 4, hostedUrls: true, hostedMaxUrls: 2, documentHere: false, documentInAgentry: false,
};
const design: DesignerInputs = { job: "design", brief: "Quarterly reports for a harbour trust.", from: "", logo: "", images: [], urls: [], document: "" };
const cover: DesignerInputs = { ...design, job: "cover", from: "7b0e5f2e-6a8c-4a77-9d0f-2a7f3c2b1e10" };

test("a brief is needed, counted in UTF-8 bytes, within 4 KiB", () => {
  assert.equal(briefBytes("é"), 2);
  assert.match(designerProblem({ ...design, brief: "  \n" }, "here", limits) ?? "", /works from a brief/);
  assert.equal(designerProblem({ ...design, brief: "é".repeat(2048) }, "here", limits), null);
  assert.match(designerProblem({ ...design, brief: "é".repeat(2049) }, "here", limits) ?? "", /at most 4 KiB/);
  assert.equal(designerProblem(design, "here", limits), null);
  assert.equal(designerProblem(design, "agentry", limits), null);
});

test("a design takes four reference images and two addresses; a cover is for a design and draws the images it may", () => {
  const four = ["a", "b", "c", "d"];
  assert.equal(designerProblem({ ...design, images: four, logo: "logo.png" }, "here", limits), null);
  assert.match(designerProblem({ ...design, images: [...four, "e"] }, "here", limits) ?? "", /at most 4 reference images/);
  assert.match(designerProblem({ ...cover, from: "" }, "here", limits) ?? "", /Choose the design the cover is for/);
  assert.equal(designerProblem({ ...cover, images: [...four, "e", "f"] }, "here", limits), null);
  assert.match(designerProblem({ ...cover, images: [...four, "e"] }, "agentry", limits) ?? "", /at most 4 images/);
  assert.match(designerProblem({ ...cover, logo: "logo.png" }, "here", limits) ?? "", /design's own logo/);
  assert.match(designerProblem({ ...cover, urls: ["https://example.com/"] }, "here", limits) ?? "", /no web addresses/);
  assert.equal(maxImages("cover", "here", limits), 6);
  assert.equal(maxImages("cover", "agentry", limits), 4);
  assert.equal(maxImages("design", "agentry", { ...limits, hostedMaxImages: 0 }), 4);
});

test("addresses: Go's check first, then each place's own rule", () => {
  assert.deepEqual(addressLines(" https://a.example/ \n\n https://b.example/\r\n"), ["https://a.example/", "https://b.example/"]);
  const two = { ...design, urls: ["https://a.example/", "https://b.example/"] };
  assert.equal(designerProblem(two, "here", limits), null);
  assert.match(designerProblem(two, "here", limits, "http://x: https only") ?? "", /https only/);
  assert.match(designerProblem({ ...two, urls: [...two.urls, "https://c.example/"] }, "here", limits) ?? "", /at most 2 web addresses/);
  assert.match(designerProblem(two, "here", { ...limits, reviewReady: false, reviewWhy: "AI Review reads every page" }) ?? "", /AI Review reads every page\.$/);
  assert.equal(designerProblem(two, "agentry", { ...limits, reviewReady: false }), null);
  assert.match(designerProblem(two, "agentry", { ...limits, hostedUrls: false }) ?? "", /does not read web addresses/);
  assert.match(designerProblem(two, "agentry", { ...limits, hostedMaxUrls: 1 }) ?? "", /at most 1 web addresses/);
});

test("the document's place is drawn only where the Report Designer reads one", () => {
  assert.equal(takesDocument("design", "here", limits), false);
  assert.equal(takesDocument("design", "agentry", limits), false);
  assert.equal(takesDocument("design", "here", { ...limits, documentHere: true }), true);
  assert.equal(takesDocument("cover", "agentry", { ...limits, documentInAgentry: true }), false);
  assert.match(designerProblem({ ...design, document: "brand.pdf" }, "here", limits) ?? "", /does not read a document/);
  assert.equal(designerProblem({ ...design, document: "brand.pdf" }, "here", { ...limits, documentHere: true }), null);
  assert.match(designerProblem({ ...design, document: "brand.pdf" }, "agentry", { ...limits, documentHere: true }) ?? "", /run it on this computer, or remove the document/);
  assert.equal(designerProblem({ ...design, document: "brand.pdf" }, "agentry", { ...limits, documentInAgentry: true }), null);
});

test("In Agentry goes first when it can be chosen; otherwise this computer, and In Agentry held says so", () => {
  assert.equal(firstWhere(limits), "agentry");
  assert.equal(firstWhere({ offered: true, available: false }), "here");
  assert.equal(firstWhere({ offered: false, available: false }), "here");
  assert.match(designerProblem(design, "agentry", { ...limits, available: false }) ?? "", /cannot run In Agentry/);
});

test("the price before the start, with a short balance and an unmetered payer", () => {
  assert.equal(designerPriceLine({ credits: 20, holdCredits: 20, balance: 180, balanceKnown: true }),
    "It costs 20 credits, held when it starts and refunded in full if it saves no design.");
  assert.match(designerPriceLine({ credits: 20, holdCredits: 20, balance: 12, balanceKnown: true }), /This account has 12 credits, fewer than it holds/);
  assert.match(designerPriceLine({ credits: 20, holdCredits: 20, balance: 12, balanceKnown: true, pool: "acme" }), /organization's pool has 12/);
  assert.match(designerPriceLine({ credits: 20, holdCredits: 0, balance: 0, balanceKnown: false }), /holds no credits/);
});

test("a finished run says the design it saved, or why not with the refund", () => {
  assert.match(hostedOutcome({ applied: true, name: "Northwind", statusLine: "Finished" }), /saved “Northwind”/);
  assert.equal(hostedOutcome({ applied: false, note: "The design it was to work on was deleted before it started, so nothing was saved. Its credits were refunded.", statusLine: "Finished" }),
    "The design it was to work on was deleted before it started, so nothing was saved. Its credits were refunded.");
  assert.equal(hostedOutcome({ applied: false, error: "lost touch", statusLine: "Running" }), "lost touch");
  assert.equal(lookedLine(true), null);
  assert.match(lookedLine(false) ?? "", /could not look/);
  assert.match(savePrompt("design"), /not your default/);
  assert.match(savePrompt("cover"), /stays as it is/);
});

test("what a run took from the document", () => {
  assert.equal(documentLine({}), null);
  assert.equal(documentLine({ documentPages: 1 }), "It looked at 1 page of your document.");
  assert.equal(documentLine({ documentPages: 4, logoFromDocument: true }), "It looked at 4 pages of your document, and took your logo from it.");
  assert.equal(documentLine({ documentPages: 0, logoFromDocument: true }), "It read your document, and took your logo from it.");
  assert.equal(
    documentLine({ documentPages: 4, logoFromDocument: true, coverFromDocument: true, coverPicturesFromDocument: 1 }),
    "It looked at 4 pages of your document, and took your logo from it. Its cover is modelled on your cover page, with 1 picture from it.",
  );
  assert.equal(
    documentLine({ documentPages: 2, logoFromDocument: true, coverImageFromDocument: true }),
    "It looked at 2 pages of your document, and took your logo and a cover image from it.",
  );
  assert.equal(documentLine({ documentPages: 3, coverFromDocument: true }), "It looked at 3 pages of your document. Its cover is modelled on your cover page.");
});
