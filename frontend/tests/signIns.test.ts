// The screens' words for the app's two sign-ins: one for the SecureAgentry
// registry the build is for, one for a self-hosted registry.
import { test } from "node:test";
import assert from "node:assert/strict";
import { asSentence, isSecureAgentry, origin, signInKindFor, signInLabel, signInLine, twoSignIns } from "../src/signIns.ts";

const built = { secureAgentryRegistry: "https://secureagentry.net", selfHostedRegistry: "", registries: [{ url: "https://secureagentry.net", signIn: "secureagentry" }] };
const withSelf = {
  secureAgentryRegistry: "https://secureagentry.net",
  selfHostedRegistry: "https://agentry.example.com",
  registries: [
    { url: "https://secureagentry.net", signIn: "secureagentry" },
    { url: "https://agentry.example.com", signIn: "self-hosted" },
    { url: "https://old.example", signIn: "" },
  ],
};

test("origins compare as a browser resolves them", () => {
  assert.equal(origin("https://SecureAgentry.NET:443/api"), "https://secureagentry.net");
  assert.equal(origin("https://secureagentry.net./"), "https://secureagentry.net");
  assert.equal(origin("https://b\u00fccher.example"), "https://xn--bcher-kva.example");
  assert.equal(origin("http://x.example:8080"), "http://x.example:8080");
  assert.equal(origin("not a url"), "");
});

test("SecureAgentry is the domain and its subdomains only", () => {
  assert.ok(isSecureAgentry("https://dev.secureagentry.net"));
  assert.ok(!isSecureAgentry("https://secureagentry.net.evil.test"));
  assert.ok(!isSecureAgentry("https://xsecureagentry.net"));
});

test("each registry is named for its own sign-in", () => {
  assert.equal(signInKindFor(withSelf, "https://SECUREAGENTRY.net/"), "secureagentry");
  assert.equal(signInKindFor(withSelf, "https://agentry.example.com"), "self-hosted");
  assert.equal(signInKindFor(withSelf, "https://old.example"), "");
  assert.equal(signInKindFor(withSelf, "https://another.example"), "");
  assert.equal(signInKindFor(withSelf, "https://dev.secureagentry.net"), "");
  assert.equal(signInKindFor(built, "https://another.example"), "self-hosted");
  assert.equal(signInLabel("secureagentry"), "SecureAgentry sign-in");
  assert.equal(signInLabel("self-hosted"), "Self-hosted sign-in");
  assert.equal(signInLabel(""), "No sign-in");
});

test("the sentence names both sign-ins and where they go", () => {
  assert.equal(twoSignIns(withSelf), "Agentry Desktop keeps two sign-ins: one for SecureAgentry at secureagentry.net, and one for your self-hosted registry, agentry.example.com. Each is sent only to its own registry.");
  assert.match(twoSignIns(built), /one for a self-hosted registry you add/);
  assert.match(twoSignIns({ ...built, secureAgentryRegistry: "" }), /signs in to no SecureAgentry registry/);
});

test("the sign-in screen says which sign-in a typed address is, or why none", () => {
  assert.equal(signInLine(built, "https://secureagentry.net"), "This is your SecureAgentry sign-in, sent only to secureagentry.net.");
  assert.equal(signInLine(built, "https://agentry.example.com/"), "This is your self-hosted sign-in, sent only to agentry.example.com.");
  assert.equal(signInLine(withSelf, "https://another.example"), "Agentry Desktop keeps one self-hosted registry, agentry.example.com. Remove it in Settings to use another.");
  assert.equal(signInLine(built, "https://dev.secureagentry.net"), "Agentry Desktop signs in to SecureAgentry at secureagentry.net only.");
  assert.equal(signInLine(built, ""), "");
  assert.equal(asSentence("this app keeps one, and it is https://a.example"), "This app keeps one, and it is https://a.example.");
  assert.equal(signInKindFor(built, "http://agentry.example.com"), "");
  assert.equal(signInKindFor(built, "http://localhost:8080"), "self-hosted");
  assert.match(signInLine(built, "http://agentry.example.com"), /use https/);
});
