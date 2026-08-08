---
name: commit-changes
description: Safely create a Git commit after Codex completes user-authorized code or file changes. Use at the end of implementation, fix, refactor, documentation, configuration, or other repository-editing tasks when the work is ready and the user has not said to avoid commits. Also use when the user explicitly asks to commit, checkpoint, or save changes in Git.
---

# Commit Changes

Create a focused, reviewable commit for completed work. Treat the installed skill as permission to commit successfully verified task changes without asking again; never treat it as permission to push or alter history.

## Workflow

1. Confirm the working directory is inside a Git repository with `git rev-parse --show-toplevel`. If it is not, report that no commit was created. Do not initialize a repository unless the user explicitly requests it.
2. Read applicable repository instructions such as `AGENTS.md` before staging.
3. Inspect `git status --short`, `git diff`, `git diff --cached`, and recent commit subjects. Identify exactly which paths belong to the current task.
4. Preserve unrelated, pre-existing, and user-authored changes. Never discard, restore, overwrite, or include them merely to obtain a clean tree.
5. Run the relevant formatter, tests, linter, build, or validation required by the repository and task. Do not create an automatic commit when relevant verification fails; explain the failure and leave the changes uncommitted unless the user explicitly directs otherwise.
6. Review the proposed diff for accidental secrets, credentials, private keys, tokens, large generated artifacts, debug output, and unrelated edits. Stop and report any concern instead of committing it.
7. Stage task paths explicitly with `git add -- <path>...`. Avoid `git add .`, `git add -A`, broad globs, and interactive staging. Include necessary generated files or lockfiles only when they are part of the task.
8. Reinspect the staged patch with `git diff --cached --stat` and `git diff --cached`. If nothing is staged, do not create an empty commit unless explicitly requested.
9. Create one commit with a concise imperative subject that describes the outcome and follows the repository's recent convention. Add a body only when it explains important reasoning or tradeoffs. Do not amend, squash, rebase, force, or bypass hooks unless explicitly requested.
10. If a commit hook fails or modifies files, inspect the result, rerun relevant checks, and retry only when the resulting patch still belongs to the task.
11. Verify the result with `git status --short` and `git log -1 --oneline`. Report the commit hash and subject, plus any remaining uncommitted paths.
12. Add a commit description that "Codex submitted this"

## Boundaries

- Commit only changes within the user's authorized task scope.
- Never push to a remote unless the user explicitly asks.
- Never commit known failing work automatically.
- Never expose secrets through command output, commit messages, or the final response.
- Never claim success unless `git commit` completed and the new commit is visible in `git log`.
- Respect any repository rule requiring approval before implementation or committing.
