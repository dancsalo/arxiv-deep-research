---
name: code-verify
description: Run tests and checks against specific code to verify correctness.
user_invocable: true
---

# Code Verify

When the user invokes this skill (e.g. `/code-verify`, "verify this", "run the tests"), run the project's test and check commands against the relevant code.

## Process

### 1. Determine scope

- If the user specifies files or a component, focus on those.
- If invoked after a checklist section, verify the code just written.
- If no scope given, run the full suite.

### 2. Run checks

Run the verification commands from CLAUDE.md / Makefile in this order:

1. **Lint** — `make lint` — catch style and static analysis issues first.
2. **Type check** — `make typecheck` — catch type errors.
3. **Tests** — `make test` — run the test suite.

If the project has more specific test commands (e.g. running a single test file), use those when the scope is narrow.

### 3. Report results

- If all checks pass: report success, list what was checked.
- If any check fails:
  - Show the failure clearly (error message, file, line).
  - Fix the issue if it's in code YOU wrote.
  - If it's a pre-existing failure, report it but don't fix it unless asked.
  - Re-run the failing check after the fix to confirm.

### 4. Iterate

Keep fixing and re-running until all checks pass or you've identified failures that aren't yours to fix. Report the final state.

## Important

- Always use `make` targets as specified in CLAUDE.md — never run pytest, ruff, or pyright directly.
- Do NOT skip checks. Run all three (lint, typecheck, test) unless the user explicitly asks for a subset.
- If tests fail on code you didn't write, report it but don't silently fix unrelated code.
