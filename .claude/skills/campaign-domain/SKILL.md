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
- `campaigns(id, brief, segment_json, status, created_at, approved_at)`
- `campaign_messages(campaign_id, channel, subject, body, edited_by_human)`
- `deliveries`: defined in `delivery-pipeline`.

## Seed data
Deterministic (fixed RNG seed), about 5000 users. Emails end in `@example.test` and phones come from a reserved fake range, so seed data can never be mistaken for real people. Make the data awkward on purpose: uneven consent per channel, some users with no consent at all, carts followed by a purchase, city spellings like `Istanbul` and `İstanbul`. Easy data hides bugs.

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
- Allowed: `city` (`in`, 1 to 20 strings), `language` (`in`), `cart_added` (`within_days`, 1 to 365), `purchase` (`within_days` or `none_within_days`, 1 to 365).
- Only `all` (AND), at most 8 conditions. Anything else is rejected, not ignored.
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
- Approve is legal only from `content_ready`. Editing content moves back to `content_ready` and re-runs validation. No edits after `approved`.
- The audience and rendered payloads are frozen into `deliveries` at approval (ADR-014). Revalidate against the approval snapshot; if membership or content differs from the preview, return an updated preview for human approval rather than silently approving a changed audience. Never recompute membership or personalize from mutable profiles at send time. Consent revocation and cancellation may reduce actual sends; show skipped deliveries separately.

## Gotchas
- Turkish case folding: `strings.ToLower("İstanbul")` yields an `i` plus a combining dot, which will not equal `istanbul`. Normalise with `golang.org/x/text/cases` and `language.Turkish`, and store `city_norm` at seed time. Test with `İstanbul`, `ISTANBUL`, `istanbul`.
- SMS encoding: the brief says 160 characters, but Turkish letters (`ş ğ ı İ`) force UCS-2, where a single SMS holds 70. Hard limit stays 160 (that is the spec). Compute an informational `sms_segments` and warn the marketer. Decision in DECISIONS.md.
- "Last 30 days" is relative to an injected clock in UTC. Tests never call `time.Now()` directly.
- Count the characters of the rendered string as runes, not bytes.

## Provenance
Written 2026-10-21 from the case brief. Add real incidents here with dates.
