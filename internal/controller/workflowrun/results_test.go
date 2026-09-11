// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	workflowpipeline "github.com/openchoreo/openchoreo/internal/pipeline/workflow"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func anyString(s string) *argoproj.AnyString {
	v := argoproj.AnyString(s)
	return &v
}

// podNode builds an Argo Pod node with the given display name and output parameters.
func podNode(nodeName, displayName string, phase argoproj.NodePhase, params map[string]string) argoproj.NodeStatus {
	node := argoproj.NodeStatus{
		Name:        nodeName,
		DisplayName: displayName,
		Type:        argoproj.NodeTypePod,
		Phase:       phase,
	}
	if params != nil {
		outputs := &argoproj.Outputs{}
		// Sorted insertion keeps the fixtures deterministic to read; the extractor does not
		// depend on order.
		for _, name := range sortedParamNames(params) {
			outputs.Parameters = append(outputs.Parameters, argoproj.Parameter{
				Name:  name,
				Value: anyString(params[name]),
			})
		}
		node.Outputs = outputs
	}
	return node
}

func sortedParamNames(params map[string]string) []string {
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j-1] > names[j]; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}
	return names
}

func argoRun(nodes ...argoproj.NodeStatus) *argoproj.Workflow {
	run := &argoproj.Workflow{}
	run.Status.Nodes = argoproj.Nodes{}
	for _, node := range nodes {
		run.Status.Nodes[node.Name] = node
	}
	return run
}

func testRun(t *testing.T, parameters string) *openchoreodevv1alpha1.WorkflowRun {
	t.Helper()
	run := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "run-1",
			Namespace: "ns-1",
			Labels:    map[string]string{"openchoreo.dev/component": "svc"},
		},
	}
	if parameters != "" {
		run.Spec.Workflow.Parameters = &runtime.RawExtension{Raw: []byte(parameters)}
	}
	return run
}

// testWorkflow returns a Workflow carrying the declarations under test. resolveResults now
// builds its CEL context through the pipeline, so the parameters an expression sees are the
// developer's values with schema defaults applied - the same ones a runTemplate sees.
func testWorkflow(results ...openchoreodevv1alpha1.WorkflowResult) *openchoreodevv1alpha1.Workflow {
	return &openchoreodevv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf", Namespace: "ns-1"},
		Spec: openchoreodevv1alpha1.WorkflowSpec{
			RunTemplate: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow"}`)},
			Results:     results,
		},
	}
}

// newResultsReconciler returns a reconciler wired only for result resolution, with a
// recorder whose events the caller can assert on.
func newResultsReconciler(limits ResultLimits) (*Reconciler, *record.FakeRecorder) {
	recorder := record.NewFakeRecorder(32)
	return &Reconciler{
		Recorder:     recorder,
		ResultLimits: limits,
		Pipeline:     workflowpipeline.NewPipeline(),
	}, recorder
}

func drainEvents(recorder *record.FakeRecorder) []string {
	var events []string
	for {
		select {
		case e := <-recorder.Events:
			events = append(events, e)
		default:
			return events
		}
	}
}

func eventsMentioning(events []string, reason string) int {
	n := 0
	for _, e := range events {
		if strings.Contains(e, reason) {
			n++
		}
	}
	return n
}

