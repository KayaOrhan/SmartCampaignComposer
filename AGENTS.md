
# Smart Campaign Composer — Codex Instructions

Read and follow these project documents:
- RULES.md — mandatory security and correctness rules.
- CLAUDE.md — architecture, workflow and conventions.
- DECISIONS.md — architecture decisions and tradeoffs.
- IMPLEMENTATION_PLAN.md — implementation phases, scope and progress.

If instructions conflict, RULES.md takes precedence.

## Workflow

1. Inspect existing code and relevant skills before editing.
2. For changes touching more than two files, or segment,
   LLM or delivery logic, propose a plan and wait for approval.
3. Build the smallest working end-to-end slice first.
4. Write tests alongside implementation.
5. Never claim success without running relevant checks.
6. Review the actual diff before committing.
7. Do not commit without explicit approval.


## Product Requirements

The original case study is located at docs/CASE_STUDY.md.

Read it before planning or implementing features.

It is the source of truth for product requirements,
constraints, evaluation criteria and deliverables.

RULES.md defines additional implementation safety
constraints. DECISIONS.md documents our technical choices.

If a technical decision conflicts with the case study,
report the conflict and request clarification.

## Skills

At session start read AGENTS.md, RULES.md, CLAUDE.md and DECISIONS.md. Before the first related edit, read the complete file below using a read-only file tool (PowerShell: `Get-Content -LiteralPath '<path>'`). A catalog description is not a substitute. Load every applicable skill when work crosses subsystems. If a required file is missing or unreadable, stop dependent edits and report the path; do not invent its contract.

| Work | Required file |
| --- | --- |
| Campaigns, segment DSL, consent, templates, channel limits | `.claude/skills/campaign-domain/SKILL.md` |
| LLM calls, prompts, schemas, validation, streaming, mock LLM | `.claude/skills/llm-contract/SKILL.md` |
| Approval/enqueue, queue, workers, limiter, retries, provider | `.claude/skills/delivery-pipeline/SKILL.md` |
| Verification, PR preparation, session completion | `.claude/skills/verify-before-done/SKILL.md` |
| Writing or exporting session records | `.claude/skills/ai-session-export/SKILL.md` |

Follow referenced resources before the operation that needs them. Re-read a skill if it changes during the session.

## Codex logical guardrails

These instructions apply before EVERY tool call, regardless of shell or tool name. They are agent-level constraints; `.claude/settings.json` and its hook do not install a Codex sandbox or Git hook. Runtime validators, database constraints and tests remain mandatory.

- Inspect the entire proposed operation, including pipelines, redirections, nested commands and scripts, before executing it. If its effect cannot be determined safely, stop that operation and report why. Never use an alternate tool to evade a denial.
- Never read, source, import, print, search contents of, write or edit `.env`, `.env.*`, `*.pem`, `id_rsa` or `id_ed25519`, matching basenames case-insensitively on Windows and Unix paths. `.env.example`, `.env.sample` and `.env.template` are template exceptions only; never put real secrets in them. Use explicit safe paths and exclusions for recursive content searches. Do not follow a link to a protected file.
- Never stage secrets or raw session exports. Do not use broad staging such as `git add .`, `git add -A` or `git commit -a`. Stage only explicitly reviewed paths; inspect staged filenames and the safe staged diff before committing. If a protected file is already staged, stop without displaying its contents and report the filename.
- Never bypass hooks using `--no-verify`, hook-path overrides, environment switches or equivalent mechanisms. Never force-push, including `--force`, `-f`, `--force-with-lease` or a `+` refspec. Fix the underlying failure instead.
- Never print or log secrets or recipient data. Do not read credential stores or private keys. Guard failures fail closed; a missing guard dependency is not permission to bypass the guard.
- No commit without explicit user approval. Plans that touch more than two files or segment, LLM or delivery logic require approval before implementation. Explicit approval of named documentation changes permits those edits only.

## API Contract & Postman Testing

- The repository's Postman collection is maintained under `postman/`.
- Treat the Go HTTP handlers and domain contracts as the source of truth for implemented API behavior.
- Whenever an API endpoint is added, removed, or its request/response contract changes, update the corresponding Postman requests and assertions in the same task.
- For API bug fixes, add a Postman regression test when the bug is observable over HTTP.
- Reuse collection variables such as `baseUrl` and `campaignId`. Never hardcode generated resource IDs.
- Use the official Postman plugin and its skills when available. Do not duplicate or override its instructions.
- Run affected collection tests against the local Docker Compose environment when available. Report pass/fail results and any tests that could not run.
- Never claim Postman tests passed without executing them.
- Run Postman tests only against local or explicitly approved test environments. Never trigger real SMS/email delivery or use real customer data.
- Keep collections synchronized with API changes, but do not change API behavior solely to satisfy outdated Postman tests.
- Follow the existing planning and approval workflow before modifying files.


## Safety

- Never read or print .env or secrets.
- Never execute LLM-generated SQL.
- Never send real SMS or email.
- Never call the real Claude API from tests.
- Never bypass failing tests or Git hooks.
- Never silently change RULES.md.
