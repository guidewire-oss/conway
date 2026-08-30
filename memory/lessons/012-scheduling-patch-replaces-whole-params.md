# PATCH /scheduling replaces the whole SchedulingParams — partial bodies wipe fields

## Lesson

`savePlanScheduling` (server/planhandlers.go) decodes the request body into a
fresh `SchedulingParams` and saves it whole: `PATCH {"wipModel":"drum-gated"}`
silently deletes `periodStart`, every calendar window, and every other field the
body omitted. The in-app form is safe only because `schedulingFromForm` always
sends the complete object. Any scripted/API client (or a future partial-form
save) loses data with a 200 OK.

Two options if this bites: merge-with-saved server-side (field-by-field, the
`InitiativeEdit` pointer pattern is the model), or document the endpoint as
full-replace. Until then: when dates vanish from a schedule after an API
scheduling change, check whether the plan's `periodStart` was wiped.

## Provenance

- Observed 2026-08-25 while testing the reference plan via curl: after
  `PATCH {"wipModel":"drum-gated"}`, `/schedule` returned `periodStart: null`
  and every dated initiative became `no-date`; the stored plan showed
  `scheduling: {wipModel: ...}` alone. Confirmed by reading
  `savePlanScheduling` (server/planhandlers.go:456).
