# Contribution Process and Issue Assignment Guide

Welcome, and thank you for your interest in contributing to OpenChoreo! This document describes **how work is
claimed, assigned, and accepted** in the project. It exists to keep contributions fair and predictable — for
first-time contributors, regular contributors, and maintainers alike.

To keep the process fair for everyone, maintainers apply these rules consistently: PRs and assignments that do not
follow them may be closed or released, with a friendly pointer to this document.

## Table of Contents

- [The Golden Rule: Issue First, Assignment First](#the-golden-rule-issue-first-assignment-first)
- [Finding Something to Work On](#finding-something-to-work-on)
- [Label Definitions and Criteria](#label-definitions-and-criteria)
- [Claiming an Issue](#claiming-an-issue)
- [Assignment Limits and Fairness Rules](#assignment-limits-and-fairness-rules)
- [Stale Assignments](#stale-assignments)
- [Pull Request Guidelines](#pull-request-guidelines)
- [Review Process and Timelines](#review-process-and-timelines)
- [Guidelines at a Glance](#guidelines-at-a-glance)

---

## The Golden Rule: Issue First, Assignment First

> **Every non-trivial pull request must reference an open issue that is assigned to you.**

The flow is always:

```text
Issue exists (or you open one) → Issue is triaged/accepted → You are assigned → You open the PR
```

Why this matters:

- It prevents two people from unknowingly working on the same thing.
- It lets maintainers validate the approach **before** you invest time writing code.
- It keeps `good first issue` tasks available for the newcomers they were written for.

**Exceptions** — the following may be sent as a PR directly. For these cases, neither an issue link nor an issue
assignment is required; just explain the change clearly in the PR description:

- Fixing an obvious typo or broken link **as part of a meaningful docs improvement** (see
  [Trivial PRs](#trivial-and-drive-by-prs) below for why we prefer these to be batched)
- CI or build fixes for a breakage that is currently blocking `main`
- Follow-up fixes explicitly requested by a reviewer on your own recently merged PR

If you are unsure whether your change needs an issue, **open the issue first**. A quick confirmation from a
maintainer only takes a moment, and it protects your time — we would hate to see effort go into a large PR that
cannot be accepted.

## Finding Something to Work On

1. **First-time contributors**: pick from
   [`good first issue`](https://github.com/openchoreo/openchoreo/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22+no%3Aassignee).
2. **Returning contributors**: pick from
   [`help wanted`](https://github.com/openchoreo/openchoreo/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22+no%3Aassignee).
3. **Proposing something new**: open an issue (feature/improvement/bug) and wait for triage before writing code.
   Feature requests go through the proposal process described in
   [development-process.md](development-process.md).

If you want to discuss before committing to anything, use the
[Slack channel](https://cloud-native.slack.com/archives/C0ABYRG1MND) or
[GitHub Discussions](https://github.com/openchoreo/openchoreo/discussions).

## Label Definitions and Criteria

### `good first issue`

Items marked with the `good first issue` label are intended for **first-time contributors** (0–2 merged PRs in
OpenChoreo repositories). After successfully completing one or two `good first issue` items, contributors should be
ready to move on to `help wanted` items.

These issues are deliberately kept simple as an on-ramp for new community members. Established contributors are
expected to leave them for newcomers and pick from `help wanted` instead — taking them removes the easiest entry
point into the project. Maintainers may release such assignments back to the pool and suggest a `help wanted` issue
instead.

Maintainers may only apply this label when the issue meets **all** of:

- The task is fully scoped and agreed — no open design questions.
- The solution approach is described in the issue body.
- Relevant code locations and existing tests are pointed out.
- It requires minimal environment setup and no deep OpenChoreo-internals knowledge.
- Background/context links are included.

Maintainers are happy to provide extra guidance and mentorship on these issues — if you get stuck on a CI failure,
finding a reviewer, or a project convention, just ask and we will help you through it.

### `help wanted`

Open to **any** contributor. Criteria for applying the label:

- The task is clear and agreed upon — no further discussion needed before implementation.
- It matters to the project (a maintainer is willing to invest review time in it).
- It is not so urgent that a maintainer needs to do it in the current sprint.

### Priority labels

Assigned by maintainers during triage — see [development-process.md](development-process.md) for the
`Priority/Highest` → `Priority/Low` definitions and the triage cadence.

## Claiming an Issue

1. **Check the assignee field.** If the issue is already assigned, someone is working on it — please pick a
   different one rather than starting work or opening a PR for it. If the assignment looks inactive, see
   [Stale Assignments](#stale-assignments) for how to follow up.
2. **Comment to claim it.** Comment on the issue asking to be assigned (e.g. *"I'd like to work on this"*), optionally
   with a sentence on your intended approach. A maintainer will assign you. If the org has the assignment bot enabled,
   `/assign` on its own line self-assigns.
3. **Wait for the assignment before opening a PR.** This helps us avoid duplicated effort — if a PR is opened for an
   issue that is unassigned or assigned to someone else, we may have to close it, even if the code itself is good.
4. **Ask questions early.** If anything about the issue is unclear, feel free to ask in the issue thread before you
   start coding — sorting out questions up front usually saves a round of rework later.

## Assignment Limits and Fairness Rules

To keep the backlog healthy and shared fairly:

- **`good first issue` items are a starting point.** Once you have completed one or two, we encourage you to move on
  to `help wanted` items — you will be ready for them, and it keeps the on-ramp clear for the next newcomer.
- **Please keep to at most 2 open assignments at a time** (non-maintainer contributors). Wrapping up or releasing one
  before claiming another keeps the backlog flowing for everyone.
- **Claim issues you are ready to start on.** Holding an assignment without activity can unintentionally block others
  who would like to help — see [Stale Assignments](#stale-assignments) for how we handle this.
- **Help rather than duplicate.** If an issue already has an assignee with an open PR, the most valuable contribution
  is reviewing or testing that PR rather than opening a second one.

## Stale Assignments

Life happens, and priorities change — that is completely understandable. We just ask that assignments reflect active
work, so that issues do not stay locked while others are waiting to help.

| Condition | Action |
|---|---|
| **7 days** assigned with no PR and no status comment | Maintainer pings the assignee on the issue asking for an update |
| **3 more days** with no response | Maintainer removes the assignment; the issue returns to the pool |
| Draft/open PR exists but has been inactive **10 days** and the author is unresponsive | Maintainer may unassign and close the PR; anyone may pick the issue up (crediting prior work where reasonable) |

If you know you'll be delayed (exams, work, vacation), just leave a short note on the issue — a one-line status
comment resets the clock, and there is no pressure to explain. If you can no longer work on an issue, feel free to
un-assign yourself or leave a comment so a maintainer can release it. Releasing an issue is completely normal and
appreciated — it simply lets someone else pick it up.

If you come across an issue whose assignment looks stale, please leave a friendly comment asking the assignee or
maintainers for a status update rather than opening a PR directly — a maintainer will then follow the steps above.

## Pull Request Guidelines

### What we look for in every PR

- **Linked, assigned issue**: the description must contain `Fixes #<issue>` (or `Part of #<issue>` for partial work),
  and that issue must be assigned to you. PRs covered by the
  [exceptions](#the-golden-rule-issue-first-assignment-first) are exempt from both the issue link and the
  assignment — a clear PR description is enough.
- **Green locally**: `make lint`, `make code.gen-check`, and `make test` pass before you push (see
  [github_workflow.md](github_workflow.md)).
- **Conventional title** and **DCO sign-off** as described in [github_workflow.md](github_workflow.md).
- **Small and focused**: one issue per PR. Split refactors, drive-by fixes, and generated-code churn into separate PRs.
  Small PRs are reviewed faster and merged sooner.
- **Tests**: code changes come with tests. Very few pull requests can touch the code and not touch tests.
- **AI disclosure**: follow the [AI Policy](AI-POLICY.md). Please make sure you understand every change you submit,
  and respond to review comments yourself.

### PRs we may not be able to accept

To keep the process fair and review time focused, we may have to close the following kinds of PRs:

- A PR for an issue **assigned to someone else**, or for a non-trivial issue that was not assigned to you.
- A PR from an **established contributor against a `good first issue`** — we will kindly ask you to pick a
  `help wanted` issue instead, so the issue can return to the pool for newcomers.
- **Trivial/drive-by PRs** (see below).
- Large machine-generated or repo-wide search/replace changes that were not agreed on in an issue first.
- PRs whose author has not responded to review comments for **21 days** — you are always welcome to reopen or
  resubmit when you have time again.

### Trivial and drive-by PRs

We kindly ask that single-word typo fixes, whitespace-only changes, badge tweaks, and similar minimal edits are not
sent as standalone PRs — each PR carries review and CI cost, and for very small edits that cost outweighs the
benefit. If you spot a typo, we would love the fix as part of a broader improvement to that document, or you can
report it in an issue so several small fixes can be batched together.

## Review Process and Timelines

1. CI runs on your PR — please help us by keeping it green, as reviewers generally wait for passing checks before
   taking a look. If a failure looks unrelated to your change, feel free to mention it on the PR.
2. A maintainer or reviewer will pick it up — typically within **5 working days**. If you have not heard anything
   after that, a friendly ping on the PR or in Slack is very welcome (rather than opening a second PR).
3. Address review comments with new commits — there is no need to squash, as the repository squash-merges (see
   [github_workflow.md](github_workflow.md#merge-strategy)).
4. It is absolutely fine to respectfully push back on feedback with reasoning; if a disagreement cannot be resolved,
   the maintainers listed in [MAINTAINERS.md](../../MAINTAINERS.md) will help make the call.

## Guidelines at a Glance

| Situation | What happens |
|---|---|
| Non-exempt PR without a linked issue assigned to the author | We may close it with a friendly pointer to this guide |
| PR against an issue assigned to someone else | We may close it to avoid duplicated effort |
| `good first issue` taken by an established contributor | We will suggest a `help wanted` issue instead; the issue returns to the pool |
| More than 2 concurrent assignments by a non-maintainer contributor | We may release the newest assignments back to the pool |
| Assigned issue has no PR and no status comment for 7 days, then no response for 3 more days | We will check in first, then release the assignment |
| PR inactive for 10 days and its author unresponsive | We may close the PR and release the issue — you are welcome to resubmit later |
| Standalone trivial/typo PR | We will ask for it to be batched or folded into a larger improvement |

These guidelines are simply about fairness — they help ensure that a first-time contributor in any timezone has the
same opportunity to contribute as someone who follows the repository closely. If you are ever in doubt, we are always
happy to help — just ask on the issue or in [Slack](https://cloud-native.slack.com/archives/C0ABYRG1MND) before you
start writing code. Thank you for contributing to OpenChoreo!
