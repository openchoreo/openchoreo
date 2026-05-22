// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/onsi/gomega"
)

// AssertWorkflowRunSucceeded checks the WorkflowRun's
// status.conditions[type==WorkflowSucceeded].status == True.
// The controller sets this once the underlying Argo Workflow reports success.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertWorkflowRunSucceeded(g gomega.Gomega, kubeContext, namespace, name string) {
	AssertJsonpathEquals(g, kubeContext, namespace, "workflowrun", name,
		`{.status.conditions[?(@.type=="WorkflowSucceeded")].status}`, "True")
}

// AssertWorkflowRunCompleted checks the WorkflowRun reports
// status.conditions[type==WorkflowCompleted].status == True. Use this when a
// spec wants to wait for the run to finish but does not care whether it
// succeeded — e.g., when a deliberately failing build is being asserted.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertWorkflowRunCompleted(g gomega.Gomega, kubeContext, namespace, name string) {
	AssertJsonpathEquals(g, kubeContext, namespace, "workflowrun", name,
		`{.status.conditions[?(@.type=="WorkflowCompleted")].status}`, "True")
}

// AssertComponentReleasePresent checks that at least one ComponentRelease in
// the namespace has spec.owner.componentName == component. ComponentRelease
// has no Ready status (and no labels), so existence is the meaningful signal
// that the build pipeline produced a release artifact.
// Designed for use inside Eventually(func(g Gomega) { ... }).
func AssertComponentReleasePresent(g gomega.Gomega, kubeContext, namespace, component string) {
	output, err := Kubectl(kubeContext,
		"get", "componentrelease",
		"-n", namespace,
		"-o", `jsonpath={.items[*].spec.owner.componentName}`,
	)
	g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("failed to list ComponentReleases in %s", namespace))
	found := false
	for _, name := range strings.Fields(output) {
		if name == component {
			found = true
			break
		}
	}
	g.Expect(found).To(gomega.BeTrue(),
		fmt.Sprintf("expected at least one ComponentRelease for component %q in %s, got: %q",
			component, namespace, output))
}

// WorkflowRunReference returns the rendered workflow's Kind/Name/Namespace
// pointer copied from WorkflowRun.status.runReference. Returns ("", "", "")
// (all empty) when the controller has not populated runReference yet.
func WorkflowRunReference(kubeContext, namespace, workflowRunName string) (kind, name, ns string, err error) {
	out, err := KubectlGetJsonpath(kubeContext, namespace, "workflowrun", workflowRunName,
		`{.status.runReference}`)
	if err != nil {
		return "", "", "", err
	}
	out = strings.TrimSpace(out)
	if out == "" || out == "null" {
		return "", "", "", nil
	}
	var ref struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
	}
	if err := json.Unmarshal([]byte(out), &ref); err != nil {
		return "", "", "", fmt.Errorf("decode runReference: %w (raw: %s)", err, out)
	}
	return ref.Kind, ref.Name, ref.Namespace, nil
}
