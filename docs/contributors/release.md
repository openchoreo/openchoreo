# Releasing a new version of OpenChoreo

The release process of OpenChoreo is tracked through a GitHub issue with a
release issue template. Please follow the steps below to create a new release
issue and complete the release process.

1. Go to the [Release Issue creation
   template](https://github.com/openchoreo/openchoreo/issues/new?template=06_release_template.md)
2. Update the issue title to the desired release. Example: `Release: v1.1.0`
3. Replace all `MAJOR`, `MINOR`, `PATCH` placeholders in the checklist with
   the version numbers
4. Complete the prerequisites listed in the issue before triggering any
   workflows
5. Follow the checklist in the issue to complete the release process

## E2E release gate

Every release — a minor release cut from `main` or a patch release cut from a
`release-vX.Y` branch (e.g. after merging backported fixes) — is gated on the
full e2e suite. The `Release Orchestrator` workflow runs the reusable
[`e2e-gate.yml`](../../.github/workflows/e2e-gate.yml) workflow against the
exact commit being tagged, after `build-and-test` has published the
sha-tagged images and Helm charts for that commit. The release tag is only
created when every leg passes.

The gate shards the suite into four parallel legs, each on its own runner
and k3d cluster:

| Leg   | Scope                                   | Typical | Timeout |
|-------|-----------------------------------------|---------|---------|
| tier1 | Core platform (CP + DP)                 | ~10 min | 45 min  |
| tier2 | API, CLI, authz, gateway (CP + DP)      | ~10 min | 45 min  |
| tier3 | Build + observability (all planes)      | ~25 min | 90 min  |
| ui    | Playwright Backstage suite (all planes) | ~15 min | 90 min  |

Because the legs run in parallel, the gate costs the wall-clock of the
slowest leg (~25 minutes), not the sum. Expect the orchestrator run to take
roughly 45–60 minutes end to end: ~15–30 minutes waiting for
`build-and-test` at the release commit, then the slowest e2e leg, then
tagging.

If a leg fails:

1. On gate failure, the gate blocks the release tag and tag-keyed
   publications, but SHA-scoped artifacts published earlier remain available.
   This includes images and Helm charts produced by `build-and-test` for the
   candidate commit. Inspect the failing leg's diagnostics artifacts on the
   workflow run.
2. Fix (or backport the fix to the release branch), wait for
   `build-and-test` on the new commit, and re-run the `Release Orchestrator`
   workflow.

The orchestrator exposes a `skip_e2e` input that bypasses the gate. It is
reserved for declared emergencies (e.g. a critical security hotfix where the
fix has been validated out of band) and should be noted on the release issue
when used.
