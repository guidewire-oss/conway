# Planning assistant deployment and validation

The first increment provides three read-only workflows from a saved plan:
schedule explanations, active-agreement comparisons and review agendas. The
user guide is in app/docs.html under Planning assistant; behavior is defined by
specs/027-evidence-linked-planning-assistant.md.

Guided questions require no model configuration. Optional free-text interpretation
uses OpenAI Responses. Configure both `CONWAY_ASSISTANT_API_KEY` and
`CONWAY_ASSISTANT_MODEL` on the server, then restart it. Select a model available
to your API project that supports strict structured outputs; no model is pinned
by the application. Missing either setting leaves guided questions available.
The API key is server-only and never returned to the browser.

Users explicitly consent before each free-text request. The request sends their
question and exact initiative/team names from the authorized saved plan. It does
not send schedule results, execution snapshots or review records. A validated
model response selects one supported task and scope; all answer facts and source
links are constructed within Conway. Model instructions cannot grant write tools.

The adapter uses `store:false`, a 20-second deadline, four concurrent requests,
a 2000-byte question limit, at most 300 initiative/team names per collection,
a 128 KiB interpretation input limit and a 64 KiB response limit. Redirects are
refused; errors do not expose provider response bodies. Provider data policies
still apply: store:false is not a claim of zero data retention.
Input errors return HTTP 400 without contacting the provider; correct the question
or use guided questions for plans beyond interpretation limits. HTTP 503 indicates
unavailable interpretation and allows retry or a guided question.

Before enabling for an organization, run a live smoke with generic plan names:
ask a scheduling question, an agreement-change question and a review question;
check the selected task/scope and source links. Confirm unsupported change
requests stay read-only and model failures preserve guided access. Local tests
use a fake HTTP provider and do not establish live model quality or account
availability. Expand evaluation cases before broadening supported intents.

API schema checked on 2026-09-07 against the official
[structured outputs guide](https://developers.openai.com/api/docs/guides/structured-outputs).
The [data controls documentation](https://developers.openai.com/api/docs/guides/your-data)
describes provider retention independently of Conway's ephemeral answers.