func resultValue(t *testing.T, results []openchoreodevv1alpha1.WorkflowRunResult, name string) openchoreodevv1alpha1.WorkflowRunResult {
	t.Helper()
	for _, r := range results {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("result %q not recorded; got %+v", name, results)
	return openchoreodevv1alpha1.WorkflowRunResult{}
}

func taskResultDecl(name, task, result string) openchoreodevv1alpha1.WorkflowResult {
	return openchoreodevv1alpha1.WorkflowResult{
		Name: name,
		ValueFrom: openchoreodevv1alpha1.WorkflowResultSource{
			TaskResult: &openchoreodevv1alpha1.TaskResultRef{Task: task, Result: result},
		},
	}
}

func expressionDecl(name, expression string) openchoreodevv1alpha1.WorkflowResult {
	return openchoreodevv1alpha1.WorkflowResult{
		Name: name,
		ValueFrom: openchoreodevv1alpha1.WorkflowResultSource{
			Expression: expression,
		},
	}
}

// ---------------------------------------------------------------------------
// task name normalization
//
// This is the pin the acceptance criteria asks for: status.tasks[].name and the task a
// result names must resolve through one helper, or a task an operator can see is not one
// they can reference.
// ---------------------------------------------------------------------------

func TestArgoTaskNameMatchesStatusTasks(t *testing.T) {
	nodes := []argoproj.NodeStatus{
		// DisplayName set - the normal case for a templateRef step.
		podNode("build[0].checkout-source", "checkout-source", argoproj.NodeSucceeded,
			map[string]string{"git-revision": "abc123"}),
		// DisplayName empty - the name has to come out of the node name instead.
		podNode("build[1].publish-image", "", argoproj.NodeSucceeded,
			map[string]string{"image": "registry/app:v1"}),
	}

	tasks := extractArgoTasksFromWorkflowNodes(argoRun(nodes...).Status.Nodes)
	outputs := newArgoResultExtractor(argoRun(nodes...)).Outputs()

	if len(tasks) != len(nodes) {
		t.Fatalf("expected %d tasks, got %d", len(nodes), len(tasks))
	}
	for _, task := range tasks {
		if _, ok := outputs[task.Name]; !ok {
			t.Errorf("task %q appears in status but has no outputs keyed under that name; keys: %v",
				task.Name, outputs)
		}
	}
	if len(outputs) != len(tasks) {
		t.Errorf("extractor produced %d task keys for %d status tasks: %v", len(outputs), len(tasks), outputs)
	}
}

func TestArgoTaskNamePrefersDisplayName(t *testing.T) {
	node := podNode("build[0].step-name", "display-name", argoproj.NodeSucceeded, nil)
	if got := argoTaskName(node); got != "display-name" {
		t.Errorf("argoTaskName = %q, want display-name", got)
	}
	node.DisplayName = ""
	if got := argoTaskName(node); got != "step-name" {
		t.Errorf("argoTaskName with empty DisplayName = %q, want step-name", got)
	}
}

// ---------------------------------------------------------------------------
// argo extraction
// ---------------------------------------------------------------------------

func TestArgoExtractorIgnoresNonPodNodes(t *testing.T) {
	run := argoRun(podNode("build[0].tests", "tests", argoproj.NodeSucceeded, map[string]string{"coverage": "91"}))
	// A StepGroup carries the same outputs; keying them too would invent a task name that
	// never appears in status.tasks.
	stepGroup := podNode("build[0]", "build[0]", argoproj.NodeSucceeded, map[string]string{"coverage": "91"})
	stepGroup.Type = argoproj.NodeTypeStepGroup
	run.Status.Nodes[stepGroup.Name] = stepGroup

	outputs := newArgoResultExtractor(run).Outputs()
	if len(outputs) != 1 {
		t.Fatalf("expected only the Pod node's outputs, got %v", outputs)
	}
	if outputs["tests"]["coverage"] != "91" {
		t.Errorf("unexpected outputs: %v", outputs)
	}
}

func TestArgoExtractorEmptyRun(t *testing.T) {
	if got := newArgoResultExtractor(nil).Outputs(); got != nil {
		t.Errorf("nil run should produce no outputs, got %v", got)
	}
	if got := newArgoResultExtractor(&argoproj.Workflow{}).Outputs(); got != nil {
		t.Errorf("run with no nodes should produce no outputs, got %v", got)
	}
	if got := newArgoResultExtractor(&argoproj.Workflow{}).Phases(); got != nil {
		t.Errorf("run with no nodes should produce no phases, got %v", got)
	}
}

func TestArgoExtractorSkipsValuelessParameters(t *testing.T) {
	node := podNode("build[0].tests", "tests", argoproj.NodeSucceeded, nil)
	node.Outputs = &argoproj.Outputs{
		Parameters: []argoproj.Parameter{
			{Name: "never-set"},                          // no Value
			{Name: "", Value: anyString("orphan")},       // no Name
			{Name: "coverage", Value: anyString("91.5")}, //nolint:godot // fixture
		},
	}
	outputs := newArgoResultExtractor(argoRun(node)).Outputs()
	if len(outputs["tests"]) != 1 || outputs["tests"]["coverage"] != "91.5" {
		t.Errorf("expected only the fully specified parameter, got %v", outputs["tests"])
	}
}

func TestArgoExtractorReportsPhases(t *testing.T) {
	run := argoRun(
		podNode("build[0].checkout", "checkout", argoproj.NodeSucceeded, map[string]string{"rev": "abc"}),
		podNode("build[1].tests", "tests", argoproj.NodeFailed, nil),
	)
	phases := newArgoResultExtractor(run).Phases()
	if phases["checkout"] != string(argoproj.NodeSucceeded) || phases["tests"] != string(argoproj.NodeFailed) {
		t.Errorf("unexpected phases: %v", phases)
	}
}

// ---------------------------------------------------------------------------
// retried steps
//
// Status.Nodes is a map, so anything that picks a node by iteration order is
// nondeterministic. These run many times: a single pass can pass by luck.
// ---------------------------------------------------------------------------

// retryRun builds a run whose step was retried `attempts` times. Every attempt shares one
// FinishedAt, which is what Argo actually produces for fast failures: metav1.Time serializes
// at second granularity, so attempts that fail quickly all carry the same timestamp.
//
// Only the last child succeeded, and each attempt carries a different value.
func retryRun(attempts int) *argoproj.Workflow {
	finished := metav1.Now()
	nodes := make([]argoproj.NodeStatus, 0, attempts)
	children := make([]string, 0, attempts)

	for i := 0; i < attempts; i++ {
		phase := argoproj.NodeFailed
		if i == attempts-1 {
			phase = argoproj.NodeSucceeded
		}
		node := podNode(
			fmt.Sprintf("build[0].deploy(%d)", i),
			// Argo suffixes each attempt's displayName with its index; the bare name lives
			// on the Retry parent. Confirmed on a live cluster.
			fmt.Sprintf("deploy(%d)", i),
			phase,
			map[string]string{"image": fmt.Sprintf("registry.example/app:attempt-%d", i)},
		)
		node.ID = fmt.Sprintf("attempt-%d", i)
		node.FinishedAt = finished
		nodes = append(nodes, node)
		children = append(children, node.ID)
	}

	run := argoRun(nodes...)
	retry := argoproj.NodeStatus{
		Name:        "build[0].deploy",
		DisplayName: "deploy",
		Type:        argoproj.NodeTypeRetry,
		Phase:       argoproj.NodeSucceeded,
		ID:          "retry-node",
		Children:    children,
	}
	run.Status.Nodes[retry.Name] = retry
	return run
}

func TestArgoExtractorUsesTheFinalRetryAttempt(t *testing.T) {
	// Eleven attempts, so the last one is "deploy(10)". With equal timestamps a name-based
	// tie-break would pick "deploy(9)" - lexicographically the largest - which is a failed
	// attempt. Only resolving the Retry node's last child gets this right, so this case
	// fails if the retry handling is removed.
	const attempts = 11
	want := fmt.Sprintf("registry.example/app:attempt-%d", attempts-1)

	for i := 0; i < 50; i++ {
		extractor := newArgoResultExtractor(retryRun(attempts))

		if got := extractor.Outputs()["deploy"]["image"]; got != want {
			t.Fatalf("iteration %d: outputs came from a superseded attempt: %q, want %q", i, got, want)
		}
		if got := extractor.Phases()["deploy"]; got != string(argoproj.NodeSucceeded) {
			t.Fatalf("iteration %d: phase came from a superseded attempt: %q", i, got)
		}
	}
}

func TestStatusTasksDropSupersededRetryAttempts(t *testing.T) {
	// status.tasks and the result extractor must agree, or a task an operator sees is not
	// one they can name in a result.
	for i := 0; i < 50; i++ {
		run := retryRun(11)
		tasks := extractArgoTasksFromWorkflowNodes(run.Status.Nodes)
		outputs := newArgoResultExtractor(run).Outputs()

		if len(tasks) != 1 {
			t.Fatalf("iteration %d: a retried step should appear once, got %d: %+v", i, len(tasks), tasks)
		}
		if tasks[0].Phase != string(argoproj.NodeSucceeded) {
			t.Fatalf("iteration %d: status reported a superseded attempt: %+v", i, tasks[0])
		}
		if _, ok := outputs[tasks[0].Name]; !ok {
			t.Fatalf("iteration %d: task %q has no outputs keyed under that name", i, tasks[0].Name)
		}
	}
}

func TestArgoExtractorIsDeterministicForDuplicateTaskNames(t *testing.T) {
	// Two Pod nodes with no Retry parent - the same template used twice. There is no
	// "correct" answer, but there must be a stable one.
	earlier := podNode("build[0].scan", "scan", argoproj.NodeSucceeded, map[string]string{"report": "first"})
	earlier.ID = "a"
	earlier.FinishedAt = metav1.NewTime(metav1.Now().Add(-time.Hour))

	later := podNode("build[1].scan", "scan", argoproj.NodeSucceeded, map[string]string{"report": "second"})
	later.ID = "b"
	later.FinishedAt = metav1.Now()

	for i := 0; i < 50; i++ {
		got := newArgoResultExtractor(argoRun(earlier, later)).Outputs()["scan"]["report"]
		if got != "second" {
			t.Fatalf("iteration %d: expected the later node to win, got %q", i, got)
		}
	}
}

// ---------------------------------------------------------------------------
// taskResult resolution
// ---------------------------------------------------------------------------

func TestResolveResultsFromTaskResult(t *testing.T) {
	r, recorder := newResultsReconciler(ResultLimits{})
	run := testRun(t, "")

	decls := []openchoreodevv1alpha1.WorkflowResult{
		func() openchoreodevv1alpha1.WorkflowResult {
			d := taskResultDecl("image", "publish-image", "image")
			d.Description = "The published image reference"
			return d
		}(),
		taskResultDecl("git-revision", "checkout-source", "git-revision"),
	}

	extractor := newArgoResultExtractor(argoRun(
		podNode("build[0].checkout-source", "checkout-source", argoproj.NodeSucceeded,
			map[string]string{"git-revision": "abc123"}),
		podNode("build[1].publish-image", "publish-image", argoproj.NodeSucceeded,
			map[string]string{"image": "registry.example/app:v1"}),
	))

	results := r.resolveResults(context.Background(), run, testWorkflow(), decls, extractor)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %+v", results)
	}
	image := resultValue(t, results, "image")
	if image.Value != "registry.example/app:v1" {
		t.Errorf("image value = %q", image.Value)
	}
	if image.Description != "The published image reference" {
		t.Errorf("description was not carried into status: %q", image.Description)
	}
	if image.Truncated {
		t.Error("a short value should not be marked truncated")
	}
	if got := resultValue(t, results, "git-revision").Value; got != "abc123" {
		t.Errorf("git-revision value = %q", got)
	}
	if events := drainEvents(recorder); len(events) != 0 {
		t.Errorf("a clean resolution should emit no events, got %v", events)
	}
}

