---
name: plan-all
description: Run the full planning pipeline end-to-end — begin, critique (dual perspectives), revise loop, test plan, and finish — without stopping for input.
user_invocable: true
---

# Plan All

When the user invokes this skill (e.g. `/plan-all`, "plan everything", "full plan pipeline"), run the entire planning workflow autonomously from start to finish. **Do NOT stop to ask the user questions at any point.** Make reasonable decisions and document them. The user will review the PR at the end.

## Overview

This is **Phase 1** of the two-phase development workflow: **planning** then **coding**. Iterate until the plan is solid, then commit it. This skill runs the full pipeline autonomously, or you can invoke each step manually.

The pipeline:
```
plan-begin → [ plan-critique + plan-critique-pm ] → plan-revise → (x 2 rounds) → plan-tests → plan-finish
```

Steps:
1. **plan-begin** — write an implementation plan with requirements, specs, and contracts. Saved to `.claude/plans/`.
2. **Dual critique** (run as parallel subagents):
   - **plan-critique** — grumpy staff-engineer review. Questions feasibility, completeness, and coherence.
   - **plan-critique-pm** — product manager review. Questions user value, scope, and priorities.
3. **plan-revise** — address feedback from both critiques: fix gaps, correct errors, resolve scope and design issues.
4. Repeat steps 2-3 for a second round.
5. **plan-tests** — generate a test plan from the implementation plan (what to test, inputs/outputs, verification strategy).
6. **plan-finish** — commit the plan to git, push to a feature branch, and open a PR for review.

**plan-all** orchestrates steps 1-6 end-to-end without stopping for user input.

## Pipeline

```
plan-begin → [ plan-critique (2 agents) → plan-revise ] × 2 → plan-tests → plan-finish
```

## Execution

### Step 1: Plan Begin

Write the implementation plan following the plan-begin skill instructions:

- Read the user's request and the relevant codebase.
- **Do NOT ask clarifying questions.** If something is ambiguous, make a reasonable assumption and document it in the Open Questions section.
- Choose a filename and save to `.claude/plans/<name>.md`.
- Write all required sections: Problem Statement, Requirements, Specs, Contracts, Decisions & Tradeoffs, Implementation Order, Open Questions.

### Step 2: Dual Critique (Round 1)

Spawn **two Agent subagents in parallel** to critique the plan from different angles:

1. **Agent 1 — Grumpy Staff Engineer** (plan-critique persona): Review the plan for feasibility, completeness, architectural coherence, and second-order effects. Follow the plan-critique skill instructions exactly.

2. **Agent 2 — Product Manager** (plan-critique-pm persona): Review the plan for user value, scope, priorities, and whether it solves the right problem. Follow the plan-critique-pm skill instructions exactly.

Both agents must:
- Receive the full plan file path so they can read it
- Return a numbered list of critique points
- End with a verdict

### Step 3: Revise (Round 1)

Apply both critiques to the plan following plan-revise skill instructions:

- Work through every critique point from BOTH reviewers.
- Address feasibility issues, missing pieces, scope concerns, and user value gaps.
- Make decisions on open questions rather than deferring them.
- Update the Revision Log section.

### Step 4: Dual Critique (Round 2)

Spawn **two Agent subagents in parallel again** — same two personas, same plan file. This round validates the revisions and catches anything new.

### Step 5: Revise (Round 2)

Apply the second round of critiques. At this point the plan should be converging. If a critique point was already addressed, note it and move on.

### Step 6: Plan Tests

Generate the test plan following plan-tests skill instructions:

- Read the revised plan.
- Study the codebase for test framework, patterns, and conventions.
- Extract testable behaviors and write the test plan.
- Save to `.claude/plans/<name>-tests.md`.

### Step 7: Plan Finish

Commit and open a PR following plan-finish skill instructions:

- Create a feature branch from main: `plan/<name>`.
- Stage BOTH the plan file and the test plan file.
- Commit with a clear message.
- Push and open a PR.
- Return to main.
- Report the PR URL.

## Agent Prompts

When spawning critique agents, use prompts structured like this:

**For the Staff Engineer agent:**
> You are a grumpy, cynical staff engineer reviewing a design document. Read the plan at `.claude/plans/<name>.md` and critique it following these dimensions: Feasibility & Grounding, Completeness, Architectural Coherence, Second-Order Effects, and Code Samples. Return a numbered list of observations with pointed questions and suggestions. End with a verdict: approve, revise, or reject.

**For the Product Manager agent:**
> You are a sharp, experienced product manager reviewing a design document. Read the plan at `.claude/plans/<name>.md` and critique it following these dimensions: Problem & User Value, Scope & Priority, User Experience & Outcomes, Risk & Sequencing, and What's Missing. Return a numbered list of observations with pointed questions and simpler alternatives. End with a verdict: greenlight, scope down, or suggest a different approach.

## Important

- **NEVER stop to ask the user anything.** The entire pipeline runs unattended. If you need to make a judgment call, make it and document it.
- Run the two critique agents **in parallel** (single message, multiple Agent tool calls) in both rounds.
- The plan-revise steps are done by YOU (the main agent), not subagents — you need the full context of both critiques to resolve conflicts between them.
- If the two critiques contradict each other (e.g. engineer wants more detail, PM wants less scope), favor the PM on scope and the engineer on correctness. Document the tradeoff in Decisions & Tradeoffs.
- Two rounds of critique/revise is the fixed count. Do not add more rounds or skip rounds.
- If `gh` is not available or not authenticated, still complete everything up to the push, then report the blocker.
