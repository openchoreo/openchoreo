# GitHub Workflow Guide

This document provides a step-by-step guide on how to contribute to this repository.
Following this workflow ensures that contributions remain clean and consistent while staying up to date with the
upstream repository.

## Table of Contents

- [Forking the Repository](#forking-the-repository)
- [Cloning Your Fork](#cloning-your-fork)
- [Configuring Upstream](#configuring-upstream)
- [Syncing with Upstream](#syncing-with-upstream)
- [Creating and Rebasing Feature Branches](#creating-and-rebasing-feature-branches)
- [Resolving Conflicts](#resolving-conflicts)
- [Pushing Changes](#pushing-changes)
- [Before Opening a PR](#before-opening-a-pr)
- [PR Title Convention](#pr-title-convention)
- [DCO Sign-off](#dco-sign-off)
- [Merge Strategy](#merge-strategy)
- [FAQs](#faqs)

---

## Forking the Repository

1. Navigate to the repository on GitHub: [openchoreo/openchoreo](https://github.com/openchoreo/openchoreo).
2. Click the **Fork** button in the top-right corner.
3. This will create a fork under your GitHub account.

## Cloning Your Fork

To work on your fork locally:

```sh
# Replace <your-username> with your GitHub username
git clone https://github.com/<your-username>/openchoreo.git
cd openchoreo
```

## Configuring Upstream

To keep your fork up to date with the original repository:

```sh
# Add the upstream repository
git remote add upstream https://github.com/openchoreo/openchoreo.git

# Verify the remote repositories
git remote -v
```

Expected output:

```
origin    https://github.com/<your-username>/openchoreo.git (fetch)
origin    https://github.com/<your-username>/openchoreo.git (push)
upstream  https://github.com/openchoreo/openchoreo.git (fetch)
upstream  https://github.com/openchoreo/openchoreo.git (push)
```

## Syncing with Upstream

Before starting new work, sync your fork with the upstream repository:

```sh
git fetch upstream
git checkout main
git rebase upstream/main
```

If you have local commits on `main`, you may need to force-push:

```sh
git push -f origin main
```

## Creating and Rebasing Feature Branches

1. Create a new branch for your feature, based on `main`:
    ```sh
    git checkout -b feature-branch upstream/main
    ```

2. Make your changes and commit them.

3. Before opening a pull request, rebase against the latest upstream changes:
    ```sh
    git fetch upstream
    git rebase upstream/main
    ```

## Resolving Conflicts

If you encounter conflicts during rebasing:

1. Git will pause at the conflicting commit. Edit the conflicting files.

2. Stage the resolved files:
    ```sh
    git add <resolved-file>
    ```

3. Continue the rebase:
    ```sh
    git rebase --continue
    ```

4. If needed, repeat the process until rebase completes.

## Pushing Changes

Once rebased, push your changes:

```sh
git push -f origin feature-branch
```

> **Note**: Force-pushing is necessary because rebase rewrites history.

## Before Opening a PR

Before opening a pull request, make sure the following pass locally:

```sh
make lint
make code.gen-check
make test
```

For more details on building and testing, see the [contributing guide](contribute.md).

Open a pull request on GitHub targeting `main` in the upstream repository.

## PR Title Convention

This project follows the [Conventional Commits](https://www.conventionalcommits.org/) specification.
PR titles must follow this format and are validated by the `lint-pr.yml` CI workflow. Since this repository uses
[squash merging](#merge-strategy), the PR title becomes the final commit message on `main`.

### Format

```
<type>(<scope>): <subject>
```

- **type** (required): The kind of change
- **scope** (optional): The area of the codebase
- **subject** (required): A short description starting with a lowercase letter

### Types

| Type       | Purpose                                  |
|------------|------------------------------------------|
| `feat`     | New feature                              |
| `fix`      | Bug fix                                  |
| `docs`     | Documentation only                       |
| `refactor` | Code restructure (no behavior change)    |
| `test`     | Adding or correcting tests               |
| `chore`    | Maintenance tasks                        |
| `ci`       | CI/CD changes                            |
| `build`    | Build system or dependencies             |
| `perf`     | Performance improvement                  |
| `style`    | Code style (formatting, no logic change) |
| `revert`   | Reverting a previous commit              |

### Scopes

| Scope        | Covers                                |
|--------------|---------------------------------------|
| `api`        | `cmd/openchoreo-api`, `api/v1alpha1/` |
| `controller` | `internal/controller/*`               |
| `cli`        | `cmd/choreoctl`, `cmd/occ`            |
| `helm`       | `install/helm/*`                      |
| `deps`       | `go.mod`, dependencies                |

Scope is optional. If a change spans multiple areas, either use the most relevant scope or omit it.

### Examples

```
feat(controller): add ComponentRelease reconciler
fix(api): handle missing organization gracefully
chore(deps): bump sigs.k8s.io/controller-runtime
docs: add DCO sign-off instructions
ci: add CodeQL security scanning workflow
refactor(controller): extract common reconciler logic
```

## DCO Sign-off

All commits must be signed off to certify that you have the right to submit the code under the project's open-source
license. This is enforced by the [DCO](https://developercertificate.org/) check on pull requests.

Add the `-s` flag when committing:

```sh
git commit -s -m "feat(api): add component validation"
```

This appends a `Signed-off-by` line to your commit message:

```text
Signed-off-by: Your Name <your.email@example.com>
```

To sign off all commits automatically, you can set up a git alias:

```sh
git config --add alias.c "commit -s"
```

### Fixing a failed DCO check

If you forgot to sign off your commits, you can amend the last commit:

```sh
git commit --amend -s --no-edit
git push -f origin feature-branch
```

Or sign off all commits in your branch:

```sh
git rebase --signoff upstream/main
git push -f origin feature-branch
```

## Merge Strategy

This repository enforces **squash merging** as the only merge strategy. When a pull request is merged, all commits in
the PR are squashed into a single commit on `main`.

### How it works

- The **PR title** becomes the commit message on `main`, with the PR number automatically appended
  (e.g., `feat(api): add component validation (#1234)`).
- The **individual commit messages** from the PR are preserved in the commit body for reference.
- Contributors can commit freely during development without worrying about keeping a clean commit history — the squash
  merge handles that automatically.

### What this means for contributors

- Focus on writing a clear, descriptive PR title that follows the [PR title convention](#pr-title-convention).
- You do not need to manually squash commits before merging.
- Use as many commits as you need during development (work-in-progress, review feedback, fixups, etc.).
- The `lint-pr.yml` CI workflow validates that PR titles follow the conventional commit format.

---

## FAQs

### Why do we use rebase instead of merge?

Rebasing keeps history linear, making it easier to track changes and avoid unnecessary merge commits. This is especially
useful to keep a clean commit history.

### Can I rebase after opening a pull request?

Yes, but you will need to force-push (`git push -f origin feature-branch`). GitHub will automatically update the pull
request with your new changes.

### How can I undo a rebase?

If something goes wrong during rebasing, you can use:

```sh
git rebase --abort
```

or, if you've already completed the rebase but want to undo it:

```sh
git reset --hard ORIG_HEAD
```

> **Warning**: This will discard changes that were part of the rebase.

Alternatively, you can use git reflog to find the commit before the rebase and reset to it:

```sh
git reflog
```

Identify the commit before the rebase (e.g., HEAD@{3}), then reset to it:

```sh
git reset --hard HEAD@{3}

```

> **Warning**: This will discard changes that were part of the rebase.
