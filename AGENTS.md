# AGENTS.md

## Project Context

- This repository is a Go CLI built with Cobra.
- The primary executable command is `dev`.
- Prefer small, targeted changes that preserve existing CLI behavior.

## Working Rules

- Read the relevant files before editing and keep changes scoped to the task.
- Use `rg` for search and prefer minimal diffs over broad refactors.
- Preserve user changes already present in the worktree unless the task explicitly requires otherwise.

## Start-of-Turn Workflow

- Before making code changes in a new turn, run `git pull --ff-only`.
- Check `git status --short` before editing.
- Start from a clean worktree whenever possible: no unstaged changes, no staged-but-uncommitted changes, and no leftover local edits from a previous turn.
- If the worktree is not clean, resolve that state before changing code unless the user explicitly asks to work on top of existing local changes.

## Validation

- Run focused verification for the files you changed.
- For CLI behavior changes, prefer `go test ./...` and a direct command check such as `go run ./cmd/dev`.
- At the end of every implementation turn, run `go test ./...` for the full repository.
- Do not finish the turn while `go test ./...` is failing.
- Every new behavior and every changed behavior must be covered by tests.
- Add new tests or update existing tests in the same turn as the logic change.
- Do not treat implementation work as complete if the corresponding tests were not added or adjusted.

## Code Organization

- Prefer struct-oriented design when organizing Go code.
- Give each primary struct its own file instead of mixing multiple primary structs in one file.
- Keep related methods close to the struct they belong to.
- Use predictable file groups such as `foobar.go`, `foobar_internal.go`, and `foobar_test.go` to separate primary logic, internal helpers, and tests.

## Completion Workflow

- End every coding turn with a tested commit.
- After finishing the current slice of work, run `go test ./...` and ensure it passes.
- Stage all repository changes with `git add -A`.
- Create a Conventional Commits commit message automatically based on the actual changes.
- Do not leave completed work uncommitted for a later turn.
- If the broader feature is still incomplete, commit the tested incremental progress and continue in the next iteration.

## Git Commits

- Git commit messages must use Conventional Commits:
  `<type>(<scope>): <subject>`
- Allowed `type` values:
  `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`
- Use a short lowercase `scope` when the changed area is clear; otherwise omit the scope.
- Keep the `subject` concise and specific. Use lowercase where natural.
- Do not use vague subjects such as `update`, `changes`, `misc`, or `fix stuff`.
- Do not end the subject with a period.
- Describe the actual change, not that work happened.

## Commitlint Failures

- If a commit is rejected because the message fails linting, read the exact error output.
- Rewrite the message to satisfy Conventional Commits and retry automatically.
- Repeat until the message passes, unless the failure is unrelated to commit message format.
- Never fall back to a non-conventional commit message.

## Examples

- `feat(cli): show help when running dev`
- `fix(config): handle missing env file`
- `docs(readme): clarify install steps`