func TestResolveResultsMissingTaskEmitsEventAndNoEntry(t *testing.T) {
	r, recorder := newResultsReconciler(ResultLimits{})
	run := testRun(t, "")

	decls := []openchoreodevv1alpha1.WorkflowResult{
		taskResultDecl("image", "publish-image", "image"),
		taskResultDecl("missing", "no-such-task", "value"),
		taskResultDecl("wrong-output", "publish-image", "no-such-output"),
	}
	extractor := newArgoResultExtractor(argoRun(
		podNode("build[0].publish-image", "publish-image", argoproj.NodeSucceeded,
			map[string]string{"image": "registry.example/app:v1"}),
	))

	results := r.resolveResults(context.Background(), run, testWorkflow(), decls, extractor)
	if len(results) != 1 {
		t.Fatalf("only the resolvable result should be recorded, got %+v", results)
	}
	events := drainEvents(recorder)
	if got := eventsMentioning(events, reasonResultExtractionFailed); got != 2 {
		t.Errorf("expected 2 extraction-failure events, got %d: %v", got, events)
	}
	// The two failures point at different mistakes and must say so.
	joined := strings.Join(events, "\n")
	if !strings.Contains(joined, "produced no outputs") {
		t.Errorf("a missing task should be reported as such: %v", events)
	}
	if !strings.Contains(joined, "has no output named") {
		t.Errorf("a missing output should be reported as such: %v", events)
	}
}

