<!-- Pull requests go into `dev`; `main` takes only promotions from `dev`.
One concern per pull request. Conventional-commit subject. -->

## What and why

Closes #

## Verification

<!-- The tests and checks you ran, on which platform, and what you did not
verify. A screenshot for a visible change. -->

## Checklist

- [ ] `make check` passes
- [ ] every commit is signed off (DCO, CONTRIBUTING.md)
- [ ] a changed bound method carries regenerated bindings (`make bindings`)
- [ ] no tokens, keys or secret values in the diff, the tests or the logs
