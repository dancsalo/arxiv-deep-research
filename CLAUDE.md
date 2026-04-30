# CLAUDE.md

## Language & tooling

This project uses **Go**. Each example under `examples/` is its own Go module.

- Dependencies are managed per-module via `go.mod`
- Do NOT use a global `GOPATH` workspace

## Verification commands

| Task | Command |
|------|---------|
| Build | `make build` |
| Lint (vet) | `make lint` |
| Run tests | `make test` |
| All checks | `make check` |

Always use `make` targets rather than running `go` commands directly.

## Project structure

- `examples/` — standalone Go examples (each is its own module)
  - `01-basic-tool-use/` — single tool use with Claude API
  - `02-parallel-tool-use/` — parallel tool use with Claude API
- `.claude/plans/` — implementation plans (markdown)
- `.claude/skills/` — Claude Code skill definitions

## Workflow

This project uses a two-phase workflow: **planning** then **coding**. Each phase has its own loop.

### Phase 1: Planning

Iterate until the plan is solid, then commit it. Use **plan-all** to run the full pipeline autonomously, or invoke each step manually.

```
plan-begin → [ plan-critique + plan-critique-pm ] → plan-revise → (× 2 rounds) → plan-tests → plan-finish
```

1. **plan-begin** — write an implementation plan with requirements, specs, and contracts. Saved to `.claude/plans/`.
2. **Dual critique** (run as parallel subagents):
   - **plan-critique** — grumpy staff-engineer review. Questions feasibility, completeness, and coherence.
   - **plan-critique-pm** — product manager review. Questions user value, scope, and priorities.
3. **plan-revise** — address feedback from both critiques: fix gaps, correct errors, resolve scope and design issues.
4. Repeat steps 2-3 for a second round.
5. **plan-tests** — generate a test plan from the implementation plan (what to test, inputs/outputs, verification strategy).
6. **plan-finish** — commit the plan to git, push to a feature branch, and open a PR for review.

**plan-all** — orchestrates steps 1-6 end-to-end without stopping for user input.

### Phase 2: Coding

Build from the plan, section by section, with verification at every step.

```
code-checklist → [ code-tests → code-verify → code-checkpoint ] × N → code-critique → code-revise → code-verify → (loop until clean) → code-finish
```

1. **code-checklist** — convert the plan into an ordered implementation checklist. Then for each section:
   - **code-tests** — write test code for that section from the test plan.
   - **code-verify** — run lint, typecheck, and tests. Fix failures.
   - **code-checkpoint** — commit locally once the section passes.
2. After all sections are implemented:
   - **code-critique** — grumpy staff-engineer code review.
   - **code-revise** — apply critique fixes: triage items, make changes, flag tradeoffs for discussion.
   - **code-verify** — run full test suite again.
   - Iterate on critique/revise/verify until clean.
3. **code-finish** — run all checks, push to a feature branch, and open a PR.
