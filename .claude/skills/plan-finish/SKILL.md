---
name: plan-finish
description: Commit a plan document, push to a feature branch, and open a GitHub PR for review.
user_invocable: true
---

# Plan Finish

When the user invokes this skill (e.g. `/plan-finish`, "ship the plan", "PR for this plan"), take a plan document from `.claude/plans/` through the commit-push-PR flow.

## 1. Find the plan

- If the user specifies a plan file, use that.
- If there's a plan file open in the IDE, use that.
- Otherwise, list files in `.claude/plans/` and ask which one to ship.

## 2. Review the plan

- Read the plan file to understand what it covers.
- Run `git status` to confirm the plan file has changes to commit.
- If the plan file is already committed with no new changes, tell the user and stop.

## 3. Create a feature branch (if on main)

- If currently on `main` or `master`, create a new branch:
  - Derive the branch name from the plan topic: `plan/context-management`, `plan/arxiv-full-text`, etc.
  - Use the prefix `plan/` with kebab-case.
  - `git checkout -b <branch-name>`
- If already on a feature branch, stay on it.

## 4. Stage and commit

- Stage the plan file (and any related files like diagrams referenced by the plan).
- Write a commit message:
  - First line: `Add <topic> design plan` or `Update <topic> design plan`
  - Body: 1-2 sentences summarizing what the plan covers.
  - End with: `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>`
- Use a HEREDOC for the commit message.

## 5. Push

- Push the branch: `git push --set-upstream origin <branch-name>`

## 6. Open the PR

- Use `gh pr create --base main` with:
  - Title: `Plan: <topic>` (under 72 chars)
  - Body:
    ```
    ## Summary
    Design plan for <topic>.

    <2-3 bullet points covering what the plan proposes>

    ## Review focus
    - [ ] Is the approach sound?
    - [ ] Are there missing failure modes or edge cases?
    - [ ] Do the code samples match the actual codebase?

    🤖 Generated with [Claude Code](https://claude.com/claude-code)
    ```
- Use a HEREDOC for the body.

## 7. Return to main

- `git checkout main`

## 8. Report back

- Print the PR URL so the user can review it.

## Important

- NEVER force-push or amend commits without explicit user request.
- NEVER push directly to main/master — always use a feature branch.
- Only commit the plan file and directly related assets — not unrelated code changes.
- If `gh` is not authenticated, tell the user to run `! gh auth login`.