// ---------------------------------------------------------------------------
// expression resolution
// ---------------------------------------------------------------------------

func TestResolveResultsFromExpression(t *testing.T) {
	r, _ := newResultsReconciler(ResultLimits{})
	run := testRun(t, `{"repository":{"url":"https://git.example/app"}}`)

	decls := []openchoreodevv1alpha1.WorkflowResult{
		expressionDecl("image", `${tasks['publish-image'].results['image']}`),
		expressionDecl("source", `${parameters.repository.url}`),
		expressionDecl("run-name", `${metadata.workflowRunName}`),
		expressionDecl("tests-ran", `${tasks['tests'].phase == 'Succeeded' ? 'yes' : 'no'}`),
		// results['image'] is resolved above, so a later declaration can build on it.
		expressionDecl("image-and-source", `${results['image'] + ' from ' + results['source']}`),
	}

	extractor := newArgoResultExtractor(argoRun(
		podNode("build[0].publish-image", "publish-image", argoproj.NodeSucceeded,
			map[string]string{"image": "registry.example/app:v1"}),
		podNode("build[1].tests", "tests", argoproj.NodeSucceeded, nil),
	))

	results := r.resolveResults(context.Background(), run, testWorkflow(), decls, extractor)
	if len(results) != len(decls) {
		t.Fatalf("expected %d results, got %+v", len(decls), results)
	}
	for name, want := range map[string]string{
		"image":            "registry.example/app:v1",
		"source":           "https://git.example/app",
		"run-name":         "run-1",
		"tests-ran":        "yes",
		"image-and-source": "registry.example/app:v1 from https://git.example/app",
	} {
		if got := resultValue(t, results, name).Value; got != want {
			t.Errorf("result %q = %q, want %q", name, got, want)
		}
	}
}

