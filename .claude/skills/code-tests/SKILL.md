---
name: code-tests
description: Write test files from a test plan — implements the actual test code with stubs, following project conventions.
user_invocable: true
---

# Code Tests

When the user invokes this skill (e.g. `/code-tests`, "write the tests", "implement the test plan"), read a **test plan** from `.claude/plans/` and write the actual test files.

This skill expects a test plan document (produced by `/plan-tests`) as input. If no test plan exists, tell the user to run `/plan-tests` first.

## Process

### 1. Find the test plan

- If the user specifies a test plan file, use that.
- Look for `*-tests.md` files in `.claude/plans/`.
- If none exist, tell the user: "No test plan found. Run `/plan-tests` first to define what needs testing."

### 2. Read the test plan and the source plan

- Read the test plan thoroughly — every component, test case, input/output, and mock requirement.
- Read the source implementation plan it references (linked in the test plan header) for additional context.
- Understand the full picture: what's being built, what's being tested, what's being mocked.

### 3. Study existing test patterns

Before writing any code:
- Read existing test files in the project to match conventions exactly.
- Check: naming (`Test_` vs `test_` vs `describe`), file organization, fixture patterns, assertion style, mock/stub approach.
- Check CLAUDE.md for the test run command.
- Match the language from the test plan's "Test environment" section.

### 4. Create type stubs (if needed)

Since implementation may not exist yet (TDD):
- Create minimal type/interface stubs — just enough for tests to compile and express intent.
- Put stubs in the implementation package, not in the test files.
- Mark them clearly: `// TODO: implement` or equivalent.
- Stubs should define the **public interface** only — no logic, no private helpers.
- Include constructor functions that tests will call (e.g., `NewTokenEstimator`, `NewContextBudget`).

### 5. Write the test files

For each component in the test plan:
- Create one test file per component, placed where the project expects tests.
- Name each test after the **behavior**, matching the test plan's table rows.
- For each test case in the plan:
  - **Arrange**: set up the specific input from the test plan.
  - **Act**: call the public interface.
  - **Assert**: check the specific expected output from the test plan.
- Group tests by component, ordered from simple (construction, basic math) to complex (multi-step, integration).
- Use table-driven tests where the test plan has many similar cases for one function.

### 6. Handle external dependencies

- Mock external services (API clients, databases, network calls) using the project's established mocking patterns.
- If the test plan says "Mocking needed: X", create the mock/stub for X.
- For API clients: use interfaces + test doubles, not monkey-patching.
- Never make real network calls in unit tests.

### 7. Verify tests compile and fail correctly

- Run the test command from CLAUDE.md / Makefile.
- Tests should **fail with assertion errors or missing-implementation errors**, not syntax or compilation errors.
- Fix any compilation issues — the tests must be valid code.
- If tests pass unexpectedly (stub returns zero value that matches expected), adjust the test to be more specific.

### 8. Report back

- List all test files created with their paths.
- Report: total test count, pass/fail breakdown, which failures are expected (TDD) vs unexpected.
- Note any test plan entries that couldn't be implemented and why.

## Important

- Do NOT write implementation code beyond minimal stubs. The stubs should have empty function bodies or return zero values / errors.
- Follow the project's existing test patterns exactly — don't introduce new frameworks, assertion libraries, or styles.
- Use the correct test run command from CLAUDE.md — never run test tools directly.
- Every test must trace back to a specific row in the test plan. No speculative tests.
- Prefer real assertions over `t.Skip()` / `pass` — a test that asserts nothing teaches nothing.
- If a test plan entry is too vague to implement, skip it and note the gap in the report.
