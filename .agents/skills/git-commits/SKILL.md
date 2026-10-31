---
name: git-commits
description: Writing git commit messages: Conventional Commits type(scope) format with English header and body, deriving type/scope from the diff and repository structure, atomicity rules and a pre-commit checklist. Project-independent; use when preparing, writing, or reviewing commit messages in any repository.
---

# Git commits (git-commits)

Commit messages are written in English using the Conventional Commits format. The header is written manually from the diff, not copied from command output.

## Header

```
type(scope): short imperative summary
```

- Summary in English imperative, lowercase, no trailing period: `add`, `fix`, `support`, not `added`/`fixes`.
- Length up to 50 characters is the target, 72 is a hard limit (including `type(scope): `).
- Breaking change — `!` after type/scope (`feat!:` or `feat(api)!:`) plus a mandatory `BREAKING CHANGE: ...` line in the footer.

### type — nature of the change

| type | when |
|---|---|
| `feat` | new functionality (CLI command, API endpoint, data format) |
| `fix` | bug fix (crash, incorrect parsing, wrong behavior) |
| `docs` | documentation only |
| `refactor` | code restructuring without behavior change |
| `test` | tests only |
| `perf` | speedup without behavior change |
| `build` | dependency manifests, build tooling |
| `ci` | CI configurations |
| `style` | formatting, whitespace, line breaks only |
| `chore` | maintenance: auxiliary files, scripts, data |
| `revert` | rollback of a previous commit |

### scope — part of the repository by diff paths

Scope is not fixed; it is derived from the structure of the specific repository:

1. Project rules take priority: check AGENTS.md, CONTRIBUTING, neighboring skills — a scope list may be pinned there.
2. No project rules — align with history and reuse scopes already encountered there: `git log --oneline -20` globally, or `git log --oneline -20 -- <path>` scoped to the touched area.
3. No history and no rules — take meaningful parts of the repository from the diff paths: top-level directories, packages, modules, components.

Multiple parts affected — take the dominant one; no dominant part — omit the scope. One commit needing four scopes is a signal the change is not atomic.

## Body

- Answers "what and why", not "how": the "how" is visible from the diff.
- Wrap at ~72 characters, separate paragraphs with a blank line.
- Required when:
  - a contract changes (API, data format, error codes, localization);
  - the storage schema changes or a migration is required;
  - the reason is non-trivial (working around a tool quirk, updating a dependency);
  - multiple parts of the repository are affected.
- Footer (after a blank line): `BREAKING CHANGE: description`, `Closes #N`, `Co-authored-by: Name <email>`.
- Match the repository's trailer conventions (`Co-authored-by` vs `Signed-off-by`, issue refs) — check them against its history before use.

## Core rules

1. **Atomicity** — one meaningful change per commit: code + its tests together; refactoring separate from behavior changes; formatting fixes (`style`) separate from `feat`/`fix`; documentation separate unless it is part of the change.
2. **Stage deliberately**: stage by specific paths, review `git diff --cached` before committing; never `git add -A` blindly.
3. **Never commit**: secrets and keys, temporary and auxiliary files, artifacts covered by `.gitignore`.
4. **Commit only on explicit user request**; do not propose draft messages unprompted.
5. Do not amend/force-push published commits and do not rewrite history without an explicit request; do not reword merge and revert commits — keep git's default revert message (`Revert "<subject>"` header, `This reverts commit <hash>.` in the body).
6. Do not create empty commits; when a hook fails — fix the cause and commit again, never `--no-verify`.

## Workflow

1. `git status`, `git diff`, `git diff --cached`, `git log --oneline -10` (style and context).
2. Choose `type` from the nature of the change and `scope` from the diff paths and repository rules (scope section above), then formulate the header.
3. Decide whether the body is needed (criteria above) and write it if required.
4. Stage the files and commit after passing the checklist below.

## Checklist before `git commit`

- [ ] Project checks passed: build/lint/test commands from AGENTS.md, README, or the CI configuration; if the commands are unknown — ask the user.
- [ ] `git diff --cached` reviewed: the commit contains only what was intended — no secrets, artifacts, or junk.
- [ ] Header: English imperative, ≤ 50 (hard limit 72), no trailing period.
- [ ] `type` and `scope` match the repository's rules (or are derived by the rules above).

Multi-line message — several `-m` flags (portable across shells):

```
git commit -m "feat(importer): accept yaml data files" -m "Map .yaml/.yml to the same section tree as jsonc." -m "Validation and ordered-key semantics are shared with the jsonc reader."
```

Each `-m` flag is a separate paragraph; keep every paragraph within the ~72-character wrap.

## Examples

Good headers:

```
feat(api): add pagination to list endpoint
fix(parser): reject empty config as user error
refactor(core): extract config loading
docs: expand troubleshooting section
chore(deps): bump dependencies
```

A complete message with a breaking change (header, body, footer):

```
feat(api)!: drop legacy id parameters

Replace opaque ids with slugs across public endpoints.

BREAKING CHANGE: clients must migrate to slug-based URLs; the old
ids stop resolving after this change.
```

Bad:

```
fix: fix parser bug              — type duplicates the verb, no scope, "bug" says nothing
update files                     — not imperative, unclear what changed
Feat(Domain): Added parameter.   — capitalized, past tense, trailing period
```

## Special cases

- **Partially staged** (`git add -p`): the message describes only the staged part; the unstaged remainder goes into a separate commit.
- **Coupled edits**: if the project requires changing related places together (code + tests + contract documentation, schema + version, localization keys + bundles) — that is one commit; committing only one side is not allowed.
- **Generated files tied to their source**: generated files that must not be committed separately from their source go together with it, as one change.
- **`fixup`** to a just-made, unpublished commit: acceptable as a separate commit marked `fixup! <header>` (collapse it with `git rebase -i --autosquash` before publishing), or (on user request) as an amend.
