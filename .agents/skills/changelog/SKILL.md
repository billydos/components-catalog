---
name: changelog
description: Writing CHANGELOG.md version entries: pinning the change range between two explicit commits (never HEAD), mapping Conventional Commit types to sections, aggregating commits into user-visible changes. The entry format is pinned by TEMPLATE.md in this skill's directory.
---

# Changelog (changelog)

A version entry in CHANGELOG.md describes changes between **two explicit commits**. The range is never «commit → HEAD»: HEAD is a moving pointer — tomorrow it includes the changelog commit itself, then the next version's work, and the entry and its analysis stop matching. Both bounds are written down as hashes and stay verifiable at any later moment.

The entry format is pinned by `TEMPLATE.md` in this directory; the language of the entries is Russian (this repository's convention), commit headers are English — translate the meaning, don't copy headers.

## Range: two explicit commits

- **End** — the last content commit of the release, an explicit hash:
  - the user named a commit — use it;
  - not named — resolve the current tip once (`git rev-parse HEAD`), write the resulting hash down, and from then on use only the hash: in `git log` commands and in the range line. The literal `HEAD` does not appear anywhere.
  - the entry being written (its own `docs: add changelog …` commit) is created on top of the end commit and never gets inside any range.
- **Start** — the previous release boundary:
  - the latest existing CHANGELOG.md entry gives it: the hash in «по коммит `X`»;
  - no entries yet — the previous version tag's commit (`git rev-parse <tag>^{commit}`) or the root commit;
  - the previous release's `docs: add changelog …` commit may fall inside the range this way — it is a service commit and is not listed.
- Range: `git log <start-hash>..<end-hash>`. Tag and branch names are not bounds: they drift; a hash does not.

If content commits landed after the entry was written but before tagging — extend the end to the new last content commit and update the range line.

## Section mapping

| Commits in range | Section |
|---|---|
| `feat!`/`fix!` or `BREAKING CHANGE` footer | **Несовместимые изменения** (first; mandatory when present in range) |
| `feat` | **Добавлено** |
| `refactor`, `perf` and behavior-affecting changes | **Изменено** |
| `fix` | **Исправлено** |
| `docs`, `chore`, `ci`, `build`, `test`, `style`, `revert` | **Документация и прочее** |

- Empty sections are removed; section order follows the template.
- `refactor` goes to **Изменено** only when behavior at the boundaries changed (CLI argument handling, REST contract, error texts); purely internal restructuring gets one compact bullet there or goes to **Документация и прочее**.
- Service commits about the changelog itself (`docs: add changelog …`) are never listed.
- Consumers are CLI users, REST clients, embedders of the library, `data/` importers, CI and docs readers — a change visible to any of them is listed.

## Aggregation

- A bullet is a user-visible change, not a commit: several atomic commits implementing one change merge into one bullet.
- A campaign (audit findings, one refactor split into commits) gets a lead-in line («По итогам аудита …:») with sub-bullets, one per fix.
- «Несовместимые изменения» bullets spell out the migration: affected contracts, what was, what became, required steps.
- Identifiers — commands, flags, codes, tables, files — in backticks; wrap long bullets; details go to sub-bullets.

## Workflow

1. Read CHANGELOG.md; take the start hash from the latest entry (or the previous tag/root commit).
2. Fix the end hash (rules above); verify both: `git show -s --format='%h %cs %s' <hash>`.
3. Collect commits: `git log --no-merges --format='%h %s' <start>..<end>`; full bodies when the `BREAKING CHANGE` footer or details matter.
4. Map to sections, aggregate, write the entry from `TEMPLATE.md`.
5. Insert the new section right after `# Changelog`; previous entries stay untouched.

## Checklist before finishing

- [ ] Range is `<hash>..<hash>`: two explicit commits; no `HEAD` and no branch/tag names as bounds — neither in commands nor in the range line.
- [ ] End = last content commit; this entry's own docs commit is outside the range; the previous release's changelog commit is not listed.
- [ ] Every bullet traces to a commit inside the range; hashes and dates are real (`git show -s`).
- [ ] **Несовместимые изменения** section present iff the range has `!` or a `BREAKING CHANGE` footer.
- [ ] Empty sections removed; previous entries untouched; text Russian, identifiers in backticks.

## Examples

Good range line (matches the existing v0.0.2 entry):

```
Изменения с v0.0.1 (коммит `fe9712c`, 2026-10-10) по коммит `df4c746` (2026-10-11).
```

Bad:

```
Изменения с v0.0.2 по HEAD.      — HEAD moves: the entry lies starting tomorrow
git log v0.0.2..HEAD             — same mistake in analysis
git log df4c746..HEAD            — pulls in the docs commit and next-version work
```
