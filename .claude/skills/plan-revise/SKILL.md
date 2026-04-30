---
name: plan-revise
description: Apply critique feedback to a plan — revise the document, address open questions, and fix gaps identified by plan-critique.
user_invocable: true
---

# Plan Revise

When the user invokes this skill (e.g. `/plan-revise`, "fix the plan", "address the critique"), take the critique output and revise the plan document accordingly.

## Process

### 1. Find the plan and the critique

- If the user points to a specific plan file, use that.
- Otherwise, list files in `.claude/plans/` and ask which one to revise.
- The critique should be in the current conversation (from a prior `/plan-critique` invocation). If not, ask the user to run `/plan-critique` first or paste the feedback.

### 2. Read the plan and referenced code

Read the full plan. If it references existing files, functions, or APIs, read those too — revisions that don't match reality are just new fiction.

### 3. Address each critique point

Work through the critique items systematically:

- **Feasibility issues** — fix incorrect API references, wrong language, or mismatched types. Verify against the actual codebase.
- **Missing pieces** — add error handling, failure modes, edge cases, or sections the critique identified as absent.
- **Architectural problems** — restructure sections that contradict each other, collapse unnecessary abstractions, clarify ownership.
- **Open questions that are actually decisions** — make the decision or flag it explicitly with tradeoffs. Don't leave load-bearing questions unanswered.
- **Code sample fixes** — correct language, types, error handling, and consistency across samples.

### 4. Mark what changed

At the bottom of the plan, add or update a `## Revision log` section with a brief summary of what changed and why, referencing the critique points addressed.

### 5. Report back

Summarize what you changed in 2-3 sentences. List any critique points you intentionally did NOT address, with a reason.

## Important

- Edit the existing plan file in place. Do NOT create a new file.
- Do NOT add content beyond what the critique calls for — this is revision, not expansion.
- If a critique point is wrong or doesn't apply, say so and explain why rather than making a bad change.
- If addressing a critique point requires information you don't have (e.g. a product decision), flag it to the user instead of guessing.
