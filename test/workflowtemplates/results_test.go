// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowtemplates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// A declared result names a step of the run template and an output that step's
// ClusterWorkflowTemplate actually produces. Both halves are strings resolved at run time,
// so a typo in either is invisible until a real build finishes and the result quietly does
// not appear. These tests are what makes that a build failure instead.

// testingSampleDir holds the custom-steps testing sample, which lives outside the
// getting-started tree that the other helpers default to.
const testingSampleDir = "../../samples/workflows/custom-steps/testing"

// stepTemplateFile maps a ClusterWorkflowTemplate name to the file that defines it, so a
// step's declared outputs can be looked up from the step it refers to.
var stepTemplateFile = map[string]string{
	"checkout-source":           "checkout-source.yaml",
	"publish-image":             "publish-image.yaml",
	"containerfile-build":       "containerfile-build.yaml",
	"ballerina-buildpack-build": "ballerina-buildpack-build.yaml",
	"gcp-buildpacks-build":      "gcp-buildpacks-build.yaml",
	"paketo-buildpacks-build":   "paketo-buildpacks-build.yaml",
	"generate-workload":         "generate-workload.yaml",
	// Lives in the custom-steps testing sample rather than the getting-started tree.
	"run-tests": "run-tests.yaml",
}

// templateDirFor returns the directory holding a step template's file. Most live in the
// getting-started tree; the custom-steps samples bring their own.
func templateDirFor(templateName string) string {
	if templateName == "run-tests" {
		return testingSampleDir
	}
	return templatesDir
}

// TestCIWorkflows_DeclareBuildResults pins the results the shipped builders surface. These
// two values were already produced as Argo step outputs before results existed; declaring
// them is what moves a consumer off pod logs and off the WorkflowRun annotation round-trip.
func TestCIWorkflows_DeclareBuildResults(t *testing.T) {
	for _, tc := range ciWorkflowContracts {
		t.Run(tc.file, func(t *testing.T) {
			wf := loadCIWorkflow(t, tc.file)

			image := requireResult(t, wf, "image",
				"every shipped builder must surface the image it published as a result")
			requireTaskResult(t, image, "publish-image", "image",
				"the image result must read publish-image's image output")

			revision := requireResult(t, wf, "git-revision",
				"every shipped builder must surface the revision it built as a result")
			requireTaskResult(t, revision, "checkout-source", "git-revision",
				"the git-revision result must read checkout-source's git-revision output")
		})
	}
}

// TestCIWorkflows_ResultsResolveToRealOutputs is the check that catches a typo: every task
// a result names must be a step in the run template, and every output it names must be one
// that step's template declares.
func TestCIWorkflows_ResultsResolveToRealOutputs(t *testing.T) {
	for _, tc := range ciWorkflowContracts {
		t.Run(tc.file, func(t *testing.T) {
			assertResultsResolve(t, loadCIWorkflow(t, tc.file))
		})
	}
}

func assertResultsResolve(t *testing.T, wf clusterWorkflow) {
	t.Helper()

	// Index the run template's steps by name, and the template each refers to.
	stepTemplate := map[string]string{}
	var stepList []string
	for _, tmpl := range wf.Spec.RunTemplate.Spec.Templates {
		for _, group := range tmpl.Steps {
			for _, step := range group {
				stepTemplate[step.Name] = step.TemplateRef.Name
				stepList = append(stepList, step.Name)
			}
		}
	}

	for _, result := range wf.Spec.Results {
		ref := result.ValueFrom.TaskResult
		if ref == nil {
			// An expression result is checked by the CEL engine at run time; there is no
			// static task name to resolve here.
			continue
		}

		templateName, ok := stepTemplate[ref.Task]
		if !ok {
			t.Fatalf(`
contract:
  result %q names a task that is not a step of this workflow

named task:
  %s

steps in the run template:
%s`,
				result.Name, ref.Task, formatStringList(stepList))
		}

		file, known := stepTemplateFile[templateName]
		if !known {
			t.Fatalf(`
contract:
  result %q refers to step %q, whose template %q is not covered by this test

add it to stepTemplateFile so its outputs can be checked`,
				result.Name, ref.Task, templateName)
		}

		outputs := declaredOutputs(t, templateDirFor(templateName), file, templateName)
		if !contains(outputs, ref.Result) {
			t.Fatalf(`
contract:
  result %q reads an output that %s does not declare

named output:
  %s

outputs declared by %s:
%s`,
				result.Name, templateName, ref.Result, templateName, formatStringList(outputs))
		}
	}
}

