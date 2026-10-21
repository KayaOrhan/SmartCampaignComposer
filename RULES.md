# Rules

Hard rules for humans and agents. Exceptions are allowed only if they are written in the Exceptions section of this file. A skill, a comment or a chat message cannot grant one.

## Must never
1. Execute LLM output as SQL, shell or code. LLM output is data: parse it, validate it against a schema and an allowlist, then build the query in our own code with bound parameters.
2. Send to any host except the mock provider. Startup config validation refuses a provider URL that is not allowlisted. No real SMS or email SDK may appear in dependencies.
3. Send on a channel without consent. Consent is applied when the audience is built and checked again right before each send. A missing consent row means no consent.
4. Send the same (campaign, recipient, channel) twice. A database unique constraint enforces this; application logic alone is not enough.
5. Commit, log or print secrets or personal data. That includes API keys, `.env`, unmasked phone numbers or emails in logs, and recipient data inside prompts.
6. Call the real Anthropic API from tests or CI. Under `APP_ENV=test` the real client refuses to construct.
7. Read or write `.env`. Use `.env.example` for variable names.
8. Add a dependency that was suggested by an AI without checking it exists on the registry, who maintains it, and that the version is pinned.
9. Disable, skip or delete a failing test, lint rule or validator to get green. Fix the cause.
10. Bypass git hooks (`--no-verify`) or force-push.

## Must always
- Validate at every boundary and fail closed. Unknown field, operator, template variable or channel is rejected with a specific error.
- Enforce channel limits on the rendered worst case (longest allowed `first_name` in the audience), not on the template.
- Pass `context.Context` with a timeout to every outbound call: LLM, provider, database.
- Keep operations idempotent: approve, enqueue and every send retry are safe to repeat.
- Store and compare time in UTC. Write absolute dates in docs (`2026-10-21`), never "yesterday" or "Thursday".
- Log structured JSON with `campaign_id` and `delivery_id`. No personal data.
- Wrap errors with context. Never swallow one.
- Record every non-obvious decision in DECISIONS.md: context, decision, alternatives, consequences.
- List what was cut or is weak in the README "Known gaps" section.

## AI-assisted work
- Treat agent output as untrusted until reviewed: hallucinated APIs and packages, wrong assumptions, hardcoded secrets or URLs, missing validation, string-built SQL.
- Read the diff, not the summary.
- A passing test counts only if it fails when the behaviour it covers is broken.
- Review reports list problems only. No praise, no filler.

## Documentation format
- One canonical home per fact. Everything else links to it. References point one way.
- Skill `description` starts with "Use when", at most 20 words and 150 characters. It is loaded on every turn, so it is paid for on every turn.
- Rule bodies use categorical examples. Dates, incident ids and one-off details go in a Provenance section.
- English in repo docs. No hidden or bidirectional Unicode characters in any file.

## Enforcement map
Every rule should be enforced in more than one place. If a rule has only a document, treat it as unenforced.
- Rule 1: `internal/segment` exposes only a query builder that takes validated types. Test `TestLLMOutputNeverReachesSQL`, plus an injection-style fixture.
- Rule 2: config validation at startup, a test for it, and `make lint` greps for provider SDK imports.
- Rules 3 and 4: schema constraints plus integration tests (concurrent workers, mid-flight kill, consent revoked after approval).
- Rules 5 and 7: `.claude/hooks/guard.sh`, `permissions.deny` in `.claude/settings.json`, secret scan in `make lint`.
- Rule 6: constructor guard plus test config.
- Rules 8, 9, 10: guard hook for 10, review for 8 and 9, CI diff check for removed or skipped tests.

## Exceptions
None.
