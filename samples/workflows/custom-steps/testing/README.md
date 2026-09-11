# Testing Custom Step — Test Coverage Collection

This sample adds a test step to an OpenChoreo CI workflow and surfaces what it measured into `WorkflowRun` status, using declarative workflow results.

Before results existed, a value a build produced — an image reference, a coverage number — was reachable only by reading pod logs or inspecting the underlying Argo resources. A workflow now declares what a run produces, and the controller records it.

## Files

| File | Description |
|------|-------------|
| `run-tests.yaml` | `ClusterWorkflowTemplate` that runs your test command and emits a summary as step outputs |
| `dockerfile-builder-tests.yaml` | `ClusterWorkflow` that extends `dockerfile-builder` with the test step and declares the results |

## Pipeline

```text
checkout-source → run-tests → build-image → publish-image → generate-workload-cr
```

Tests run against the checked-out source, before anything is built, so a failing test stops the pipeline before an image is produced.

## What lands in status

The `dockerfile-builder-tests` workflow declares six results:

```yaml
spec:
  results:
    - name: image
      valueFrom: { taskResult: { task: publish-image, result: image } }
    - name: git-revision
      valueFrom: { taskResult: { task: checkout-source, result: git-revision } }
    - name: test-report
      valueFrom: { taskResult: { task: run-tests, result: test-report } }
    - name: coverage-percent
      valueFrom: { taskResult: { task: run-tests, result: coverage-percent } }
    - name: tests-verdict
      valueFrom: { taskResult: { task: run-tests, result: tests-outcome } }
    - name: tests-summary
      valueFrom:
        expression: "${tasks['run-tests'].results['tests-total'] == '' ? 'no test counts reported' : tasks['run-tests'].results['tests-passed'] + '/' + tasks['run-tests'].results['tests-total'] + ' passed'}"
```

`tests-verdict` reads `tests-outcome`, which the step derives from the test command's exit
code. It deliberately does **not** derive the verdict from `tests-failed`: with the shipped
defaults no JUnit report is parsed, so every count is empty — and an empty count is not a
pass. `tests-summary` shows the expression form, and reports honestly when no counts exist.

After a run finishes:

```console
$ kubectl get workflowrun my-run -o yaml
status:
  results:
    - name: image
      description: The image reference this build published
      value: registry.example.com/acme-corp-my-project-my-component:v1
    - name: git-revision
      description: The git revision this build checked out
      value: 4f2a19c
    - name: test-report
      description: Summary of the test run, projected into status.testReport
      value: '{"coveragePercent":"87.5","testsTotal":120,...}'
    - name: coverage-percent
      description: Line coverage measured by the test step, as a percentage
      value: "87.5"
    - name: tests-verdict
      description: Whether the test command succeeded
      value: failed
    - name: tests-summary
      description: Test counts, when the step produced a report
      value: 118/120 passed
  testReport:
    coveragePercent: "87.5"
    testsTotal: 120
    testsPassed: 118
    testsFailed: 1
    testsSkipped: 1
    testDurationSeconds: "42.500"
    reportFormat: cobertura
```

`occ workflowrun get <name>` renders the same thing.

### The `test-report` result is special

`test-report` is a **reserved result name**. When a workflow declares it, the controller parses its value as JSON and additionally projects it into the typed `status.testReport`, so a consumer reads `status.testReport.coveragePercent` as a field instead of unpacking a JSON string. The raw value stays in `status.results` either way.

If the value does not parse, the run is unaffected: `status.results` still holds the raw value, `status.testReport` is left unset, and a `TestReportInvalid` event is recorded on the `WorkflowRun`.

The controller also rejects a report that parses but that the API server would refuse — a
`coveragePercent` of `"187.5"`, a `testDurationSeconds` of `"42s"`, a negative count. This
matters more than it looks: the test report is written by the same status update that records
the run's terminal condition, so a report the API server rejects would fail that write and
leave the run stuck short of completion. Dropping the report keeps a miscalibrated test step
from wedging the build.

## Two ways to produce the summary

### 1. Let the step parse your reports (default)

Point `testing.junitPath` and `testing.coveragePath` at what your test command leaves behind:

| Report | Format | How it is read |
|--------|--------|----------------|
| `junitPath` | JUnit XML | Test counts and duration summed across every `<testsuite>` element |
| `coveragePath` ending in `.xml` | Cobertura | The root `<coverage>` element's `line-rate`, as a percentage |
| `coveragePath` otherwise | Plain text | The file's contents read as a bare percentage, e.g. `87.5` |

A coverage value that is not a percentage between 0 and 100 is discarded with a note in the
step's logs, rather than emitted. The whole file is stripped of spaces and `%`, so pointing
`coveragePath` at raw `go tool cover -func` output (`total: (statements) 87.5%`) yields
nonsense — extract the number, as the default command does.

The default parameters do this for Go:

```yaml
testing:
  image: golang:1.24-alpine
  command: "go test ./... -coverprofile=/tmp/cover.out && go tool cover -func=/tmp/cover.out | tail -1 | awk '{print $3}' > coverage.txt"
  coveragePath: coverage.txt
```

### 2. Write the summary yourself

When the built-in parsing does not fit your toolchain, have the test command write the JSON summary directly and point `testing.summaryPath` at it. When that file exists it is used verbatim and nothing else is parsed:

```yaml
testing:
  image: node:20-alpine
  command: "npm test -- --json --outputFile=/tmp/jest.json; node scripts/to-openchoreo-summary.js > .openchoreo/test-summary.json"
  summaryPath: .openchoreo/test-summary.json
```

