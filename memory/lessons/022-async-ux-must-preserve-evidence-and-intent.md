# Async interfaces must preserve evidence and user intent

Observed 2026-09-05 while exercising the planning and Help workflows in local
Chrome against an isolated server and fixture APIs.

- A successful unit renderer can still receive the wrong integration field:
  Home was comparing a clamped queue-model input with the overcapacity threshold.
  Use measured `load` for that claim and retain explicit synthetic/missing states.
  Provenance: `app/js/main.js` load construction; `tests/shell-ux.test.mjs`
  uncapped-load and synthetic-data regressions.
- A modal close during Bootstrap's opening animation can be ignored. Queue the
  close intent until the shown event and restore focus to a visible invoking
  control, including a dropdown toggle. Provenance: `app/js/modal.js` and
  `tests/shell-ux.test.mjs` lifecycle regression; observed via local Chrome
  Help/import dialog interactions.
- Route, overlay and plan identities must be checked after asynchronous body
  reads. A request for an earlier plan or reused preview must not replace the
  current task. Provenance: `tests/proposal-navigation.test.mjs`.
- Test narrow layouts after asynchronous sizing settles. The snapshot select
  auto-fit and populated plan navigation overflowed 360px even though an empty
  shell fit. Provenance: `tests/browser/planning-ux.mjs` and the observed local
  Chrome viewport check; controls now wrap or constrain their own scroll area.

These observations point to the implementation and regression tests; spec 017
remains the source of requirements and decision rationale.
