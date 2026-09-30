// The AI Review key's recommendation (plan desktop-review-key-and-run-cards.md
// D1, D12; agentry-desktop#461): the operator's words, the link's label and
// where the link goes, in one place, so every surface that recommends a key
// says the same thing and sends the person to the same address. The address
// is a constant: no registry answer and no setting can change it. Pure, so
// node's own test runner can hold the words without a browser.

/** Where a person gets a TypeSafe key. Opened in the system browser. */
export const TYPESAFE_URL = "https://typesafe.ai";

/** Why a person who builds QuickAgents should have their own key. */
export const KEY_RECOMMENDATION =
  "Agentry's AI Review is powered by TypeSafe AI's Jev System One. We highly recommend signing up with TypeSafe and " +
  "getting an API key if you want to build shareable QuickAgents or submit them to Agentry.";

export const GET_A_KEY_LABEL = "Get a key from TypeSafe";