The file must be a single JSON object using only these fields — an unknown field is rejected rather than silently dropped, so a typo is reported instead of losing the number:

```json
{
  "coveragePercent": "87.5",
  "testsTotal": 120,
  "testsPassed": 118,
  "testsFailed": 1,
  "testsSkipped": 1,
  "testDurationSeconds": "42.5",
  "reportFormat": "cobertura",
  "reportArtifact": ""
}
```

Every field is optional. `coveragePercent` and `testDurationSeconds` are decimal **strings**, not numbers, so a value like `87.5` round-trips through the API server exactly.

## Parameters

`dockerfile-builder-tests` accepts everything `dockerfile-builder` does, plus a `testing` block:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `testing.image` | `golang:1.24-alpine` | Image the tests run in. Must contain the toolchain your command needs — the step ships no runtime of its own. |
| `testing.command` | Go test + coverage | Shell command that runs the tests, executed from the repository root |
| `testing.summaryPath` | `""` | Path to a JSON summary the command writes itself. When set and present, it is used verbatim. |
| `testing.junitPath` | `""` | Path to a JUnit XML report, parsed for test counts |
| `testing.coveragePath` | `coverage.txt` | Path to a coverage report |
| `testing.reportFormat` | `""` | Name of the format produced, recorded in `status.testReport.reportFormat` |
| `testing.failOnTestFailure` | `"true"` | Whether a failing test fails the build — see below |

### `failOnTestFailure`

The summary reaches `status.results` either way. Argo records a step's output parameters even when its container exits non-zero, and the controller resolves results on failed runs as well as successful ones — both verified on a real run, where a step exiting 7 still produced a complete `status.testReport`. So this parameter only decides whether the build goes red:

- **`"true"` (default)** — a failing test fails the build. The `WorkflowRun` ends `Failed` and its status still carries the coverage and counts. This is what most teams want.
- **`"false"`** — the step succeeds regardless, so the pipeline continues to the build steps and a consumer gates on `status.testReport.testsFailed` instead. Choose this when you want an image built from a commit whose tests fail, or when coverage is enforced elsewhere.

## Size caps

Results land in `status`, which every watcher of the object reads on every change, so both a single value and the run's total are capped. Over-cap values are truncated and marked `truncated: true`, with an event recorded; once the run's total is reached, remaining results are not recorded at all, so the ones that did fit stay whole.

The defaults are 4 KiB per value and 32 KiB per run. An operator can change them with the controller's `--workflowrun-result-max-bytes` and `--workflowrun-results-max-bytes` flags.

## What is not stored

The **full** coverage report is not in `status` and is not stored as an artifact. The workflow plane configures no Argo artifact repository, so there is nowhere to put a multi-megabyte file today. `status.testReport.reportArtifact` exists in the schema from day one and is empty in every shipped configuration, so adding an artifact repository later is a workflow-plane change rather than a schema change.

Coverage **history** is also out of scope: all four shipped builders set `ttlAfterCompletion: "1d"`, so anything in `status` is deleted within a day. Treat `status.testReport` as a per-run value, not a trend.

## Applying the Samples

### Step 1 — Apply the ClusterWorkflowTemplate

The `ClusterWorkflowTemplate` defines the reusable test step and must be applied before the `ClusterWorkflow` that references it:

```bash
kubectl apply -f run-tests.yaml
```

Verify it was created:

```bash
kubectl get clusterworkflowtemplate run-tests
```

### Step 2 — Apply the ClusterWorkflow

```bash
kubectl apply -f dockerfile-builder-tests.yaml
```

Verify it was created:

```bash
kubectl get clusterworkflow dockerfile-builder-tests
```

### Step 3 — Add the Workflow to the Allowed List for your Component Type

Add `dockerfile-builder-tests` to the `allowedWorkflows` of the `ComponentType` or `ClusterComponentType` your component uses, the same way you would for any other custom workflow.

### Step 4 — Run a build and read the results

```bash
occ workflowrun get <run-name> --namespace <namespace>
```

If a result you declared does not appear, the controller says why in an event rather than failing the run:

```bash
kubectl describe workflowrun <run-name>
```

Look for `ResultExtractionFailed` (the named task or output does not exist), `ResultTruncated`, `ResultsBudgetExceeded`, or `TestReportInvalid`.

## Adding results to your own workflow

Results are not specific to testing. Any `Workflow` or `ClusterWorkflow` can declare them:

```yaml
spec:
  results:
    # Read one output of one step. The task name is the step name as it appears in
    # status.tasks[].name.
    - name: image
      description: "The image reference this build published"
      valueFrom:
        taskResult:
          task: publish-image
          result: image

    # Or compute one, on the same CEL engine as the rest of the spec.
    - name: image-digest
      valueFrom:
        expression: "${tasks['publish-image'].results['image'].split('@')[1]}"

    # Record that a value was produced without writing it into status.
    - name: db-password
      sensitive: true
      valueFrom:
        taskResult:
          task: provision
          result: password
```

Expressions can read `tasks`, `parameters`, `metadata`, and `results` declared earlier in the list. A value that is not a string is recorded as its JSON encoding, so an expression may assemble an object.

`sensitive: true` records the entry without its value. It is not a way to move a secret out of a workflow: there is no `valueFrom` source that reads a Secret, and marking a result sensitive does not make an already-printed value private — whatever a step wrote to its logs stays in its logs.
