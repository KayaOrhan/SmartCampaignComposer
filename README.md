# SmartCampaignComposer

Mock-only campaign composer. Architecture and workflow: [CLAUDE.md](CLAUDE.md). Safety contract: [RULES.md](RULES.md). Proposed implementation: [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md).

## Status

Documentation and empty subsystem directories exist. No runtime, dependency manifests, migrations, tests, Makefile or Compose services have been implemented. Architecture decisions remain unverified proposals unless explicitly superseded.

## Known gaps

- Purchase exclusion approximates cart abandonment as no order within the selected period; see ADR-002 in [DECISIONS.md](DECISIONS.md).
- SMS segment estimates are informational and approximate; see ADR-005.
- One worker owns each channel. Multiple replicas require shared rate limiting; see ADR-013.
- Consent revocation can race with HTTP dispatch after the final lookup; see ADR-013.
- Conservative template bounds can reject copy that fits individual recipients; see ADR-014.
- Codex guardrails in AGENTS.md are logical instructions. Runtime enforcement, secret scanning and Git/CI checks are not implemented yet.
- Session scrubbing and Claude hooks have not been executed or verified in this scaffold.
- Scheduled sends, A/B variants, per-user language and CI remain deferred under ADR-011. Core safety validators and tests are mandatory.
