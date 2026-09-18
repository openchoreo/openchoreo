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
exact commit being released, after `build-and-test` has published the
sha-tagged images and Helm charts for that commit. The release tag is only
created when every leg passes.

The gate runs at two points, both keyed to the exact commit:

- **Branch creation** (`action=branch` or `full`) — the gate runs against the
  new `release-vX.Y` tip as soon as the branch is cut.
- **Tagging** (`action=tag` or `full`) — the gate runs against the commit being
  tagged, unless that commit already passed (e.g. it is still the tip cut at
  branch creation), in which case the earlier green gate is reused instead of
  re-run.

Reuse is tracked by an `e2e-gate` commit status, which `e2e-gate.yml` stamps on
its own tested commit whenever every leg passes — regardless of what triggered
that run. So it also picks up a manual pre-flight dispatch or a nightly
schedule run, not just ones run by the orchestrator itself, as long as it
lands on the identical commit. Any new commit on the branch — a fix or a
backported patch — is gated afresh, and only passing gates are recorded, so a
failed gate is never reused.

Reuse also requires the same Helm chart version and Backstage image tag, not
just the same commit: the status description encodes both, and the
orchestrator only reuses a prior gate when that description matches the
versions it just resolved for the current run. This matters because the
Backstage image tag tracks the `backstage-plugins` release branch tip rather
than anything in this repo's history, so it can change between two gate
checks at the same openchoreo commit. When the description doesn't match, the
gate is re-run even though the commit already has a passing status.

The gate shards the suite into five parallel legs, each on its own runner
and k3d cluster:

| Leg         | Scope                                     | Typical | Timeout |
| ----------- | ----------------------------------------- | ------- | ------- |
| tier1       | Core platform (CP + DP)                   | ~10 min | 45 min  |
| tier2       | API, CLI, authz, gateway (CP + DP)        | ~10 min | 45 min  |
| tier3       | Multi-cluster (4 clusters, one per plane) | ~25 min | 90 min  |
| ui          | Playwright Backstage suite (all planes)   | ~15 min | 90 min  |
| quick-start | Quick Start journey (all planes + sample) | ~20 min | 75 min  |

Because the legs run in parallel, the gate costs the wall-clock of the
slowest leg plus overhead — ~30 minutes, not the sum. The full orchestrator
run is longer: it first waits ~15–30 minutes for `build-and-test` at the
release commit before the gate, then tags once every leg passes.

If a leg fails:

1. On gate failure, the gate blocks the release tag and tag-keyed
   publications, but SHA-scoped artifacts published earlier remain available.
   This includes images and Helm charts produced by `build-and-test` for the
   candidate commit. Inspect the failing leg's diagnostics artifacts on the
   workflow run.
2. Fix (or backport the fix to the release branch), wait for
   `build-and-test` on the new commit, and re-run the `Release Orchestrator`
   workflow. Pushing the fix only re-triggers `build-and-test`, not the gate —
   the orchestrator re-dispatch runs the gate against the new commit, which is
   gated afresh (the failed gate is never reused).

The orchestrator exposes a `skip_e2e` input that bypasses the gate. It is
reserved for declared emergencies (e.g. a critical security hotfix where the
fix has been validated out of band) and should be noted on the release issue
when used.

## Default observability module versions

The k3d installers, quick-start, e2e suite, and multi-cluster guide install
observability modules from
[community-modules](https://github.com/openchoreo/community-modules). On
`main`, those modules are pinned to `0.0.0-latest-dev`: the chart and images
that community-modules publishes from its own `main`. A module release therefore
needs no change in this repo. Release branches pin released module versions
instead.

The versions live in the files tracked by
[`hack/pin-observability-modules.sh`](../../hack/pin-observability-modules.sh).
Run it with `--help` to list the charts and files.

- **Minor releases (new branch).** Before cutting the branch, release the
  community-modules versions this release should ship (bump `<module>/VERSION`
  in community-modules). Then pass each version to the `Release Orchestrator`:

  | Input                           | Script flag                       | Chart                                 |
  |---------------------------------|-----------------------------------|---------------------------------------|
  | `logs_opensearch_version`       | `--logs-opensearch-version`       | `observability-logs-opensearch`       |
  | `tracing_opensearch_version`    | `--tracing-opensearch-version`    | `observability-tracing-opensearch`    |
  | `metrics_prometheus_version`    | `--metrics-prometheus-version`    | `observability-metrics-prometheus`    |
  | `events_otel_collector_version` | `--events-otel-collector-version` | `observability-events-otel-collector` |
  | `logs_openobserve_version`      | `--logs-openobserve-version`      | `observability-logs-openobserve`      |
  | `finops_opencost_version`       | `--finops-opencost-version`       | `finops-opencost`                     |

  All six are required whenever the orchestrator creates a release branch,
  and each version must already be published to
  `oci://ghcr.io/openchoreo/helm-charts`. The branch job commits the pins to
  the new branch, so the e2e gate on branch creation tests those exact
  versions.

  `finops-opencost` is the exception: nothing in this repo installs it, so
  there is no location to rewrite. The orchestrator still requires the input
  and checks that the chart is published, then echoes every version into the
  run summary for the docs constants below. `--check` cannot report it.

- **Patch releases (existing branch).** Pins carry over from the branch cut.
  The orchestrator rejects the module version inputs when the branch already
  exists, so change a pin with a PR against `release-vX.Y` before releasing:

  ```sh
  git fetch upstream
  git switch -c pin-observability-modules-vX.Y upstream/release-vX.Y
  # Rewrites every file that pins the module (--help lists them)
  hack/pin-observability-modules.sh --metrics-prometheus-version 0.7.1
  # Fails if anything is unpinned, otherwise prints the pinned versions
  hack/pin-observability-modules.sh --check
  git commit -s -am "chore: pin observability-metrics-prometheus 0.7.1 on release-vX.Y"
  ```

  Release lines cut before the script existed (v1.2 and older) don't have
  it. List every line that pins a module, edit the ones for the module you are
  changing, then commit the same way:

  ```sh
  git grep -n -E '^(export )?(OBSERVABILITY_)?(LOGS_OPENSEARCH|TRACES_OPENSEARCH|METRICS_PROMETHEUS|EVENTS_OTEL_COLLECTOR|LOGS_OPENOBSERVE)_VERSION *\??= *"?[0-9]' -- install make
  git grep -n -A8 -E 'observability-(logs-opensearch|tracing-opensearch|metrics-prometheus|events-otel-collector|logs-openobserve)' -- install make | grep -E -- '--version"? +"?[0-9]'
  ```

- **Docs constants.** The module keys in the versioned docs'
  `_constants.mdx` (`logsOpensearchModule`, `tracingOpensearchModule`,
  `metricsPrometheusModule`, `eventsOtelCollectorModule`) must match the
  release branch's pins. From an openchoreo `main` checkout, print them with
  `hack/pin-observability-modules.sh --check --ref upstream/release-vX.Y`.

  `finOpsOpenCostModule` is not pinned in this repo, so `--check` does not
  print it. Take it from the `finops_opencost_version` the orchestrator run
  recorded in its branch-job summary, or from `finops-opencost/VERSION` in
  community-modules. For a patch release that only needs a newer FinOps
  module, updating this constant in the docs repo is the whole change: there
  is no pin PR against `release-vX.Y` to open.
- **Guards.** `hack/pin-observability-modules.sh --check` fails if a tracked
  location is still unpinned. It runs in `build-and-test` for `release-v*`
  pushes and PRs, which catches backports that carry `0.0.0-latest-dev` over
  from `main`. The orchestrator also runs it against the target commit before
  the gate and tag.
