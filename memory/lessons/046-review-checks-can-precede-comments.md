# Review completion can precede comment delivery

Do not treat a successful external review check as proof that every review
comment has arrived. Read all thread pages again after completion and allow a
follow-up observation before declaring the review finished or pausing monitoring.
Keep the claim tied to the inspected commit and time; later comments can still
require another correction.

Provenance: observed 2026-09-07 through the GitHub PR API for
https://github.com/guidewire-oss/conway/pull/83. The external review check reported
success at 19:19 UTC, while seven additional review threads were created around
19:21 UTC. A prior complete thread read had contained only the already-resolved
input-validation comment. The later findings begin at
https://github.com/guidewire-oss/conway/pull/83#discussion_r3952182695.