// declaredOutputs returns the output parameter names an Argo template declares. The
// template name doubles as the inner template name in every shipped file except
// checkout-source, whose template is called "checkout".
func declaredOutputs(t *testing.T, dir, file, templateName string) []string {
	t.Helper()

	tmpl := loadTemplateFromDir(t, dir, file)
	inner := templateName
	if templateName == "checkout-source" {
		inner = "checkout"
	}
	if templateName == "generate-workload" {
		inner = "generate-workload-cr"
	}

	for _, step := range tmpl.Spec.Templates {
		if step.Name != inner {
			continue
		}
		names := make([]string, 0, len(step.Outputs.Parameters))
		for _, p := range step.Outputs.Parameters {
			names = append(names, p.Name)
		}
		return names
	}

	var available []string
	for _, step := range tmpl.Spec.Templates {
		available = append(available, step.Name)
	}
	t.Fatalf(`
contract:
  template %q declares no inner template %q

templates in %s:
%s`, file, inner, file, formatStringList(available))
	return nil
}

// TestTestingSample_ResultsResolveToRealOutputs covers the custom-steps testing sample the
// same way. It is not one of the shipped builders, so it is loaded from its own directory —
// but it declares five results against a template in this repo, and a result naming an
// output run-tests does not emit would be just as invisible until a real build ran.
func TestTestingSample_ResultsResolveToRealOutputs(t *testing.T) {
	wf := loadClusterWorkflowFromDir(t, testingSampleDir, "dockerfile-builder-tests.yaml")

	outputs := declaredOutputsFromDir(t, testingSampleDir, "run-tests.yaml", "run-tests")

	// tests-verdict must read the exit-code-derived outcome, not a parsed count. With the
	// sample's defaults no JUnit report is produced, so every count is empty, and an empty
	// count is not a pass.
	verdict := requireResult(t, wf, "tests-verdict",
		"the testing sample must report a verdict that holds when no report was parsed")
	requireTaskResult(t, verdict, "run-tests", "tests-outcome",
		"tests-verdict must read tests-outcome, which is derived from the test command's exit code")

	// Every taskResult in the sample, not a hand-listed subset - a result added later must
	// be checked too, and the ones reading the shipped build steps resolve through the same
	// helper the builders use.
	if len(wf.Spec.Results) == 0 {
		t.Fatal("the testing sample declares no results")
	}
	assertResultsResolve(t, wf)

	// assertResultsResolve only knows the getting-started templates, so the run-tests
	// references are checked here against run-tests.yaml's own outputs.
	checked := 0
	for _, result := range wf.Spec.Results {
		ref := result.ValueFrom.TaskResult
		if ref == nil || ref.Task != "run-tests" {
			continue
		}
		checked++
		if !contains(outputs, ref.Result) {
			t.Fatalf(`
contract:
  result %q reads an output that run-tests does not declare

named output:
  %s

outputs declared by run-tests:
%s`, result.Name, ref.Result, formatStringList(outputs))
		}
	}
	if checked == 0 {
		t.Fatal("no result reads a run-tests output, so this test checked nothing")
	}
}

// loadClusterWorkflowFromDir parses a ClusterWorkflow from any directory. loadCIWorkflow
// is hard-wired to the getting-started tree; the custom-steps samples live elsewhere.
func loadClusterWorkflowFromDir(t *testing.T, dir, filename string) clusterWorkflow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filename))
	require.NoError(t, err, "reading ClusterWorkflow %s/%s", dir, filename)

	var wf clusterWorkflow
	require.NoError(t, yaml.Unmarshal(data, &wf), "unmarshalling ClusterWorkflow %s/%s", dir, filename)
	return wf
}

// declaredOutputsFromDir returns the output parameter names an Argo template declares,
// reading the template from an explicit directory.
func declaredOutputsFromDir(t *testing.T, dir, file, templateName string) []string {
	t.Helper()
	step := workflowTemplateByNameFromDir(t, dir, file, templateName)
	names := make([]string, 0, len(step.Outputs.Parameters))
	for _, p := range step.Outputs.Parameters {
		names = append(names, p.Name)
	}
	return names
}

func requireResult(t *testing.T, wf clusterWorkflow, name string, contract string) workflowResult {
	t.Helper()
	var names []string
	for _, r := range wf.Spec.Results {
		if r.Name == name {
			return r
		}
		names = append(names, r.Name)
	}
	t.Fatalf(`
contract:
  %s

missing result:
  %s

declared results:
%s`, contract, name, formatStringList(names))
	return workflowResult{}
}

func requireTaskResult(t *testing.T, result workflowResult, task, output string, contract string) {
	t.Helper()
	ref := result.ValueFrom.TaskResult
	if ref == nil {
		t.Fatalf(`
contract:
  %s

result %q has no valueFrom.taskResult`, contract, result.Name)
	}
	requireEqualContract(t, ref.Task, task, contract)
	requireEqualContract(t, ref.Result, output, contract)
	requireEqualContract(t, strings.TrimSpace(result.Description) != "", true,
		"a declared result must carry a description, since it is what a consumer renders")
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
