# External Compatibility Scan Sources

`external-compatibility-scan.yaml` reads
`.github/external-compatibility-sources.json` and checks a curated list of
external API, webhook, SaaS, model, and CRD/API contract sources.

The scan is intentionally high-signal:

- It posts feed notices matching configured deprecation, sunset, retirement,
  removal, end-of-support, or breaking-change keywords.
- It checks static Kubernetes manifests and Helm templates for API versions that
  are already removed in supported Kubernetes releases.
- It does not create Slack findings from generic page keyword snippets. Static
  documentation pages are either reference-only or checked for specific expected
  values.
- It reports source-health failures as deduped findings so a broken monitored
  source does not silently remove coverage. A single transient failure is
  recorded in state but Slack starts at the second consecutive failure.
- It baselines the first run by recording historical matches without sending a
  Slack message.
- It keeps new findings pending until a Slack POST succeeds. If Slack delivery
  fails or the webhook secret is missing, the same pending findings are retried
  on the next run instead of being marked delivered.
- It splits Slack notices into multiple payloads when more than 10 findings are
  pending, and only marks them delivered after all payloads post successfully.
- It deduplicates future notices using the previous successful run's
  `external-compatibility-scan-state` artifact, with separate observed and
  notified state.
- It does not report routine Dependabot updates, normal Docker image refreshes,
  normal Helm chart bumps, or e2e failures.

## Slack Setup

Create a GitHub Actions secret named
`SLACK_EXTERNAL_COMPATIBILITY_WEBHOOK_URL` with a Slack incoming webhook URL.
Until the secret exists, the workflow still runs and prints any would-be Slack
payload in the job log.

Manual runs support:

- `notify_on_first_run`: send matching historical findings if no prior state is
  available. Keep this disabled for normal use.
- `post_to_slack`: disable Slack posting while testing source/config changes.

## Adding A Source

Add an entry to `.github/external-compatibility-sources.json` with:

- `id`: stable unique identifier.
- `name`: human-readable source name.
- `type`: `feed`, `github_api_versions`, `page_contains`, `page_terms`,
  `reference_page`, or `eol_api`.
- `url`: provider page, RSS feed, or Atom feed.
- `severity`: `critical`, `high`, `medium`, or `low`.
- `owner`: expected OpenChoreo owner area.
- `affected_files`: files or directories that explain why the source matters.
- `action`: what the team should do when a new matching notice appears.
- `keywords`: source-specific feed keywords when the defaults are too broad or
  too narrow.
- `required_terms`: optional source-specific relevance terms. When present for
  a feed, an item must match `keywords` and at least one nearby
  `required_terms` entry before it can produce a Slack finding.
- `required_terms_context_chars`: optional proximity window for feed
  `required_terms`; defaults to 350 characters around the matched keyword.
- `required_text`: for `page_contains`, specific text that must remain present.
- `match_terms`: for `page_terms`, model names, endpoint paths, or provider
  terms to look for near deprecation or breaking-change keywords on a provider
  page.
- `pinned_version`, `supported_until`, and `warn_within_days`: for
  `github_api_versions`, the REST API version OpenChoreo pins and the support
  deadline to warn on.
- `cycle` and `warn_within_days`: for `eol_api`, the lifecycle cycle to monitor
  and the alert window.

Use `header_probes` for providers that announce deprecation or sunset through
HTTP response headers such as `Deprecation`, `Sunset`, or `Warning`.

Use `manifest_scans` for local repository checks that do not need a remote URL.
The current `kubernetes_api_versions` scan checks configured YAML paths for
Kubernetes API versions removed in recent releases. It is a lightweight static
guard; use rendered `helm template` output plus `pluto` or `kubent` when chart
templating makes the static source insufficient.

`reference_page` entries are source-health checks only. They keep important
provider pages visible in the report, but do not generate compatibility
findings from generic keyword matches.

The workflow stores state artifacts for 90 days. If scheduled scans are paused
longer than that and no state artifact remains, the next run establishes a new
baseline unless manually run with `notify_on_first_run`.

Prefer feeds, provider APIs, header probes, and explicit expected-value checks
over generic documentation-page keyword matching.

## Maintenance Notes

Feed `required_terms` intentionally trade recall for precision. A provider
notice must use one of the configured scope terms near a lifecycle or
breaking-change keyword before it can produce a Slack finding. This keeps Slack
high-signal, but it also means coverage can narrow silently if an upstream
provider changes terminology or release-note structure.

Review `required_terms` periodically, especially after adding or changing an
integration. For the most critical compatibility risks, prefer direct checks
such as provider lifecycle APIs, `Deprecation` / `Sunset` response headers, or
local manifest scans instead of relying only on feed wording.

## Validation

Validate config locally without fetching remote sources:

```sh
python3 .github/scripts/external_compatibility_scan.py \
  --config .github/external-compatibility-sources.json \
  --state /tmp/external-compatibility-state.json \
  --state-out /tmp/external-compatibility-state.json \
  --slack-payload /tmp/external-compatibility-slack.json \
  --report /tmp/external-compatibility-report.json \
  --validate-only
```
