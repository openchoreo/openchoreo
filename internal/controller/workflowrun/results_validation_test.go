// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// These specs run the results schema past a real API server rather than the Go types.
// Everything asserted here is enforced by the CRD, not by controller code: an XValidation
// rule that is syntactically valid but semantically wrong would pass every unit test in
// this package and still let a broken Workflow be created in a cluster.

var _ = Describe("Workflow results schema", func() {
	var counter int

	// Each spec gets its own object name so a rejected create cannot be confused with a
	// name collision from an earlier spec.
	newWorkflow := func(results []openchoreov1alpha1.WorkflowResult) *openchoreov1alpha1.Workflow {
		counter++
		return &openchoreov1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("results-schema-%d", counter),
				Namespace: "default",
			},
			Spec: openchoreov1alpha1.WorkflowSpec{
				RunTemplate: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow"}`)},
				Results:     results,
			},
		}
	}

	taskResult := func(name, task, result string) openchoreov1alpha1.WorkflowResult {
		return openchoreov1alpha1.WorkflowResult{
			Name: name,
			ValueFrom: openchoreov1alpha1.WorkflowResultSource{
				TaskResult: &openchoreov1alpha1.TaskResultRef{Task: task, Result: result},
			},
		}
	}

	Context("valueFrom requires exactly one source", func() {
		It("accepts a taskResult source", func() {
			wf := newWorkflow([]openchoreov1alpha1.WorkflowResult{
				taskResult("image", "publish-image", "image"),
			})
			Expect(k8sClient.Create(ctx, wf)).To(Succeed())
			Expect(k8sClient.Delete(ctx, wf)).To(Succeed())
		})

		It("accepts an expression source", func() {
			wf := newWorkflow([]openchoreov1alpha1.WorkflowResult{{
				Name: "verdict",
				ValueFrom: openchoreov1alpha1.WorkflowResultSource{
					Expression: "${tasks['run-tests'].results['tests-failed']}",
				},
			}})
			Expect(k8sClient.Create(ctx, wf)).To(Succeed())
			Expect(k8sClient.Delete(ctx, wf)).To(Succeed())
		})

		It("rejects both sources at once", func() {
			wf := newWorkflow([]openchoreov1alpha1.WorkflowResult{{
				Name: "ambiguous",
				ValueFrom: openchoreov1alpha1.WorkflowResultSource{
					TaskResult: &openchoreov1alpha1.TaskResultRef{Task: "a", Result: "b"},
					Expression: "${metadata.workflowRunName}",
				},
			}})
			err := k8sClient.Create(ctx, wf)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly one of taskResult or expression"))
		})

		It("rejects neither source", func() {
			wf := newWorkflow([]openchoreov1alpha1.WorkflowResult{{
				Name:      "empty",
				ValueFrom: openchoreov1alpha1.WorkflowResultSource{},
			}})
			err := k8sClient.Create(ctx, wf)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly one of taskResult or expression"))
		})
	})

	Context("result names", func() {
		It("rejects a name that is not a DNS-style label", func() {
			wf := newWorkflow([]openchoreov1alpha1.WorkflowResult{
				taskResult("Image_Ref", "publish-image", "image"),
			})
			Expect(k8sClient.Create(ctx, wf)).NotTo(Succeed())
		})

		It("rejects two results sharing a name", func() {
			// listMapKey=name: two entries under one key would make which one wins depend
			// on apply order.
			wf := newWorkflow([]openchoreov1alpha1.WorkflowResult{
				taskResult("image", "publish-image", "image"),
				taskResult("image", "checkout-source", "git-revision"),
			})
			Expect(k8sClient.Create(ctx, wf)).NotTo(Succeed())
		})
	})

	Context("expression form", func() {
		It("rejects an expression that is not wrapped in ${...}", func() {
			wf := newWorkflow([]openchoreov1alpha1.WorkflowResult{{
				Name: "bare",
				ValueFrom: openchoreov1alpha1.WorkflowResultSource{
					Expression: "tasks['a'].results['b']",
				},
			}})
			Expect(k8sClient.Create(ctx, wf)).NotTo(Succeed())
		})
	})
})

var _ = Describe("WorkflowRun results status schema", func() {
	var counter int

	newRun := func() *openchoreov1alpha1.WorkflowRun {
		counter++
		return &openchoreov1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("results-status-%d", counter),
				Namespace: "default",
			},
			Spec: openchoreov1alpha1.WorkflowRunSpec{
				Workflow: openchoreov1alpha1.WorkflowRunConfig{
					Kind: openchoreov1alpha1.WorkflowRefKindClusterWorkflow,
					Name: "some-workflow",
				},
			},
		}
	}

	It("round-trips results through the API server", func() {
		run := newRun()
		Expect(k8sClient.Create(ctx, run)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, run) })

		run.Status.Results = []openchoreov1alpha1.WorkflowRunResult{
			{Name: "image", Description: "published image", Value: "registry.example/app:v1"},
			{Name: "big", Value: "truncated...", Truncated: true},
			{Name: "db-password", Sensitive: true},
		}
		Expect(k8sClient.Status().Update(ctx, run)).To(Succeed())

		fetched := &openchoreov1alpha1.WorkflowRun{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(run), fetched)).To(Succeed())

		Expect(fetched.Status.Results).To(HaveLen(3))
		Expect(fetched.Status.Results[0].Value).To(Equal("registry.example/app:v1"))
		Expect(fetched.Status.Results[1].Truncated).To(BeTrue())
		// A sensitive entry survives with an empty value, which is the whole point: the
		// consumer can tell the run produced it without status carrying the value.
		Expect(fetched.Status.Results[2].Sensitive).To(BeTrue())
		Expect(fetched.Status.Results[2].Value).To(BeEmpty())
	})

	It("rejects two status results sharing a name", func() {
		run := newRun()
		Expect(k8sClient.Create(ctx, run)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, run) })

		run.Status.Results = []openchoreov1alpha1.WorkflowRunResult{
			{Name: "image", Value: "a"},
			{Name: "image", Value: "b"},
		}
		Expect(k8sClient.Status().Update(ctx, run)).NotTo(Succeed())
	})
})
