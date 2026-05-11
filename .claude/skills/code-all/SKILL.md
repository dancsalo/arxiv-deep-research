---
name: code-all
description: Run the full coding pipeline end-to-end — checklist, implement, critique, revise, verify, and checkpoint each section — without stopping for input.
user_invocable: true
---

# Code All

When the user invokes this skill (e.g. `/code-all`, "code everything", "full code pipeline", "implement the plan end-to-end"), run the entire coding workflow autonomously from start to finish. **Do NOT stop to ask the user questions at any point.** Make reasonable decisions and document them. The user will review the PR at the end.

## Overview

This is **Phase 2** of the two-phase development workflow: **planning** then **coding**. Build from the plan, section by section, with verification at every step. This skill runs the full pipeline autonomously, or you can invoke each step manually.

The pipeline:
```
code-checklist → [ code-implement → code-critique → code-revise (loop) → code-verify → code-revise (if failures) → code-checkpoint ] × N sections → code-finish
```

Steps:
1. **code-checklist** — convert the plan into an ordered implementation checklist. Then for each section:
   - **code-implement** — write production code for that checklist item following specs and contracts.
   - **code-critique** — grumpy staff-engineer code review of the section.
   - **code-revise** — apply critique fixes (loop with critique until approved, max 3 rounds).
   - **code-verify** — run lint and tests. Fix failures.
   - **code-revise** — if verification fails, fix and re-verify.
   - **code-checkpoint** — commit locally once the section passes all checks.
2. **code-finish** — run all checks, push to a feature branch, and open a PR.

**code-all** orchestrates step 1 end-to-end without stopping for user input.

## Prerequisites

- An implementation plan must exist in `.claude/plans/`.
- A test plan (`*-tests.md`) should exist. If it does not, generate one inline before starting.

## Pipeline

```
code-checklist → [ code-implement → code-critique → code-revise (loop) → code-verify → code-revise (if failures) → code-checkpoint ] × N sections
```

## Execution

### Step 1: Build the Checklist

Follow the code-checklist skill instructions to convert the plan into discrete tasks:

- If the user specifies a plan file, use that. Otherwise, list `.claude/plans/*.md` (excluding `*-tests.md`) and pick the most recent one. **Do NOT ask the user to choose.**
- Read the plan thoroughly — requirements, specs, contracts, implementation order.
- Convert the Implementation Order into a numbered checklist of discrete tasks.
- Each task must have: a clear "done" condition, files to create/modify, and dependencies on earlier tasks.
- Save the checklist using TaskCreate so progress is tracked.

### Step 2: Execute Each Section

For each checklist item, run this inner loop **in sequence** (do NOT parallelize across sections — each depends on the previous):

#### 2a. Implement

Write the production code for this section following code-implement skill instructions:

- Follow the plan's specs and contracts exactly.
- Match existing codebase patterns.
- Replace any stubs from `code-tests` with real implementations.
- Keep changes focused on the single checklist item.

#### 2b. Critique

Spawn an **Agent subagent** with the code-critique persona to review the implementation:

> You are a grumpy, cynical staff engineer reviewing code changes. Run `git diff HEAD` to see the recent changes. Critique across these dimensions: Structure & Design, Error Handling & Edge Cases, Staff-Level Thinking, Things That Smell. Return a numbered list of observations with pointed questions and suggestions. End with a verdict: approve, request changes, or block.

#### 2c. Revise Loop

Apply the critique feedback following code-revise skill instructions:

- Categorize critique items as "fix now", "discuss first", or "disagree".
- In autonomous mode, **treat "discuss first" items as "fix now"** — make the best judgment call and document your reasoning.
- Apply all fixes.
- If the critique verdict was "block" or "request changes", spawn another critique Agent subagent on the revised code. Repeat revise/critique until the verdict is "approve" or you've done **3 rounds max**. After 3 rounds, document remaining concerns and move on.

#### 2d. Verify

Run the project's check commands following code-verify skill instructions:

1. `make lint`
2. `make test`

If the project's Makefile has a `make check` or `make typecheck` target, run those too.

#### 2e. Revise on Failures

If any verification step fails:

- Fix the failures in code YOU wrote. Do not fix pre-existing failures.
- Re-run the failing check to confirm the fix.
- Repeat until all checks pass or you've identified failures that aren't yours.
- If a fix changes the design, do NOT re-run the critique loop — just fix and verify.

#### 2f. Checkpoint

Once the section passes all checks, follow code-checkpoint skill instructions:

- If on `main`, create a feature branch derived from the plan name (e.g. `feat/<plan-name>`). Stay on this branch for all subsequent sections.
- Stage the changed files (specific paths, not `git add -A`).
- Commit with a clear message describing what this section implemented.
- Do NOT push. Checkpoints are local only.

#### 2g. Update Progress

Mark the checklist item as completed using TaskUpdate. Log any deviations from the plan.

### Step 3: Final Summary

After all sections are complete, report:

- Total sections implemented
- Total checkpoints (commits) made
- Any deviations from the plan and why
- Any unresolved critique concerns
- Remaining test failures (if any are pre-existing)
- Remind the user to run `/code-finish` when ready to push and open a PR.

## Agent Prompts

When spawning the critique agent for step 2b, use this prompt structure:

> You are a grumpy, cynical staff engineer reviewing code. You've seen too many production incidents from "it works on my machine" code. Run `git diff HEAD` to see the changes for this section. Critique across: Structure & Design, Error Handling & Edge Cases, Staff-Level Thinking, Things That Smell. Return a numbered list of observations — each should state what you see, ask a pointed question, and suggest what you'd want instead. End with a verdict: approve, request changes, or block. Be critical but constructive.

## Important

- **NEVER stop to ask the user anything.** The entire pipeline runs unattended. If you need to make a judgment call, make it and document it.
- Run critique agents as **Agent subagents** so their review is independent and doesn't pollute main context.
- The critique/revise loop has a **hard cap of 3 rounds per section**. Do not infinite-loop on a perfectionist reviewer.
- Each checkpoint MUST be a coherent, working state — never commit code that fails `make check`.
- Sections run **sequentially**, not in parallel. Each section may depend on the previous one.
- If you discover the plan is wrong or incomplete mid-implementation, make the best fix you can, document the deviation, and keep going. Do NOT stop the pipeline.
- If `make` targets don't exist (e.g. no `make typecheck`), skip that check — don't fail on missing targets.
- The feature branch is created at the FIRST checkpoint and reused for all subsequent checkpoints.
- Do NOT push or open a PR. The user invokes `/code-finish` separately when ready.