func TestResolveResultsExpressionObjectIsJSONEncoded(t *testing.T) {
	r, _ := newResultsReconciler(ResultLimits{})
	run := testRun(t, "")

	decls := []openchoreodevv1alpha1.WorkflowResult{
		expressionDecl("summary", `${{"coveragePercent": tasks['tests'].results['coverage-percent']}}`),
	}
	extractor := newArgoResultExtractor(argoRun(
		podNode("build[0].tests", "tests", argoproj.NodeSucceeded,
			map[string]string{"coverage-percent": "87.5"}),
	))

	results := r.resolveResults(context.Background(), run, testWorkflow(), decls, extractor)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %+v", results)
	}
	if got := results[0].Value; got != `{"coveragePercent":"87.5"}` {
		t.Errorf("expression object = %q", got)
	}
}

func TestResolveResultsBadExpressionEmitsEvent(t *testing.T) {
	r, recorder := newResultsReconciler(ResultLimits{})
	run := testRun(t, "")

	decls := []openchoreodevv1alpha1.WorkflowResult{
		expressionDecl("broken", `${tasks['nope'].results['nope']}`),
	}
	results := r.resolveResults(context.Background(), run, testWorkflow(), decls,
		newArgoResultExtractor(argoRun(podNode("build[0].a", "a", argoproj.NodeSucceeded, map[string]string{"x": "1"}))))

	if len(results) != 0 {
		t.Fatalf("an expression that does not evaluate should record nothing, got %+v", results)
	}
	if got := eventsMentioning(drainEvents(recorder), reasonResultExtractionFailed); got != 1 {
		t.Errorf("expected one extraction-failure event, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// size caps
// ---------------------------------------------------------------------------

func TestResolveResultsTruncatesOversizedValue(t *testing.T) {
	r, recorder := newResultsReconciler(ResultLimits{ValueMaxBytes: 16})
	run := testRun(t, "")

	long := strings.Repeat("x", 100)
	extractor := newArgoResultExtractor(argoRun(
		podNode("build[0].tests", "tests", argoproj.NodeSucceeded, map[string]string{"report": long}),
	))

	results := r.resolveResults(context.Background(), run, testWorkflow(),
		[]openchoreodevv1alpha1.WorkflowResult{taskResultDecl("report", "tests", "report")}, extractor)

	if len(results) != 1 {
		t.Fatalf("an oversized value should still be recorded, got %+v", results)
	}
	entry := results[0]
	if !entry.Truncated {
		t.Error("the entry should be marked truncated")
	}
	if len(entry.Value) > 16 {
		t.Errorf("value was not capped: %d bytes", len(entry.Value))
	}
	if got := eventsMentioning(drainEvents(recorder), reasonResultTruncated); got != 1 {
		t.Errorf("expected one truncation event, got %d", got)
	}
}

func TestTruncateResultValueCutsOnUTF8Boundary(t *testing.T) {
	// A 4-byte rune straddling the cap must not leave an invalid trailing fragment.
	value := strings.Repeat("a", 14) + "🙂"
	got, truncated := truncateResultValue(value, 16)
	if !truncated {
		t.Fatal("expected truncation")
	}
	if !isValidUTF8(got) {
		t.Errorf("truncated value is not valid UTF-8: %q", got)
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func TestResolveResultsStopsAtRunBudget(t *testing.T) {
	r, recorder := newResultsReconciler(ResultLimits{ValueMaxBytes: 64, TotalMaxBytes: 100})
	run := testRun(t, "")

	sixty := strings.Repeat("y", 60)
	extractor := newArgoResultExtractor(argoRun(
		podNode("build[0].tests", "tests", argoproj.NodeSucceeded,
			map[string]string{"a": sixty, "b": sixty, "c": sixty}),
	))

	results := r.resolveResults(context.Background(), run, testWorkflow(), []openchoreodevv1alpha1.WorkflowResult{
		taskResultDecl("a", "tests", "a"),
		taskResultDecl("b", "tests", "b"),
		taskResultDecl("c", "tests", "c"),
	}, extractor)

	if len(results) != 1 {
		t.Fatalf("only the first result fits in a 100 byte budget, got %d: %+v", len(results), results)
	}
	// The one that did fit is complete, not a fragment.
	if len(results[0].Value) != 60 {
		t.Errorf("the recorded result should be whole, got %d bytes", len(results[0].Value))
	}
	if got := eventsMentioning(drainEvents(recorder), reasonResultsBudgetExceeded); got != 1 {
		t.Errorf("expected one budget event, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// sensitive results
// ---------------------------------------------------------------------------

func TestResolveResultsSensitiveWithholdsValue(t *testing.T) {
	r, _ := newResultsReconciler(ResultLimits{})
	run := testRun(t, "")

	decl := taskResultDecl("db-password", "provision", "password")
	decl.Sensitive = true

	extractor := newArgoResultExtractor(argoRun(
		podNode("build[0].provision", "provision", argoproj.NodeSucceeded,
			map[string]string{"password": "hunter2"}),
	))

	results := r.resolveResults(context.Background(), run, testWorkflow(),
		[]openchoreodevv1alpha1.WorkflowResult{
			decl,
			// A later expression must not be able to read the withheld value back out.
			expressionDecl("leak", `${'db-password' in results ? results['db-password'] : 'withheld'}`),
		}, extractor)

	entry := resultValue(t, results, "db-password")
	if entry.Value != "" {
		t.Errorf("a sensitive result must not carry its value into status, got %q", entry.Value)
	}
	if !entry.Sensitive {
		t.Error("the entry should be marked sensitive")
	}
	if got := resultValue(t, results, "leak").Value; got != "withheld" {
		t.Errorf("a sensitive value leaked into a later expression: %q", got)
	}
}

// ---------------------------------------------------------------------------
// test report projection
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// no-results workflows are unaffected
// ---------------------------------------------------------------------------

func TestResolveResultsNoDeclarationsIsInert(t *testing.T) {
	r, recorder := newResultsReconciler(ResultLimits{})
	results := r.resolveResults(context.Background(), testRun(t, ""), testWorkflow(), nil,
		newArgoResultExtractor(argoRun(
			podNode("build[0].a", "a", argoproj.NodeSucceeded, map[string]string{"x": "1"}))))

	if results != nil {
		t.Errorf("a workflow declaring no results should record none, got %+v", results)
	}
	if events := drainEvents(recorder); len(events) != 0 {
		t.Errorf("and emit no events, got %v", events)
	}
}

func TestSyncWorkflowRunStatusRecordsResultsOnFailure(t *testing.T) {
	// A failed run still surfaces what its steps produced: a coverage number from a build
	// that later failed is exactly the case this exists for.
	r, _ := newResultsReconciler(ResultLimits{})
	run := testRun(t, "")

	argoWorkflow := argoRun(
		podNode("build[0].run-tests", "run-tests", argoproj.NodeSucceeded,
			map[string]string{"coverage-percent": "62.0"}),
		podNode("build[1].publish-image", "publish-image", argoproj.NodeFailed, nil),
	)
	argoWorkflow.Status.Phase = argoproj.WorkflowFailed

	r.syncWorkflowRunStatus(context.Background(), run,
		testWorkflow(
			taskResultDecl("coverage-percent", "run-tests", "coverage-percent"),
		), argoWorkflow)

	if got := resultValue(t, run.Status.Results, "coverage-percent").Value; got != "62.0" {
		t.Errorf("coverage-percent = %q on a failed run", got)
	}
}

func TestSyncWorkflowRunStatusLeavesResultsUnsetWhileRunning(t *testing.T) {
	r, _ := newResultsReconciler(ResultLimits{})
	run := testRun(t, "")

	argoWorkflow := argoRun(podNode("build[0].run-tests", "run-tests", argoproj.NodeRunning, nil))
	argoWorkflow.Status.Phase = argoproj.WorkflowRunning

	r.syncWorkflowRunStatus(context.Background(), run,
		testWorkflow(
			taskResultDecl("coverage-percent", "run-tests", "coverage-percent"),
		), argoWorkflow)

	if run.Status.Results != nil {
		t.Errorf("results should only be resolved once the run finishes, got %+v", run.Status.Results)
	}
}
