---
name: campaign-domain
description: Use when touching campaigns, segments, consent, templates or channel limits. Domain model, segment DSL, state machines.
---

# Campaign domain

Flavour: Component. Describes what the system is. Delivery mechanics live in `delivery-pipeline`, Claude calls in `llm-contract`.

## Pipeline
brief -> segment DSL (Claude) -> validated query -> audience preview -> messages (Claude) -> validation -> human edit and approve -> snapshot into deliveries -> worker -> mock provider -> dashboard.

## Entities
- `users(id, first_name, last_name, email, phone, city, city_norm, language, created_at)`
- `consents(user_id, channel, granted, updated_at)`: one row per channel. A missing row means no consent.
- `cart_events(user_id, product_id, added_at)` and `orders(user_id, placed_at, total)`
- `campaigns(id, brief, channels, segment_json, status, revision, messages_validated_revision, message_validation_preview, created_at, approved_at)`
- `campaign_messages(campaign_id, channel, subject, body)`: complete selected-channel set; explicit saves do not establish generated/human provenance.
- `deliveries`: defined in `delivery-pipeline`.

## Seed data
Deterministic (fixed RNG seed), 5000 users. Emails end in `@example.test`; phone values are non-dialable identifiers (`mock-phone-000001`), never invented real-world numbers. Seed once transactionally when `SEED_DATA=true`, recording its version and UTC reference time. Repeated startup must preserve profile and consent edits. The dataset ages naturally; tests inject a fixed reference time. Include uneven consent per channel, missing consent, repeated carts, purchases and city spelling variants.

## Segment DSL (version 1)
```json
{
  "version": 1,
  "all": [
    {"field": "city", "op": "in", "value": ["istanbul"]},
    {"field": "cart_added", "op": "within_days", "value": 30},
    {"field": "purchase", "op": "none_within_days", "value": 30}
  ]
}
```
- Allowed: `city` (`in`, 1 to 20 nonempty strings of at most 120 runes), `language` (`in`, 1 to 20 language codes of at most 16 characters), `cart_added` (`within_days`, integer 1 to 365), `purchase` (`within_days` or `none_within_days`, integer 1 to 365).
- Only `all` (AND), 1 to 8 conditions. Reject unknown/case-mismatched keys, duplicate JSON keys, wrong types, nulls, unsupported versions, malformed UTF-8 and trailing JSON. HTTP request JSON is capped at 32768 bytes. Canonical and stored JSONB documents are revalidated with a separate 262144-byte cap to allow serialization expansion; every structural and domain validator still applies. Nesting depth is capped at 16.
- Capture one UTC `as_of` for the entire preview; event windows are `[as_of - N * 24h, as_of)`. Use EXISTS/NOT EXISTS to avoid duplicates from repeated events.
- Consent is NOT part of the DSL. The query compiler adds the channel consent filter itself, always. Claude can never remove it.
- `none_within_days` means "no order in the last N days". It is a simplification of "added to cart and did not buy". The trade-off is recorded in DECISIONS.md.

## Message contract
```json
{"sms": {"body": "..."}, "email": {"subject": "...", "body": "..."}}
```
- `sms.body` at most 160 characters, `email.subject` at most 80, measured after rendering. `email.body` is non-empty and at most 2000 characters.
- Template variables use exactly `{{name}}`. Allowlist: `first_name`, `city`. Unknown variable is rejected.
- Worst case rendering: compute separate maximum rune lengths for BOTH `first_name` and `city` over the consent-filtered audience snapshot. Render using both maxima together, including every repeated variable occurrence; the maxima may come from different recipients. Apply this conservative bound to SMS body, email subject and email body. Reject malformed or unknown placeholders before rendering. Empty audiences cannot be approved. Revalidate against the snapshot in the approval transaction so changes since preview cannot invalidate the bound.

## State machines
- Campaign: `draft -> content_ready -> approved -> sending -> completed`, and any state -> `cancelled`.
- Segment and message writes require the current campaign revision through If-Match; successful writes increment it once. Segment writes from draft/content_ready retain saved text, clear validation metadata and return to draft. Message saves recompute consent-filtered bounds and validate the complete selected-channel set inside one short repeatable-read transaction before replacing rows and moving to content_ready. Rejected writes change nothing. No full content history.
- Message generation is preview-only and allowed from draft/content_ready with a segment. Read campaign and audience in one snapshot, then close the transaction before calling the provider. Final validated includes base_revision; an explicit save uses that revision and revalidates against current bounds. Never silently retry stale content with a new revision. content_ready is not approval or a frozen audience; stored validation metadata is historical.
- Approve is legal only from `content_ready`. Editing content moves back to `content_ready` and re-runs validation. No edits after approval. The implemented approval/queue increment atomically passes through approved to sending, increments the revision once and inserts pending deliveries; sending currently means queued, with no worker implemented. Require the reviewed audience-preview fingerprint and current If-Match. A reconciled repeat with the current revision and original fingerprint is read-only; stale tokens still fail. Canonical endpoint/error contracts are in README and ADR-023.
- The audience and rendered payloads are frozen into `deliveries` at approval (ADR-014). Revalidate against the approval snapshot; if membership or content differs from the preview, return an updated preview for human approval rather than silently approving a changed audience. Never recompute membership or personalize from mutable profiles at send time. Consent revocation and cancellation may reduce actual sends; show skipped deliveries separately.

## Gotchas
- Normalize cities with NFC, trimmed whitespace and `cases.Lower(language.Turkish)`. Explicitly map the resulting `ıstanbul` and `istanbul` aliases to `istanbul`; locale lowercasing alone does not unify English ASCII I with Turkish dotted I. Apply the same function to seed data and filters. Tests cover composed/decomposed dotted I and ASCII upper/lower variants. Do not strip accents globally.
- SMS encoding: the brief says 160 characters, but Turkish letters (`ş ğ ı İ`) force UCS-2, where a single SMS holds 70. Hard limit stays 160 (that is the spec). Compute an informational `sms_segments` and warn the marketer. Decision in DECISIONS.md.
- "Last 30 days" is relative to an injected clock in UTC. Tests never call `time.Now()` directly.
- Count the characters of the rendered string as runes, not bytes.

## Provenance
Written 2026-10-21 from the case brief. Add real incidents here with dates.
