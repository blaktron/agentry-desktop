# Security policy

The desktop app is part of the Agentry open core; one policy covers all of
it. **Do not open a public issue for a security vulnerability.**

Report it as described in the core's
[SECURITY.md](https://github.com/blaktron/agentry/blob/main/SECURITY.md):
email **security@secureagentry.net** (or use GitHub's private vulnerability
reporting on this repository) with what you found, how to reproduce it and
what an attacker gains. The same response targets apply (a human reads it
within 2 business days; an assessment within 5; a fix or mitigation for
high/critical issues within 30 days; credit with the advisory if you want it).

Particularly interesting here:

- **Verification bypass** — an install that proceeds on an archive whose
  digest or DSSE attestation does not verify, or that accepts a signing key
  the registry did not publish (keys are pinned at login).
- **Runner containment** — a BriefAgent run from the app reaching the
  network, the filesystem, a subprocess or a secret its manifest did not
  declare (the app runs the same runner as the CLI).
- **Credential handling** — the browser sign-in's one-time code or the
  minted token reaching a log, the frontend bundle, or a file it should not.
- **The frontend boundary** — anything from a registry (README, manifest,
  findings) rendered in the app without going through the sanitized path.

Malicious content on a registry is reported to that registry (the report
control on the agent page, or its abuse address), not here.
