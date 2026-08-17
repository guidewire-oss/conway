# Spec Template

Local copy of the template Conway specs follow, so the format is available
offline and in-context for agents. Source of truth:
<https://github.com/anoop2811/ai-craft/blob/main/SPEC_TEMPLATE.md> — when the
upstream template changes, re-copy it here.

Conventions for this repo (see `CLAUDE.md`):

- Specs live in `specs/`, one file per feature.
- File naming: `NNN-kebab-case-name.md` (numbered, stable IDs for
  cross-referencing).
- Fill sections top-down: problem before solution, stories before requirements.
- Mark unknowns `[NEEDS CLARIFICATION]` rather than assuming.
- The spec is a decision record. Tests capture what the system does; the spec
  captures why. Keep it alive as understanding evolves.
- Not a design document: say WHAT and WHY in the requirements; technical
  choices, alternatives, and their consequences belong in §11 Decision Record.

---

```markdown
# [Feature Name]

**Status:** [Draft | In Review | Approved | In Progress | Done]
**Author(s):** [names]
**Date:** [created date]
**Story/Ticket:** [link to Jira/Linear/GitHub issue]
**Sprint/Cycle:** [if applicable]

---

## 1. Overview

[2-3 sentences: What is being built and why it matters. A busy person should
be able to read this section alone and understand the feature.]

---

## 2. Problem

[What specific problem does this solve? Who feels the pain? What happens
if we don't solve it?]

[Include a concrete example or user story that illustrates the problem.
A real scenario is worth more than an abstract description.]

---

## 3. User Stories

### Story 1: [Short title]

**As a** [type of user]
**I want** [capability]
**So that** [benefit/outcome]

---

## 4. Acceptance Criteria

[Given/When/Then. These become your BDD scenarios and your definition of
"done." Cover happy paths AND edge cases.]

### Story 1: [Title]

**AC 1.1: [Short description]**

> Given [precondition or initial state]
> When [action or event]
> Then [observable, testable outcome]

---

## 5. Functional Requirements

[Specific, testable requirements with identifiers for traceability.
RFC 2119 language: MUST, SHOULD, MAY, MUST NOT.]

| ID | Requirement | Priority |
|----|------------|----------|
| FR-001 | The system MUST [specific behavior] | MUST |

---

## 6. Non-Functional Requirements

| ID | Requirement | Threshold | How to Verify |
|----|------------|-----------|---------------|
| NFR-001 | [attribute] | [measurable threshold] | [method] |

---

## 7. Data Model

### Entities

**[EntityName]**
- [attribute]: [type] — [description]

### Relationships

- [EntityA] has many [EntityB]

---

## 8. API Contract

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| GET | /path | [description] | — | [schema reference] |

---

## 9. Out of Scope

- [Feature or capability explicitly excluded]

---

## 10. Open Questions

| # | Question | Owner | Target Date | Resolution |
|---|----------|-------|-------------|------------|
| Q1 | [NEEDS CLARIFICATION] [question] | [name] | [date] | [pending / resolved: answer] |

---

## 11. Decision Record

### Decision 1: [Short title]

**Context:** [What situation or constraint led to this decision?]

**Decision:** We will [specific decision].

**Alternatives considered:**
- [Alternative A] — rejected because [reason]

**Consequences:** [What becomes easier or harder as a result?]

---

## 12. Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| [metric name] | [baseline] | [target] | [measurement method] |

---

## Review Checklist

- [ ] Problem is clearly stated and justified
- [ ] User stories represent real user value
- [ ] Acceptance criteria are in Given/When/Then format
- [ ] Edge cases and error scenarios are covered
- [ ] Requirements use MUST/SHOULD/MAY language
- [ ] Non-functional requirements have measurable thresholds
- [ ] Out of Scope is explicit
- [ ] Open questions are marked, owned, and time-bound
- [ ] No implementation details in the requirements (WHAT/WHY, not HOW)
- [ ] AI can read this spec (markdown, in the repo)
```
