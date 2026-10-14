// The browser in the desktop (plan browser-testing.md §5, D2, D9;
// agentry-desktop#878): the install review, the badge and the Library say
// it in the CLI's consent words; the run setup asks a site question as the
// address of the site to test, never as a source to fetch, and holds it to
// what the CLI's browser.ParseSite takes; and a run In Agentry is not
// offered for it until hosted runs carry one.
import { test } from "node:test";
import assert from "node:assert/strict";
import { browserBadge, browserCap, browserLine, browserReach, isSiteQuestion, siteBlockers, siteProblem } from "../src/browser.ts";
import { needList, whereFallback } from "../src/runSetup.ts";

test("the consent line is the CLI's, word for word", () => {
  const b = browserCap({ sites: ["site"], hosts: ["fonts.googleapis.com", "fonts.gstatic.com"], secrets: ["SITE_USER", "SITE_PASSWORD"] })!;
  assert.equal(
    browserLine(b),
    "a web browser that can open the site you name for site and its subdomains, and fonts.googleapis.com, fonts.gstatic.com; it can type SITE_USER, SITE_PASSWORD into the site's sign-in forms (it never sees them)",
  );
  assert.equal(
    browserLine(browserCap({ sites: ["site"], scope: "public" })!),
    "a web browser that can open the site you name for site and its subdomains, and ANY public site (policy-gated)",
  );
});

test("the badge names the sites and hosts, or any public site", () => {
  assert.equal(browserBadge(browserCap({ sites: ["site"], hosts: ["cdn.example.com"] })!), "browser: site, cdn.example.com");
  assert.equal(browserBadge(browserCap({ scope: "public" })!), "browser: any public site");
  assert.equal(browserReach(browserCap({})!), "declared");
});

test("no line is no browser, and a shape this build does not know is still one", () => {
  assert.equal(browserCap(undefined), null);
  assert.equal(browserCap(null), null);
  assert.deepEqual(browserCap("yes")?.sites, ["declared"]);
  assert.equal(browserCap({ sites: ["s"], scope: "intranet" })?.scope, "public");
});

test("a site question is a url question the browser names, and nothing else", () => {
  assert.equal(isSiteQuestion({ type: "url", site: true }), true);
  assert.equal(isSiteQuestion({ type: "url" }), false);
  assert.equal(isSiteQuestion({ type: "text", site: true }), false);
});

test("a site answer is held to what the CLI's ParseSite takes", () => {
  assert.equal(siteProblem("https://staging.example.com"), null);
  assert.equal(siteProblem("http://192.168.1.20:8080/app"), null);
  assert.match(siteProblem("ftp://example.com") ?? "", /must start with https/);
  assert.match(siteProblem("example.com") ?? "", /is not a web address/);
  assert.match(siteProblem("https://me:pw@example.com") ?? "", /user name or password/);
  assert.match(siteProblem("https://example.com/#top") ?? "", /# part/);
  const q = { id: "site", prompt: "Which site should I test?", type: "url", required: true, site: true };
  assert.deepEqual(siteBlockers([q], { site: "" }), []);
  assert.deepEqual(siteBlockers([q], { site: "example.com" }), ["The site for “Which site should I test?” is not a web address. It starts with https:// or http://."]);
});

test("what to have ready names the site, not a file to fetch", () => {
  const list = needList({ intake: [{ id: "site", prompt: "Which site should I test?", type: "url", required: true, site: true }], secrets: [] });
  assert.ok(list.some((l) => l.startsWith("The address of the site to test for “Which site should I test?”")), list.join("\n"));
  assert.ok(!list.some((l) => l.includes("https address of the file")), list.join("\n"));
});

test("Run defaults' In Agentry says why a QuickAgent with a browser runs here", () => {
  assert.equal(
    whereFallback("agentry", { offered: false, available: false, signedIn: true, browser: true }, false),
    "Your Run defaults say In Agentry, but a run In Agentry has no browser yet, so this run starts on this computer.",
  );
});
