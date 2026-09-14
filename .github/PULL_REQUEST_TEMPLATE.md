---
name: Pull request
about: Open a PR against main
title: ""
labels: ''
assignees: ''
---

## What does this PR do?

A short summary of the change. Why is it needed? Link the tracking issue with
`Fixes #NNN` or `Closes #NNN` if applicable.

## Type of change

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality
      to change)
- [ ] Documentation only
- [ ] Refactor / cleanup

## Modules touched

Which modules / services does this PR affect?

- [ ] Frontend (`apps/dj`)
- [ ] Backend (`api`)
- [ ] Storage / Garage
- [ ] Migrations
- [ ] CI / build
- [ ] Docs only
- [ ] Other: _______

## How was it tested?

Describe the tests you ran and how to reproduce them locally.

- [ ] `pnpm nx lint <project>` passes
- [ ] `pnpm nx typecheck <project>` passes
- [ ] `pnpm nx run-many -t test` passes
- [ ] `pnpm nx e2e dj-e2e` passes (if UI / integration change)
- [ ] Manual smoke test steps: _______

## Screenshots / recordings

If the change is user-visible, attach before/after screenshots or a short clip.

## Checklist

- [ ] My code follows the project's conventions (see [CONTRIBUTING.md](./CONTRIBUTING.md))
- [ ] I have updated the README / docs for any user-facing change
- [ ] I have added or updated tests where applicable
- [ ] No secrets, generated artifacts, or build outputs are included
- [ ] Commit messages follow Conventional Commits
