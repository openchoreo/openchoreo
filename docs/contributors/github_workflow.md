# GitHub Workflow Guide

This document provides a step-by-step guide on how to contribute to this repository. 
Following this workflow ensures that contributions remain clean and consistent while staying up to date with the upstream repository.

## Table of Contents
- [Forking the Repository](#forking-the-repository)
- [Cloning Your Fork](#cloning-your-fork)
- [Configuring Upstream](#configuring-upstream)
- [Syncing with Upstream](#syncing-with-upstream)
- [Creating and Rebasing Feature Branches](#creating-and-rebasing-feature-branches)
- [Resolving Conflicts](#resolving-conflicts)
- [Pushing Changes](#pushing-changes)
- [Squashing Commits to Meaningful Milestones](#squashing-commits-to-meaningful-milestones)
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
$ git clone https://github.com/<your-username>/openchoreo.git
$ cd openchoreo
```

## Configuring Upstream
To keep your fork up to date with the original repository:
```sh
# Add the upstream repository
$ git remote add upstream https://github.com/openchoreo/openchoreo.git

# Verify the remote repositories
$ git remote -v
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
$ git fetch upstream
$ git checkout main
$ git rebase upstream/main
```

If you have local commits on `main`, you may need to force-push:

```sh
$ git push -f origin main
```

## Creating and Rebasing Feature Branches

1. Create a new branch for your feature, based on `main`:
    ```sh
    $ git checkout -b feature-branch upstream/main
    ```

2. Make your changes and commit them.

3. Before opening a pull request, rebase against the latest upstream changes:
    ```sh
    $ git fetch upstream
    $ git rebase upstream/main
    ```

## Resolving Conflicts

If you encounter conflicts during rebasing:

1. Git will pause at the conflicting commit. Edit the conflicting files.

2. Stage the resolved files:
    ```sh
    $ git add <resolved-file>
    ```

3. Continue the rebase:
    ```sh
    $ git rebase --continue
    ```

4. If needed, repeat the process until rebase completes.

## Pushing Changes
Once rebased, push your changes:
```sh
$ git push -f origin feature-branch
```
> **Note**: Force-pushing is necessary because rebase rewrites history.

Open a pull request on GitHub targeting `main` in the upstream repository.

## Squashing Commits to Meaningful Milestones

After a review, ensure your PR is ready by squashing unnecessary commits. The commits remaining in your branch should reflect meaningful milestones.
Though this is not a strict requirement, it is recommended to keep the commit history clean and consistent.

Examples of unnecessary commits:
- Multiple review feedbacks
- Minor typo corrections
- Merge and rebase commits
- Work-in-progress commits

To squash commits interactively:
```sh
# Replace N with the number of commits to squash
$ git rebase -i HEAD~N
```

In the interactive rebase menu:
- Change `pick` to `squash` (or `s`) for commits that should be merged into the previous commit.
- Save and close the editor.
- Update the commit message as needed and save again.

Example:

Before squashing:
```
pick 1a2b3c4 Add new events to the deployment controller
pick 5d6e7fa Fix typo in a variable name
pick 4b3c2a3 Address review comments
pick b0475dd Improve conditionas in the deployment controller

# Rebase 6a34ff9..b0475dd onto 6a34ff9 (4 commands)
#
# Commands:
# p, pick = use commit
# r, reword = use commit, but edit the commit message
# e, edit = use commit, but stop for amending
# s, squash = use commit, but meld into previous commit
# f, fixup = like "squash", but discard this commit's log message
```

After squashing:
```
pick 1a2b3c4 Add new events to the deployment controller
squash 5d6e7fa Fix typo in a variable name
squash 4b3c2a3 Address review comments
pick b0475dd Improve conditionas in the deployment controller
```

Finally, push the squashed commit (force push required):
```sh
$ git push --force origin feature-branch
```

---

## FAQs

### Why do we use rebase instead of merge?
Rebasing keeps history linear, making it easier to track changes and avoid unnecessary merge commits. This is especially useful to keep a clean commit history.

### Can I rebase after opening a pull request?
Yes, but you will need to force-push (`git push -f origin feature-branch`). GitHub will automatically update the pull request with your new changes.

### How can I undo a rebase?
If something goes wrong during rebasing, you can use:
```sh
$ git rebase --abort
```
or, if you've already completed the rebase but want to undo it:
```sh
$ git reset --hard ORIG_HEAD
```
> **Warning**: This will discard changes that were part of the rebase.

Alternatively, you can use git reflog to find the commit before the rebase and reset to it:
```sh
$ git reflog
```
Identify the commit before the rebase (e.g., HEAD@{3}), then reset to it:
```sh
$ git reset --hard HEAD@{3}
```
> **Warning**: This will discard changes that were part of the rebase.
