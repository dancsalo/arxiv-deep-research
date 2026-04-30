---
name: code-checkpoint
description: Create a git branch (if on main) and commit recently touched files with an informative message.
user_invocable: true
---

# Code Checkpoint

When the user invokes this skill (e.g. `/code-checkpoint`, or ends their message with "checkpoint"), save progress by committing current work.

## 1. Review changes

- Run `git status` and `git diff --stat` to understand the scope.
- If there are no changes to commit, tell the user and stop.

## 2. Create a feature branch (if on main)

- If currently on `main` or `master`, create a new branch:
  - Derive the branch name from the work done (e.g. `feat/add-memory-system`, `fix/arxiv-query-parsing`).
  - Use kebab-case with a conventional prefix: `feat/`, `fix/`, `refactor/`, `docs/`, `chore/`, `component/`.
  - `git checkout -b <branch-name>`
- If already on a feature branch, stay on it.

## 3. Stage and commit

- Stage relevant changed files. Prefer specific file paths over `git add -A`.
- Do NOT stage `.env`, credentials, or secrets.
- Write a clear commit message:
  - First line: imperative, under 72 chars, summarizes the "why".
  - Body (if needed): bullet points of key changes.
  - End with: `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>`
- Use a HEREDOC for the commit message to preserve formatting.

## 4. Report back

- Summarize what was committed in 1-2 sentences.
- Do NOT push. This is a local checkpoint only.

## Important

- NEVER force-push or amend commits without explicit user request.
- NEVER push — this skill is for local checkpoints only. Use `/code-finish` to push and open a PR.
- If pre-commit hooks fail, fix the issue and create a NEW commit (do not --amend).
