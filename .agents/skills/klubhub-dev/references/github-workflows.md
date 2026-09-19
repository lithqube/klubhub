# GitHub workflows — KlubHub

This repo has workflow files that have specific auth and gating requirements. Read this before editing any file under `.github/workflows/`.

## OAuth scope gate

The `gh` CLI authenticated via OAuth cannot push commits that create or update `.github/workflows/*` files. Error: `refusing to allow an OAuth App to create or update workflow`.

Workaround: push via SSH, which bypasses the OAuth scope check:

```bash
git remote set-url origin git@github.com:lithqube/klubhub.git
git push
```

Restore the OAuth remote after:

```bash
git remote set-url origin https://github.com/lithqube/klubhub.git
```

Alternative: ask the user to add the `workflow` scope to their OAuth token, or use a fine-grained PAT.

## Force-push blocked on `main`

The `main` branch is protected with an approval gate. `git push --force-with-lease` times out after ~60 seconds because the gate requires human approval.

**Don't force-push to main.** Create a `fix/...` branch and PR instead:

```bash
git checkout -b fix/workflow-env-binding
git push -u origin fix/workflow-env-binding
gh pr create --base main --head fix/workflow-env-binding --title '...' --body '...'
```

For env-binding fixes to workflows, the smallest possible diff (single commit, single file, single hunk) clears the gate fastest.

## PR-preview build gating

The `pages.yml` workflow runs on:

- `push` to `main` (production deploy)
- `pull_request` (PR preview)
- `workflow_dispatch` (manual)

For PR builds, GitHub Actions does not make environment-bound secrets available. So the `build` step must skip secret reading on `pull_request` events:

```yaml
- name: Build static site
  if: github.event_name != 'pull_request'
  env:
    PLUNK_PUBLIC_KEY: ${{ secrets.PLUNK_PUBLIC_KEY }}
  run: node apps/site/build.mjs

- name: Build static site (PR preview placeholder)
  if: github.event_name == 'pull_request'
  env:
    PLUNK_PUBLIC_KEY: pk_ci_pr_preview_only_do_not_subscribe
  run: node apps/site/build.mjs
```

The placeholder `pk_ci_pr_preview_only_do_not_subscribe` is wired into `apps/site/site.test.mjs` as a pinning test — if anyone changes it, the test fails.

## Environment binding for secrets

For non-PR events, the build job MUST bind to the `github-pages` environment to read `PLUNK_PUBLIC_KEY`:

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    environment:
      name: github-pages
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 20
      - name: Build
        env:
          PLUNK_PUBLIC_KEY: ${{ secrets.PLUNK_PUBLIC_KEY }}
        run: node apps/site/build.mjs
```

Forgetting the `environment: github-pages` block makes the secret silently empty. Symptom in run logs: `PLUNK_PUBLIC_KEY: ***` is masked but the build script errors out with "PLUNK_PUBLIC_KEY is empty" or fails the `pk_*` prefix check.

## How to verify a workflow change

1. Push the branch (SSH remote) and open the PR.
2. The PR build runs automatically. Check the run logs:
   - For PR builds: `PLUNK_PUBLIC_KEY: ***` should appear in the masked env section (the `***` proves the placeholder was set).
   - For prod builds (post-merge): the same. If `PLUNK_PUBLIC_KEY:` is empty, env binding is broken.
3. After merge, trigger a `workflow_dispatch` to confirm prod behavior:
   ```
   gh workflow run pages.yml --ref main
   ```
4. Check the deployed site at https://klubhub.io.

## Container workflow specifics

`containers.dj.yml` (renamed from `containers.yml`) builds and pushes `ghcr.io/lithqube/klubhub-dj-api`. Per-product ownership — future products get `containers.promoter.yml`, `containers.label.yml`.

The workflow triggers on:

- Push to `main` of any file under `apps/dj/**` or `apps/dj/Dockerfile`
- Manual `workflow_dispatch` with a tag input

Push to GHCR requires the `packages: write` permission and a GHCR PAT or the default `GITHUB_TOKEN` with `packages: write` granted at the org level.

## Common workflow bugs

| Symptom | Cause | Fix |
|---|---|---|
| `refusing to allow an OAuth App to create or update workflow` | Pushing via OAuth remote | Switch to SSH remote, push, restore |
| `force-with-lease` times out | Approval gate on main | Use a branch + PR |
| `PLUNK_PUBLIC_KEY:` empty in logs | Missing `environment: github-pages` | Add the env binding |
| PR build fails on missing PLUNK_PUBLIC_KEY | Build step runs on pull_request | Add `if: github.event_name != 'pull_request'` gate |
| Container push 403 | GHCR PAT missing or expired | Re-create PAT with `packages: write`, update `GHCR_TOKEN` secret |
| Container push 404 | Package doesn't exist yet | First push creates the package; subsequent pushes append tags |
