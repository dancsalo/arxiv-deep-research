---
name: plan-begin
description: Shape a user's idea into a structured implementation plan with requirements, specs, and contracts in .claude/plans/.
user_invocable: true
---

# Plan Begin

When the user invokes this skill (e.g. `/plan-begin`, "start a plan", "begin planning", "plan this", "write a plan for"), take their idea and produce a structured implementation plan saved to `.claude/plans/` in the repo root as a single markdown file.

## Process

### 1. Understand the request

Read the user's input carefully. If the request is vague or missing critical context, ask clarifying questions before writing. A plan without clear requirements is a wish list.

### 2. Research the codebase

Before writing anything, read the relevant parts of the codebase:
- What already exists that relates to this work?
- What language, patterns, and libraries does the project use?
- What interfaces or contracts will this work need to respect?
- Are there tests, configs, or infrastructure concerns?
- If the plan is for a new feature, identify where it hooks into existing code.

Plans that don't match the actual codebase are fiction.

### 3. Choose a filename

- Derive from the topic: `context-management.md`, `arxiv-full-text.md`, `progressive-disclosure-refactor.md`
- Use kebab-case, no prefix
- Save to `.claude/plans/<name>.md` in the repo root
- If a plan with that name exists, read it first and ask the user whether to replace or create a new one

### 4. Write the plan

The plan MUST contain all of the following sections:

#### Problem Statement

What problem are we solving? Why does it matter? What's the current state?

#### Requirements

What must this feature/change do? Numbered list of concrete, testable requirements. Each requirement should be verifiable — if you can't write a test for it, it's not a requirement, it's a vibe.

- Functional requirements (what it does)
- Non-functional requirements (performance, cost, reliability constraints)
- Out of scope (what this explicitly does NOT do)

#### Specs

The technical specification for how the requirements will be met:

- **Architecture** — high-level design, component diagram (ASCII), how it fits into the existing codebase
- **Detailed design** — broken into numbered sections by component. Include code samples in the correct language with realistic types and APIs. Address error handling and edge cases.
- **Data flow** — how data moves through the system, what transforms happen where
- **Error handling** — what fails, how it fails, what happens when it does

#### Contracts

The interfaces between components — the promises each piece makes to the others:

- **Public API** — function signatures, method signatures, endpoint definitions with types
- **Input/output contracts** — what each function accepts and returns, including error cases
- **Invariants** — things that must always be true (ordering guarantees, budget constraints, state consistency)
- **Dependencies** — what this code requires from other components, and what it provides to them

#### Decisions & Tradeoffs

What alternatives were considered? Why this approach over others? What are we deliberately NOT doing?

#### Implementation Order

Numbered steps. What to build first, what depends on what. Each step should be independently testable.

#### Open Questions

Things that need answers before or during implementation. Flag anything that could change the approach.

### 5. Code samples

When including code samples:
- Use the project's actual language (check existing files)
- Reference real types, packages, and APIs from the codebase
- Include error handling — happy-path-only samples are a lie
- Keep samples focused — show the interesting parts, not boilerplate

### 6. Report back

Tell the user the file path and give a 2-3 sentence summary of the plan.

## Important

- Always save to `.claude/plans/` in the repo root — never to `~/.claude/plans/`
- All mandatory sections (Problem Statement, Requirements, Specs, Contracts) must be present. If you can't fill one, explain why and flag it as an open question. Decisions & Tradeoffs, Implementation Order, and Open Questions may be omitted if genuinely not relevant.
- Do NOT implement the plan. This skill produces the document only.
- Ground the plan in reality — read the codebase first so the plan references real code, real APIs, and real constraints.
- Keep it concise. A good plan is the shortest document that lets someone implement the work without guessing.
