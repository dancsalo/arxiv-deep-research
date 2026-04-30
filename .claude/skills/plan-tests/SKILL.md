---
name: plan-tests
description: Generate a test plan from an implementation plan — what to test, inputs/outputs, verification strategy — saved as a companion markdown file.
user_invocable: true
---

# Plan Tests

When the user invokes this skill (e.g. `/plan-tests`, "plan the tests", "what tests do we need for this plan"), read an implementation plan from `.claude/plans/` and produce a **test plan** — a companion document describing what needs to be tested, how, and why. No test code is written.

## Process

### 1. Identify the plan

- If the user specifies a plan file, use that.
- If a plan file is open in the IDE, use that.
- Otherwise, list `.claude/plans/*.md` and ask which one.
- Read the plan thoroughly — every section, code sample, and error path.

### 2. Study the codebase context

Before writing the test plan:
- What language will the implementation be in?
- What test framework does the project use? (Check `Makefile`, existing tests, CLAUDE.md)
- Where do tests live? What naming conventions exist?
- Are there existing test patterns to follow? (Fixtures, mocking, table-driven tests, etc.)

### 3. Extract testable behaviors

Go through the plan section by section and identify:

- **Public interfaces** — every function, method, or type the plan defines that will be called by other code
- **Happy paths** — the expected behavior when inputs are valid
- **Error paths** — every failure mode, fallback, and error return the plan mentions
- **Edge cases** — boundary conditions (empty input, zero values, overflow, nil/null), off-by-one, concurrency
- **Integration contracts** — how components interact, what invariants must hold across boundaries
- **Invariants** — things the plan says must always be true (budget never negative, turns always ordered, etc.)

### 4. Write the test plan

Save to `.claude/plans/<original-name>-tests.md` with this structure:

```markdown
# Test Plan: <topic>

Source plan: `.claude/plans/<original>.md`

## Test environment
- Language: <Go, Python, etc.>
- Framework: <go test, pytest, etc.>
- Test location: <where files go>
- Run command: <make test, go test ./..., etc.>
- Mocking strategy: <interfaces, dependency injection, test doubles, etc.>

## Components to test

### <Component 1 name>

**What it does:** <one sentence>
**Source plan section:** <section number>

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | <descriptive name> | <specific input> | <specific output or assertion> | unit/integration/edge |
| 2 | ... | ... | ... | ... |

**Mocking needed:** <what external dependencies need to be mocked>
**Setup/fixtures:** <any shared test state>

### <Component 2 name>
...

## Integration tests

| # | Scenario | Components involved | Setup | Verification |
|---|----------|-------------------|-------|-------------|
| 1 | <end-to-end scenario from the plan> | <list> | <what to arrange> | <what to assert> |

## Test gaps

Behaviors in the plan that are **too vague to test** — list them here as feedback for the plan author:
- <thing that needs more specificity before a test can be written>

## Verification strategy

How to confirm the implementation is correct:
- <what to run>
- <what passing looks like>
- <what to watch for in CI>
```

### 5. Report back

- Tell the user the file path.
- Summarize: how many test cases, which components are covered, what gaps exist.
- Call out any plan ambiguities discovered — this is design feedback.

## Important

- Do NOT write test code. This skill produces a test plan document only.
- Every test case must trace back to something specific in the implementation plan. No speculative tests.
- Be specific about inputs and outputs — "passes valid input" is useless; "passes a 44-char English string, expects 11 tokens" is testable.
- If the plan is too vague to define a test for a section, say so in the Test Gaps section rather than inventing behavior.
- The test plan should be detailed enough that `/code-tests` can implement it without re-reading the implementation plan.
