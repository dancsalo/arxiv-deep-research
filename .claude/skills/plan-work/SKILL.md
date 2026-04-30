---
name: plan-work
description: Shape a user's idea into a structured implementation plan and save it as a markdown file in .claude/plans/.
user_invocable: true
---

# Plan Work

When the user invokes this skill (e.g. `/plan-work`, "plan this", "write a plan for"), take their input and produce a structured implementation plan saved to `.claude/plans/` in the repo root.

## Process

### 1. Understand the request

Read the user's input carefully. If the request is vague or missing critical context, ask clarifying questions before writing the plan. Good plans come from good questions.

### 2. Research the codebase

Before writing anything, read the relevant parts of the codebase:
- What already exists that relates to this work?
- What language, patterns, and libraries does the project use?
- What interfaces or contracts will this work need to respect?
- Are there tests, configs, or infrastructure concerns?

Plans that don't match the actual codebase are fiction.

### 3. Choose a filename

- Derive from the topic: `context-management.md`, `arxiv-full-text.md`, `progressive-disclosure-refactor.md`
- Use kebab-case, no prefix
- Save to `.claude/plans/<name>.md`
- If a plan with that name exists, read it first and ask the user whether to replace or create a new one

### 4. Write the plan

Structure the plan with these sections (include only what's relevant — not every plan needs every section):

```markdown
# Title

## Problem Statement
What problem are we solving? Why does it matter? What's the current state?

## Architecture Overview
High-level design. Diagrams (ASCII art) if the system has multiple components.
How does this fit into the existing codebase?

## Detailed Design
The meat of the plan. Break into numbered sections by component or concern.
Include code samples in the correct language with realistic types and APIs.
Address error handling, edge cases, and failure modes.

## Decisions & Tradeoffs
What alternatives were considered? Why this approach over others?
What are we deliberately NOT doing?

## Implementation Order
Numbered steps. What to build first, what depends on what.
Each step should be independently testable.

## Open Questions
Things that need answers before or during implementation.
Flag anything that could change the approach.
```

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
- Do NOT implement the plan. This skill produces the document only.
- Ground the plan in reality — read the codebase first so the plan references real code, real APIs, and real constraints.
- If the plan is for a new feature, identify where it hooks into existing code.
- Keep it concise. A good plan is the shortest document that lets someone implement the work without guessing.
