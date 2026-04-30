---
name: code-revise
description: Apply critique feedback to code — fix issues identified by code-critique, then verify the changes pass checks.
user_invocable: true
---

# Code Revise

When the user invokes this skill (e.g. `/code-revise`, "fix the critique", "address the review"), take the critique output and apply fixes to the code.

## Process

### 1. Find the critique

The critique should be in the current conversation (from a prior `/code-critique` invocation). If not, ask the user to run `/code-critique` first or paste the feedback.

### 2. Triage the critique points

Read through all critique items and categorize them:

- **Fix now** — clear code issues: missing error handling, bad structure, hardcoded values, missing timeouts, swallowed errors, copy-paste drift.
- **Discuss first** — architectural concerns, design tradeoffs, or scope questions where the right answer isn't obvious. Flag these to the user before changing anything.
- **Disagree** — critique points that are wrong or don't apply given the project's context. State your reasoning.

### 3. Apply fixes

For each "fix now" item:

1. Make the code change.
2. Keep changes minimal and focused — fix what was called out, don't refactor the neighborhood.
3. If fixing one issue reveals another, fix it only if it's directly related.

### 4. Verify

Run `make check` (or the appropriate verification commands from CLAUDE.md). Fix any failures introduced by your changes.

### 5. Report back

Summarize what you fixed, what you flagged for discussion, and what you disagreed with. Keep it brief — the diff speaks for itself.

## Important

- Do NOT expand scope beyond the critique. This is a fix pass, not a feature pass.
- Do NOT add comments explaining the critique or the fix — the commit message and PR description are the right place for that context.
- If a fix requires changing a public API or interface, flag it to the user before proceeding.
- Prefer the simplest fix that addresses the concern. The critique asked for better code, not more code.
