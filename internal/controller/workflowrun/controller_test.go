// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
)

var _ = Describe("WorkflowRun Controller", func() {
	const (
		namespace = "default"
		timeout   = time.Second * 10
		interval  = time.Millisecond * 250
	)

	var (
		testCtx context.Context
	)

	BeforeEach(func() {
		testCtx = context.Background()
	})

	Context("When reconciling a non-existent WorkflowRun", func() {
		It("should not return an error", func() {
			reconciler := &Reconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				k8sClientMgr: kubernetesClient.NewManager(),
			}

			result, err := reconciler.Reconcile(testCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent-workflowrun",
					Namespace: namespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("When creating a WorkflowRun", func() {
		var (
			workflowRun *openchoreodevv1alpha1.WorkflowRun
			workflow    *openchoreodevv1alpha1.Workflow
		)

		BeforeEach(func() {
			// Create a Workflow first
			workflowName := "test-workflow-" + time.Now().Format("20060102150405")
			workflow = &openchoreodevv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workflowName,
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.WorkflowSpec{
					Schema: mustMarshalRaw(map[string]any{
						"repository": map[string]any{
							"url": "string",
						},
					}),
					Resource: mustMarshalRaw(map[string]any{
						"apiVersion": "argoproj.io/v1alpha1",
						"kind":       "Workflow",
						"metadata": map[string]any{
							"name":      "test-workflow",
							"namespace": "default",
						},
						"spec": map[string]any{
							"serviceAccountName": "build-bot",
							"entrypoint":         "main",
							"templates": []any{
								map[string]any{
									"name": "main",
									"container": map[string]any{
										"image":   "alpine:latest",
										"command": []string{"echo", "hello"},
									},
								},
							},
						},
					}),
				},
			}
			Expect(k8sClient.Create(testCtx, workflow)).To(Succeed())

			// Create a WorkflowRun
			workflowRunName := "test-workflowrun-" + time.Now().Format("20060102150405")
			workflowRun = &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workflowRunName,
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Owner: openchoreodevv1alpha1.WorkflowOwner{
						ProjectName:   "test-project",
						ComponentName: "test-component",
					},
					Workflow: openchoreodevv1alpha1.WorkflowConfig{
						Name: workflowName,
						Schema: mustMarshalRaw(map[string]any{
							"repository": map[string]any{
								"url": "https://github.com/example/repo.git",
							},
						}),
					},
				},
			}
			Expect(k8sClient.Create(testCtx, workflowRun)).To(Succeed())
		})

		AfterEach(func() {
			if workflowRun != nil {
				_ = k8sClient.Delete(testCtx, workflowRun)
			}
			if workflow != nil {
				_ = k8sClient.Delete(testCtx, workflow)
			}
		})

		It("should set WorkflowPending condition on initial reconcile", func() {
			reconciler := &Reconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				k8sClientMgr: kubernetesClient.NewManager(),
			}

			_, err := reconciler.Reconcile(testCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      workflowRun.Name,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				updatedWorkflowRun := &openchoreodevv1alpha1.WorkflowRun{}
				if err := k8sClient.Get(testCtx, types.NamespacedName{
					Name:      workflowRun.Name,
					Namespace: namespace,
				}, updatedWorkflowRun); err != nil {
					return false
				}
				return isWorkflowInitiated(updatedWorkflowRun)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When checking condition helper functions", func() {
		It("should correctly identify workflow states", func() {
			workflowRun := &openchoreodevv1alpha1.WorkflowRun{
				Status: openchoreodevv1alpha1.WorkflowRunStatus{
					Conditions: []metav1.Condition{},
				},
			}

			// Initially not initiated
			Expect(isWorkflowInitiated(workflowRun)).To(BeFalse())
			Expect(isWorkflowCompleted(workflowRun)).To(BeFalse())

			// After setting pending condition
			setWorkflowPendingCondition(workflowRun)
			Expect(isWorkflowInitiated(workflowRun)).To(BeTrue())
			Expect(isWorkflowCompleted(workflowRun)).To(BeFalse())

			// After setting succeeded condition
			setWorkflowSucceededCondition(workflowRun)
			Expect(isWorkflowCompleted(workflowRun)).To(BeTrue())
			Expect(isWorkflowSucceeded(workflowRun)).To(BeTrue())
		})

		It("should correctly identify workload update status", func() {
			workflowRun := &openchoreodevv1alpha1.WorkflowRun{
				Status: openchoreodevv1alpha1.WorkflowRunStatus{
					Conditions: []metav1.Condition{},
				},
			}

			Expect(isWorkloadUpdated(workflowRun)).To(BeFalse())

			setWorkloadUpdatedCondition(workflowRun)
			Expect(isWorkloadUpdated(workflowRun)).To(BeTrue())
		})
	})

	Context("When converting parameter values to strings", func() {
		It("should convert all parameter values to strings", func() {
			resource := map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"spec": map[string]any{
					"arguments": map[string]any{
						"parameters": []any{
							map[string]any{"name": "int-param", "value": 42},
							map[string]any{"name": "bool-param", "value": true},
							map[string]any{"name": "string-param", "value": "hello"},
							map[string]any{"name": "float-param", "value": 3.14},
						},
					},
				},
			}

			converted := convertParameterValuesToStrings(resource)
			spec := converted["spec"].(map[string]any)
			args := spec["arguments"].(map[string]any)
			params := args["parameters"].([]any)

			Expect(params).To(HaveLen(4))
			for _, p := range params {
				param := p.(map[string]any)
				value := param["value"]
				Expect(value).To(BeAssignableToTypeOf(""))
			}
		})
	})
})

// Helper functions

func mustMarshalRaw(v any) *runtime.RawExtension {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return &runtime.RawExtension{Raw: data}
}
