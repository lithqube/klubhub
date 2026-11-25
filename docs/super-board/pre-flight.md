# super-board pre-flight

Written by `super-board lint` on 2026-11-24 for the Ready cards #52, #63 and #69. Every unchecked item stops `super-board run`.

## Credentials

- [x] `gh` signed in with `repo`, `project` and `workflow` scopes (board moves, PRs, workflow edits in #52)
- [x] No app secrets needed: none of the three cards calls an external service from a developer machine

## Tools

- [x] pnpm 12.21.0, Node 26 locally (CI uses Node 22), Go 1.27 (`api/go.mod` asks for 1.26.11)
- [x] Docker running (#52 builds the DJ image for `linux/amd64` and `linux/arm64`)
- [x] `actionlint` installed (#52 edits a workflow)
- [x] Helper skills resolve for `claude` (`super-board-setup.py check` exits 0)
- [ ] Playwright Chromium revision 1243 (only 1208 is installed) → `pnpm exec playwright install chromium`

## Repository

- [x] All five `verify_commands` passed locally on 2026-11-24 (lint, both typechecks, unit tests, `api:test`)
- [ ] The two added helper skills (`.claude/skills/verification-before-completion`, `.claude/skills/testing-strategy`) and `skills-lock.json` are on `main`, so lane worktrees can see them

## After a card merges (your steps)

- #52: check the new `main` image tag lists `linux/amd64` and `linux/arm64`
