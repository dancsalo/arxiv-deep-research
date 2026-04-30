---
name: code-finish
description: Run checks, commit, push to a feature branch, and open a GitHub PR.
user_invocable: true
---

# Code Finish

When the user invokes this skill (e.g. `/code-finish`, "ship it", "we're done", "open a PR"), run the full sequence to get code changes into a PR.

## 1. Run checks

- Run `make lint` to catch lint issues. Fix any that are in files YOU changed (not pre-existing ones).
- Run `make test` if tests exist for the changed code.
- If checks fail on your changes, fix them before proceeding.

## 2. Review changes

- Run `git status` and `git diff --stat` to understand the scope.
- Summarize what was done in 2-3 sentences for the user.
- If there are no changes to commit, tell the user and stop.

## 3. Create a feature branch (if on main)

- If currently on `main` or `master`, create a new branch:
  - Derive the branch name from the work done (e.g. `feat/add-memory-system`, `fix/arxiv-query-parsing`).
  - Use kebab-case with a conventional prefix: `feat/`, `fix/`, `refactor/`, `docs/`, `chore/`.
  - `git checkout -b <branch-name>`
- If already on a feature branch, stay on it.

## 4. Stage and commit

- Stage relevant files. Prefer specific file paths over `git add -A`.
- Do NOT stage `.env`, credentials, or secrets.
- Write a clear commit message:
  - First line: imperative, under 72 chars, summarizes the "why".
  - Body (if needed): bullet points of key changes.
  - End with: `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>`
- Use a HEREDOC for the commit message to preserve formatting.

## 5. Push

- Push the branch: `git push --set-upstream origin <branch-name>`

## 6. Open the PR

- Use `gh pr create --base main` with:
  - A concise title (under 72 chars).
  - A body with:
    ```
    ## Summary
    <1-3 bullet points describing the changes>

    ## Test plan
    - [ ] <verification steps>

    🤖 Generated with [Claude Code](https://claude.com/claude-code)
    ```
- Use a HEREDOC for the body.

## 7. Return to main

- `git checkout main`

## 8. Report back

- Print the PR URL so the user can click through.

## Important

- NEVER force-push or amend commits without explicit user request.
- NEVER push directly to main/master — always use a feature branch.
- If `gh` is not authenticated, tell the user to run `! gh auth login`.
- If pre-commit hooks fail, fix the issue and create a NEW commit (do not --amend).
- Ask for confirmation before pushing if the diff is larger than expected.
