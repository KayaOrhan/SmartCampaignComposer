---
name: ai-session-export
description: Use when exporting Claude sessions into ai-sessions/ before submission. Scrubs keys and personal data.
---

# Session export

Flavour: Tool. The submission requires every session export in `ai-sessions/`, with keys and personal data removed.

## Steps
1. Export the session. Claude Code: `/export`. claude.ai: copy the conversation text. Save raw files to `ai-sessions/raw/`, which is gitignored and never committed.
2. Scrub: `bash .claude/hooks/scrub.sh ai-sessions/raw/<file>.md ai-sessions/<YYYY-MM-DD>-<slice>.md`
3. Read the scrubbed file yourself, top to bottom. No scrubber is perfect, and names, company details and pasted logs need human eyes.
4. If the script exits 1, something still looks like a key. Fix by hand, run again.
5. Add an entry to `ai-sessions/INDEX.md` (see `verify-before-done`).
6. Commit only the scrubbed file.

## What the script redacts
Anthropic, OpenAI-style, GitHub and AWS keys; `KEY=`, `TOKEN=`, `PASSWORD=` style values; Bearer tokens; emails except `@example.test`; Turkish mobile numbers; home directory usernames. Turkish text and seed data are left alone.

## Gotchas
- A claude.ai share link cannot be scrubbed after it exists. Share one only if no secret or personal data was ever pasted into that chat. Otherwise copy the text and scrub it.
- The scrubber refuses to overwrite its input. Keep the raw file until the submission is accepted.

## Provenance
Written 2026-10-21 from the case brief. Add real incidents here with dates.
