# CLAUDE.md

## Language & tooling

This project uses **Go** as a single module (`github.com/dancsalo/arxiv-deep-research`).

- Dependencies are managed via the root `go.mod`
- Do NOT use a global `GOPATH` workspace

## Verification commands

| Task | Command |
|------|---------|
| Build | `make build` |
| Lint (vet) | `make lint` |
| Run all tests | `make test` |
| Go tests only | `make test-go` |
| Race detector | `make test-race` |
| Python tests only | `make test-python` |
| All checks | `make check` |
| Run server | `make run-server` |

Always use `make` targets rather than running `go` commands directly.

## Project structure

- `cmd/server/` — HTTP server binary (agentic loop with SSE streaming)
- `memoryclient/` — memory store client (search, store, bootstrap, migrations)
- `server/` — HTTP server and SSE handler packages
- `tools/` — tool definitions, size estimators, handlers
- `services/embedding-api/` — Python embedding service
- `.claude/plans/` — implementation plans (markdown)
- `.claude/skills/` — Claude Code skill definitions

## Workflow

This project uses a two-phase workflow: **planning** then **coding**. Each phase has its own loop.

### Phase 1: Planning

Iterate until the plan is solid, then commit it. Use **plan-all** to run the full pipeline autonomously, or invoke each step manually.

```
plan-begin -> [ plan-critique + plan-critique-pm ] -> plan-revise -> (x 2 rounds) -> plan-tests -> plan-finish
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

Build from the plan, section by section, with verification at every step. Use **code-all** to run the full pipeline autonomously, or invoke each step manually.

```
code-checklist -> [ code-implement -> code-critique -> code-revise (loop) -> code-verify -> code-revise (if failures) -> code-checkpoint ] x N -> code-finish
```

1. **code-checklist** — convert the plan into an ordered implementation checklist. Then for each section:
   - **code-implement** — write production code for that checklist item following specs and contracts.
   - **code-critique** — grumpy staff-engineer code review of the section.
   - **code-revise** — apply critique fixes (loop with critique until approved, max 3 rounds).
   - **code-verify** — run lint and tests. Fix failures.
   - **code-revise** — if verification fails, fix and re-verify.
   - **code-checkpoint** — commit locally once the section passes all checks.
2. **code-finish** — run all checks, push to a feature branch, and open a PR.

**code-all** — orchestrates step 1 end-to-end without stopping for user input.
