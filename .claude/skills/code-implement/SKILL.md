---
name: code-implement
description: Implement a single checklist item from the plan — write production code for one section following specs and contracts.
user_invocable: true
---

# Code Implement

When the user invokes this skill (e.g. `/code-implement`, "implement this section", "build section 3"), write the production code for a single checklist item from the implementation plan.

## Process

### 1. Identify the task

- The task should be provided as context: a checklist item with its "done" condition, files to create/modify, and dependencies.
- If no task is specified, ask the user which checklist item to implement.
- Read the implementation plan and test plan for full context on specs, contracts, and expected behavior.

### 2. Study the codebase

Before writing code:
- Read existing source files to match conventions: naming, file organization, error handling patterns, import style.
- Check what already exists — earlier checklist items may have created files or interfaces this task depends on.
- Read existing tests (if written by `code-tests`) to understand expected behavior.

### 3. Write the implementation

- Follow the plan's specs and contracts exactly — function signatures, types, and invariants are not suggestions.
- Match codebase conventions for style, error handling, and structure.
- Keep changes minimal and focused on the single checklist item.
- If stubs exist from `code-tests`, replace them with real implementations.
- Do NOT add features, abstractions, or error handling beyond what the plan specifies.

### 4. Report back

- List files created or modified.
- Note which spec/contract items are now fulfilled.
- Flag anything that couldn't be implemented as specified and why.

## Important

- Implement ONE checklist item only. Do not bleed into adjacent sections.
- Follow the plan's contracts exactly. If a contract is wrong, flag it — do not silently deviate.
- Do NOT run tests or checks — that's `code-verify`'s job.
- Do NOT commit — that's `code-checkpoint`'s job.
- If the task depends on an earlier task that hasn't been implemented, stop and report the blocker.
