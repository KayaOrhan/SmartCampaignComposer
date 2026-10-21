# Smart Campaign Composer

A marketer writes a campaign brief in natural language. Claude turns it into an audience segment plus SMS and email copy. A human previews, edits and approves. A queue delivers to a **mock** provider and a dashboard shows live status. Nobody real ever receives a message.

Hard rules (security, correctness, scope) live in @RULES.md and always apply. If this file and RULES.md disagree, RULES.md wins.

## Stack
Go (stdlib `net/http`) API and worker, Postgres (data and queue), Vue 3 + TypeScript + Vite, Claude API through the official SDK, Docker Compose. Reasons are in DECISIONS.md.

## Layout
- `cmd/api`, `cmd/worker`, `cmd/mockprovider`
- `internal/{segment,llm,campaign,delivery,store,config}`
- `web/` frontend, `seed/` deterministic fake data, `ai-sessions/` exported sessions

## Commands
- `make up` start everything (compose, seed, mock provider)
- `make test` all tests, no external network or real API key; local Postgres and mock HTTP services are allowed
- `make lint` linters and secret scan
- `make check` lint + test; must be green before anything is called done

## Working agreement
1. Plan first when a change touches more than two files or any of `segment`, `llm`, `delivery`. Write the plan, wait for confirmation.
2. Build the thinnest end-to-end slice first (brief -> segment -> messages -> approve -> send -> dashboard, all on mocks). Harden after that. Bonus features come last and only if the core is solid.
3. Tests ship with the code, not after it.
4. Claims need evidence: test output, a log line, or `file:line`. Anything else is labelled "hypothesis". This applies to chat replies too.
5. Look before you build: grep for an existing validator, helper or type before writing a new one.
6. If a requirement is ambiguous and the answer changes a DECISIONS.md entry, ask instead of guessing.

## Skills (invoke the skill before the first related edit in a session)
- `campaign-domain`: entities, segment DSL, limits, state machines, consent
- `llm-contract`: any Claude call, prompt, schema, validation, the mock LLM
- `delivery-pipeline`: queue, worker, rate limit, retries, idempotency, mock provider
- `verify-before-done`: before saying "done", opening a PR, or ending a session
- `ai-session-export`: before putting anything into `ai-sessions/`

## Context hygiene
Keep this file short. Deep knowledge belongs in skills. Do not paste large logs or files into the conversation; use `rg` and read ranges. When a mistake costs more than ten minutes, propose a one-line addition to the relevant skill's Gotchas. Propose changes to RULES.md, never edit it silently.

## Environment
Config comes from environment variables only. Names are listed in `.env.example`. Never read or print `.env`.
