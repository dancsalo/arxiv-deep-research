---
name: code-checklist
description: Convert a plan into an ordered implementation checklist, then execute it section by section with checkpoints.
user_invocable: true
---

# Code Checklist

When the user invokes this skill (e.g. `/code-checklist`, "build from the plan", "implement the plan"), read a plan from `.claude/plans/` and convert it into a checklist of implementation tasks, then execute them.

## Process

### 1. Identify the plan

- If the user specifies a plan file, use that.
- Otherwise, list `.claude/plans/*.md` and ask which one.
- Read the plan thoroughly — requirements, specs, contracts, implementation order.

### 2. Build the checklist

Convert the plan's Implementation Order (and any detail from Specs/Contracts) into a numbered checklist of discrete implementation tasks. Each task should:

- Be small enough to implement and verify in one focused session
- Have a clear "done" condition tied to a requirement or contract from the plan
- List the files to create or modify
- Note any dependencies on earlier tasks

Save the checklist using TaskCreate so progress is tracked.

### 3. Execute section by section

For each checklist item:

1. **Implement** — write the code for this section. Follow the plan's specs and contracts. Use existing patterns from the codebase.
2. **Verify** — run `code-verify` (the project's test/check commands) to confirm the section works. Fix any failures before moving on.
3. **Checkpoint** — after each section passes verification, invoke `code-checkpoint` to commit the work locally.

Do NOT skip ahead. Each section must be verified and committed before starting the next.

### 4. Report progress

After completing all sections, summarize:
- What was implemented
- How many checkpoints were made
- Any deviations from the plan and why

## Important

- Follow the plan's contracts exactly — function signatures, types, and invariants are not suggestions.
- If you discover the plan is wrong or incomplete mid-implementation, stop and tell the user. Do not silently deviate.
- Each checkpoint should be a coherent, working state — never commit broken code.
- Use subagents for independent sections when possible to parallelize work.
- Do NOT push. Checkpoints are local commits only. Use `/code-finish` when all sections are done.
