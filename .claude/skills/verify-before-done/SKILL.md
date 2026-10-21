---
name: verify-before-done
description: Use when verifying completion, preparing a PR or ending an implementation session.
---

# Verify before done

Flavour: Convention.

## Definition of done
For a documentation-only task, inspect the actual diff and validate changed paths and metadata. Report that application checks were not run; do not claim runtime verification. A read-only task does not require writing a session entry.

1. `make check` is green and the actual output is quoted, not paraphrased.
2. New behaviour has a test that fails without the change. If it was cheap to try, say that you watched it fail.
3. Name which RULES.md safety rules the change touches and which test proves each.
4. DECISIONS.md is updated if a decision was made or changed.
5. README "Known gaps" is updated if anything was cut.

## Review pass
Do this as a separate pass, as if reviewing a junior's pull request. Read the diff, not your own summary.
- What input makes this crash, duplicate, hang or leak?
- Which assumption is untested? Label it "hypothesis".
- Does every imported package and every called function exist, with the semantics assumed? Compiling is not proof.
- Any hardcoded secret, URL or magic number? Missing validation? SQL built from strings? Template or shell injection?
- Did the test pass for the right reason? Break the code on purpose and confirm the test fails.
- Is there anything beyond the task: new dependency, new abstraction, unrelated file?

Report format: problems only, one per item, with `file:line`, severity (blocker, major, minor) and evidence. No praise, no summary of what is fine. Empty report is one line: "No findings."

## Session discipline
- One session per vertical slice. Name it after the slice.
- Before ending, add four lines to `ai-sessions/INDEX.md`: what I delegated, what I kept, where the agent was wrong, how I caught it. This is the walkthrough the reviewers ask for. Writing it while it is fresh costs two minutes.

## Provenance
Written 2026-10-21 from the case brief. Add real incidents here with dates.
