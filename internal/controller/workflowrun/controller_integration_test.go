// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// forceDelete removes finalizers and deletes a WorkflowRun. Used in AfterEach cleanup.
func forceDelete(ctx context.Context, nn types.NamespacedName) {
	resource := &openchoreodevv1alpha1.WorkflowRun{}
	if err := k8sClient.Get(ctx, nn, resource); err != nil {
		return // already gone
	}
	if controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer) {
		controllerutil.RemoveFinalizer(resource, WorkflowRunCleanupFinalizer)
		_ = k8sClient.Update(ctx, resource)
	}
	_ = k8sClient.Delete(ctx, resource)
}

// forceDeleteClusterWorkflow removes a ClusterWorkflow resource if it exists.
func forceDeleteClusterWorkflow(ctx context.Context, name string) {
	cwf := &openchoreodevv1alpha1.ClusterWorkflow{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, cwf); err == nil {
		_ = k8sClient.Delete(ctx, cwf)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: WorkflowRun resource CRUD
// ---------------------------------------------------------------------------

var _ = Describe("WorkflowRun Controller Integration", func() {
	ctx := context.Background()

	Context("Resource creation and initial state", func() {
		const resourceName = "int-test-create"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())
		})

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should persist the resource and have empty status", func() {
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(resource.Name).To(Equal(resourceName))
			Expect(resource.Spec.Workflow.Name).To(Equal("test-workflow"))
			Expect(resource.Status.Conditions).To(BeEmpty())
			Expect(resource.Status.RunReference).To(BeNil())
			Expect(resource.Status.Resources).To(BeNil())
		})
	})

	// ---------------------------------------------------------------------------
	// Reconcile: resource not found
	// ---------------------------------------------------------------------------

	Context("Reconciling a non-existent resource", func() {
		It("should return no error and no requeue", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "non-existent", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})
	})

	// ---------------------------------------------------------------------------
	// Reconcile: first reconcile adds finalizer
	// ---------------------------------------------------------------------------

	Context("First reconcile adds finalizer", func() {
		const resourceName = "int-test-finalizer-add"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())
		})

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should add the finalizer and return early", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			By("Verifying the finalizer was added")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer)).To(BeTrue())
		})
	})

	// ---------------------------------------------------------------------------
	// Reconcile: second reconcile sets pending condition and StartedAt
	// ---------------------------------------------------------------------------

	Context("Second reconcile sets pending condition", func() {
		const resourceName = "int-test-pending"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())
		})

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should set WorkflowCompleted=False/WorkflowPending and requeue", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			By("Verifying the status was updated")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())

			cond := meta.FindStatusCondition(resource.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonWorkflowPending)))

			Expect(resource.Status.StartedAt).NotTo(BeNil())
		})
	})

	// ---------------------------------------------------------------------------
	// Reconcile: no Workflow found → requeue
	// ---------------------------------------------------------------------------

	Context("Workflow not found after pending condition set", func() {
		const resourceName = "int-test-no-workflow"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "nonexistent-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Setting the pending condition via first reconcile")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should not requeue and set WorkflowNotFound condition when Workflow cannot be found", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			// Verify the WorkflowNotFound condition is set
			var wr openchoreodevv1alpha1.WorkflowRun
			Expect(k8sClient.Get(ctx, nn, &wr)).To(Succeed())
			cond := meta.FindStatusCondition(wr.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonWorkflowFailed)))
		})
	})

	// ---------------------------------------------------------------------------
	// Reconcile: Workflow exists but no WorkflowPlane → sets condition
	// ---------------------------------------------------------------------------

	Context("Workflow exists but no WorkflowPlane", func() {
		const (
			resourceName = "int-test-no-workflowplane"
			workflowName = "int-test-workflow-no-wp"
		)
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			By("Creating a minimal ClusterWorkflow")
			runTemplate := map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"metadata":   map[string]any{"name": "test", "namespace": "default"},
				"spec":       map[string]any{"entrypoint": "main"},
			}
			runTemplateJSON, err := json.Marshal(runTemplate)
			Expect(err).NotTo(HaveOccurred())

			clusterWorkflow := &openchoreodevv1alpha1.ClusterWorkflow{
				ObjectMeta: metav1.ObjectMeta{Name: workflowName},
				Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
					RunTemplate: &runtime.RawExtension{Raw: runTemplateJSON},
				},
			}
			Expect(k8sClient.Create(ctx, clusterWorkflow)).To(Succeed())

			By("Creating WorkflowRun with finalizer and pending condition")
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: workflowName},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Setting pending condition via reconcile")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
			forceDeleteClusterWorkflow(ctx, workflowName)
		})

		It("should set WorkflowPlaneNotFound condition and requeue after 1 minute", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Minute))

			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())

			cond := meta.FindStatusCondition(resource.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonWorkflowPlaneNotFound)))
		})
	})

	// ---------------------------------------------------------------------------
	// Finalizer lifecycle: add and remove
	// ---------------------------------------------------------------------------

	Context("Finalizer add and remove lifecycle", func() {
		const resourceName = "int-test-finalizer-lifecycle"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should add finalizer on create and remove on cleanup", func() {
			By("Creating a WorkflowRun without finalizer")
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Verifying no finalizer initially")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer)).To(BeFalse())

			By("Running first reconcile to let the controller add the finalizer")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the finalizer was added by the controller")
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer)).To(BeTrue())

			By("Deleting the WorkflowRun to set the deletion timestamp")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Running reconcile to trigger the controller cleanup path")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the resource is gone after finalization")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreodevv1alpha1.WorkflowRun{})
				return err != nil
			}, "5s", "250ms").Should(BeTrue())
		})
	})

	// ---------------------------------------------------------------------------
	// Status persistence: RunReference and Resources
	// ---------------------------------------------------------------------------

	Context("Status subresource persistence", func() {
		const resourceName = "int-test-status-persist"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())
		})

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should persist RunReference in status", func() {
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())

			resource.Status.RunReference = &openchoreodevv1alpha1.ResourceReference{
				APIVersion: "argoproj.io/v1alpha1",
				Kind:       "Workflow",
				Name:       "test-workflow-run",
				Namespace:  "build-namespace",
			}
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())

			By("Verifying the RunReference was persisted")
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(resource.Status.RunReference).NotTo(BeNil())
			Expect(resource.Status.RunReference.Name).To(Equal("test-workflow-run"))
			Expect(resource.Status.RunReference.Namespace).To(Equal("build-namespace"))
		})

		It("should persist Resources in status", func() {
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())

			resources := []openchoreodevv1alpha1.ResourceReference{
				{APIVersion: "v1", Kind: "Secret", Name: "registry-creds", Namespace: "build-ns"},
				{APIVersion: "v1", Kind: "ConfigMap", Name: "build-config", Namespace: "build-ns"},
			}
			resource.Status.Resources = &resources
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())

			By("Verifying the Resources were persisted")
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(resource.Status.Resources).NotTo(BeNil())
			Expect(*resource.Status.Resources).To(HaveLen(2))
			Expect((*resource.Status.Resources)[0].Kind).To(Equal("Secret"))
			Expect((*resource.Status.Resources)[1].Kind).To(Equal("ConfigMap"))
		})
	})

	// ---------------------------------------------------------------------------
	// TTL inheritance from Workflow
	// ---------------------------------------------------------------------------

	Context("TTL inheritance from Workflow", func() {
		const (
			workflowName = "int-test-wf-ttl"
		)

		BeforeEach(func() {
			runTemplate := map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"metadata":   map[string]any{"name": "${metadata.workflowRunName}", "namespace": "${metadata.namespaceName}"},
				"spec": map[string]any{
					"entrypoint": "main",
					"templates": []any{
						map[string]any{
							"name":      "main",
							"container": map[string]any{"image": "alpine:latest", "command": []string{"echo", "hello"}},
						},
					},
				},
			}
			runTemplateJSON, err := json.Marshal(runTemplate)
			Expect(err).NotTo(HaveOccurred())

			clusterWorkflow := &openchoreodevv1alpha1.ClusterWorkflow{
				ObjectMeta: metav1.ObjectMeta{Name: workflowName},
				Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
					RunTemplate:        &runtime.RawExtension{Raw: runTemplateJSON},
					TTLAfterCompletion: "1h",
				},
			}
			Expect(k8sClient.Create(ctx, clusterWorkflow)).To(Succeed())
		})

		AfterEach(func() { forceDeleteClusterWorkflow(ctx, workflowName) })

		It("should have empty TTL initially when created without TTLAfterCompletion", func() {
			resourceName := "int-test-ttl-inherit"
			nn := types.NamespacedName{Name: resourceName, Namespace: "default"}
			defer forceDelete(ctx, nn)

			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: workflowName},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Asserting TTL is empty before any reconcile")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(resource.Spec.TTLAfterCompletion).To(Equal(""))

			By("Verifying the parent ClusterWorkflow has TTLAfterCompletion set")
			cwf := &openchoreodevv1alpha1.ClusterWorkflow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: workflowName}, cwf)).To(Succeed())
			Expect(cwf.Spec.TTLAfterCompletion).To(Equal("1h"))

			By("Driving reconciles so the controller processes the WorkflowRun")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			// First reconcile: finalizer already present, sets WorkflowPending condition.
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// Second reconcile: WorkflowPending is set, fetches Workflow (ok), then attempts
			// ResolveWorkflowPlane. In this test environment no WorkflowPlane is configured, so the
			// controller returns early at that check (controller.go ResolveWorkflowPlane) and the
			// TTL copy branch (after ResolveWorkflowPlane) is not reached.
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// The controller should have set the WorkflowPlaneNotFound condition and requeued.
			Expect(result.RequeueAfter).To(Equal(time.Minute))

			By("Re-fetching and asserting TTL is still empty (no WorkflowPlane to trigger propagation)")
			fetched := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Spec.TTLAfterCompletion).To(Equal(""))
		})

		It("should not override explicit TTL", func() {
			resourceName := "int-test-ttl-explicit"
			nn := types.NamespacedName{Name: resourceName, Namespace: "default"}
			defer forceDelete(ctx, nn)

			// Pre-set the finalizer so the first reconcile skips ensureFinalizer and
			// progresses further through the reconcile loop.
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow:           openchoreodevv1alpha1.WorkflowRunConfig{Name: workflowName},
					TTLAfterCompletion: "30m",
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			By("First reconcile: finalizer present, sets WorkflowPending condition")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("Second reconcile: WorkflowPending set, progresses to ResolveWorkflowPlane check")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying TTL was not overridden after multiple reconcile passes")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(resource.Spec.TTLAfterCompletion).To(Equal("30m"))
		})
	})

	// ---------------------------------------------------------------------------
	// Component workflow validation: only project label
	// ---------------------------------------------------------------------------

	Context("WorkflowRun with only project label fails validation", func() {
		const resourceName = "int-test-only-project-label"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
					Labels: map[string]string{
						"openchoreo.dev/project": "my-proj",
					},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Setting pending condition via first reconcile")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should set WorkflowCompleted=True/ComponentValidationFailed", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())

			cond := meta.FindStatusCondition(resource.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonComponentValidationFailed)))
			Expect(cond.Message).To(ContainSubstring("must have both"))
		})
	})

	// ---------------------------------------------------------------------------
	// Component workflow validation: only component label
	// ---------------------------------------------------------------------------

	Context("WorkflowRun with only component label fails validation", func() {
		const resourceName = "int-test-only-comp-label"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
					Labels: map[string]string{
						"openchoreo.dev/component": "my-comp",
					},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Setting pending condition via first reconcile")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should set WorkflowCompleted=True/ComponentValidationFailed", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())

			cond := meta.FindStatusCondition(resource.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonComponentValidationFailed)))
		})
	})

	// ---------------------------------------------------------------------------
	// Component workflow validation: workflow not in allowedWorkflows
	// ---------------------------------------------------------------------------

	Context("WorkflowRun with workflow not in allowedWorkflows", func() {
		const (
			resourceName = "int-test-wf-not-allowed"
			ctName       = "int-val-ct"
			compName     = "int-val-comp"
		)
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			By("Creating ComponentType with allowedWorkflows=[allowed-wf]")
			ct := &openchoreodevv1alpha1.ComponentType{
				ObjectMeta: metav1.ObjectMeta{Name: ctName, Namespace: "default"},
				Spec: openchoreodevv1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					AllowedWorkflows: []openchoreodevv1alpha1.WorkflowRef{
						{Name: "allowed-wf"},
					},
					Resources: []openchoreodevv1alpha1.ResourceTemplate{
						{ID: "deployment", Template: &runtime.RawExtension{Raw: []byte("{}")}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			By("Creating Component referencing the ComponentType")
			comp := &openchoreodevv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: compName, Namespace: "default"},
				Spec: openchoreodevv1alpha1.ComponentSpec{
					Owner:         openchoreodevv1alpha1.ComponentOwner{ProjectName: "my-proj"},
					ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "deployment/" + ctName},
					Workflow:      &openchoreodevv1alpha1.ComponentWorkflowConfig{Name: "not-allowed-wf"},
				},
			}
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())

			By("Creating WorkflowRun with disallowed workflow")
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
					Labels: map[string]string{
						"openchoreo.dev/project":   "my-proj",
						"openchoreo.dev/component": compName,
					},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "not-allowed-wf"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Setting pending condition via first reconcile")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
			_ = k8sClient.Delete(ctx, &openchoreodevv1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Name: compName, Namespace: "default"}})
			_ = k8sClient.Delete(ctx, &openchoreodevv1alpha1.ComponentType{ObjectMeta: metav1.ObjectMeta{Name: ctName, Namespace: "default"}})
		})

		It("should set WorkflowCompleted=True/ComponentValidationFailed with not allowed message", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())

			cond := meta.FindStatusCondition(resource.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonComponentValidationFailed)))
			Expect(cond.Message).To(ContainSubstring("not allowed"))
		})
	})

	// ---------------------------------------------------------------------------
	// Component workflow validation: valid workflow run passes
	// ---------------------------------------------------------------------------

	Context("WorkflowRun with valid component workflow passes validation", func() {
		const (
			resourceName = "int-test-wf-valid"
			ctName       = "int-val-valid-ct"
			compName     = "int-val-valid-comp"
			workflowName = "int-val-valid-wf"
		)
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			By("Creating ComponentType with allowedWorkflows")
			ct := &openchoreodevv1alpha1.ComponentType{
				ObjectMeta: metav1.ObjectMeta{Name: ctName, Namespace: "default"},
				Spec: openchoreodevv1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					AllowedWorkflows: []openchoreodevv1alpha1.WorkflowRef{
						{Name: workflowName},
					},
					Resources: []openchoreodevv1alpha1.ResourceTemplate{
						{ID: "deployment", Template: &runtime.RawExtension{Raw: []byte("{}")}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			By("Creating Component with matching workflow")
			comp := &openchoreodevv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: compName, Namespace: "default"},
				Spec: openchoreodevv1alpha1.ComponentSpec{
					Owner:         openchoreodevv1alpha1.ComponentOwner{ProjectName: "my-proj"},
					ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "deployment/" + ctName},
					Workflow:      &openchoreodevv1alpha1.ComponentWorkflowConfig{Name: workflowName},
				},
			}
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())

			By("Creating WorkflowRun with valid workflow")
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
					Labels: map[string]string{
						"openchoreo.dev/project":   "my-proj",
						"openchoreo.dev/component": compName,
					},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: workflowName},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Setting pending condition via first reconcile")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
			_ = k8sClient.Delete(ctx, &openchoreodevv1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Name: compName, Namespace: "default"}})
			_ = k8sClient.Delete(ctx, &openchoreodevv1alpha1.ComponentType{ObjectMeta: metav1.ObjectMeta{Name: ctName, Namespace: "default"}})
		})

		It("should pass validation and proceed to workflow resolution (not found since no Workflow CR exists)", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// Validation passed → proceeds to ResolveWorkflow which will fail (no Workflow CR)
			// This means the condition should NOT be ComponentValidationFailed
			Expect(result.Requeue).To(BeFalse())

			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())

			cond := meta.FindStatusCondition(resource.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			// Should be WorkflowFailed (not found) rather than ComponentValidationFailed
			Expect(cond.Reason).To(Equal(string(ReasonWorkflowFailed)))
		})
	})

	// ---------------------------------------------------------------------------
	// Component workflow validation: standalone workflow run (no labels) proceeds normally
	// ---------------------------------------------------------------------------

	Context("Standalone WorkflowRun (no labels) proceeds normally", func() {
		const resourceName = "int-test-standalone-wfr"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "nonexistent-wf"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Setting pending condition via first reconcile")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should skip validation and proceed to workflow resolution", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())

			// Should proceed past validation to WorkflowFailed (not found), not ComponentValidationFailed
			cond := meta.FindStatusCondition(resource.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal(string(ReasonWorkflowFailed)))
		})
	})

	// ---------------------------------------------------------------------------
	// Finalization: no resources in status, Workflow gone
	// ---------------------------------------------------------------------------

	Context("Finalization when Workflow is gone and no resources to clean", func() {
		const resourceName = "int-test-finalize-no-resources"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		It("should remove finalizer and allow deletion", func() {
			By("Creating a WorkflowRun with finalizer")
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "gone-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Deleting the WorkflowRun")
			Expect(k8sClient.Delete(ctx, wfr)).To(Succeed())

			By("Verifying it's marked for deletion but still exists")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(resource.DeletionTimestamp.IsZero()).To(BeFalse())

			By("Reconciling to finalize")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			By("Verifying the resource is gone")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreodevv1alpha1.WorkflowRun{})
				return err != nil
			}, "5s", "250ms").Should(BeTrue())
		})
	})

	// ---------------------------------------------------------------------------
	// TTL expiration triggers deletion (integration)
	// ---------------------------------------------------------------------------

	Context("TTL expiration triggers deletion", func() {
		const resourceName = "int-test-ttl-expire"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		It("should delete the WorkflowRun when TTL is expired", func() {
			By("Creating a WorkflowRun with expired TTL")
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow:           openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-workflow"},
					TTLAfterCompletion: "0s",
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Setting CompletedAt in the past via status update")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			now := metav1.Now()
			resource.Status.CompletedAt = &now
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())

			By("Reconciling to trigger TTL deletion")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the resource is marked for deletion")
			Eventually(func() bool {
				resource := &openchoreodevv1alpha1.WorkflowRun{}
				if err := k8sClient.Get(ctx, nn, resource); err != nil {
					return true // already gone
				}
				return !resource.DeletionTimestamp.IsZero()
			}, "5s", "250ms").Should(BeTrue())

			By("Reconciling finalization to complete deletion")
			Eventually(func() bool {
				_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				if err != nil {
					return false
				}
				resource := &openchoreodevv1alpha1.WorkflowRun{}
				return k8sClient.Get(ctx, nn, resource) != nil
			}, "5s", "250ms").Should(BeTrue())
		})
	})

	// ---------------------------------------------------------------------------
	// Completed workflow returns early without further processing
	// ---------------------------------------------------------------------------

	Context("Completed workflow returns early", func() {
		const resourceName = "int-test-completed-early-exit"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should set CompletedAt and return without requeue", func() {
			By("Creating a WorkflowRun with succeeded condition (simulating completed workflow)")
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Setting WorkflowCompleted=True via status update")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			meta.SetStatusCondition(&resource.Status.Conditions, metav1.Condition{
				Type:               string(ConditionWorkflowCompleted),
				Status:             metav1.ConditionTrue,
				Reason:             string(ReasonWorkflowSucceeded),
				Message:            "Workflow has completed successfully",
				ObservedGeneration: resource.Generation,
			})
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())

			By("Reconciling — should set CompletedAt and return")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			By("Verifying CompletedAt was set")
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(resource.Status.CompletedAt).NotTo(BeNil())
		})
	})

	// ---------------------------------------------------------------------------
	// Multi-cycle reconciliation: from creation to pending to workflow-not-found
	// ---------------------------------------------------------------------------

	Context("Multi-cycle reconciliation lifecycle", func() {
		const resourceName = "int-test-multi-cycle"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should progress through states across reconcile cycles", func() {
			By("Creating a WorkflowRun without finalizer")
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "nonexistent-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			By("Cycle 1: adds finalizer")
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(resource.Finalizers).To(ContainElement(WorkflowRunCleanupFinalizer))
			Expect(resource.Status.Conditions).To(BeEmpty())

			By("Cycle 2: sets pending condition and StartedAt")
			result, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(resource.Status.StartedAt).NotTo(BeNil())
			cond := meta.FindStatusCondition(resource.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonWorkflowPending)))

			By("Cycle 3: workflow not found — sets completed condition")
			result, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			cond = meta.FindStatusCondition(resource.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonWorkflowFailed)))

			By("Cycle 4: completed workflow sets CompletedAt and returns early")
			result, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(resource.Status.CompletedAt).NotTo(BeNil())
		})
	})

	// ---------------------------------------------------------------------------
	// Namespace-scoped Workflow resolution followed by workflow-plane-not-found
	// ---------------------------------------------------------------------------

	Context("Workflow resolution with namespace-scoped Workflow kind and workflow-plane-not-found", func() {
		const (
			resourceName = "int-test-ns-workflow"
			workflowName = "int-ns-wf"
		)
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			By("Creating a namespace-scoped Workflow")
			runTemplate := map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"metadata":   map[string]any{"name": "test", "namespace": "default"},
				"spec":       map[string]any{"entrypoint": "main"},
			}
			runTemplateJSON, err := json.Marshal(runTemplate)
			Expect(err).NotTo(HaveOccurred())

			workflow := &openchoreodevv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{Name: workflowName, Namespace: "default"},
				Spec: openchoreodevv1alpha1.WorkflowSpec{
					RunTemplate: &runtime.RawExtension{Raw: runTemplateJSON},
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
			wf := &openchoreodevv1alpha1.Workflow{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: workflowName, Namespace: "default"}, wf); err == nil {
				_ = k8sClient.Delete(ctx, wf)
			}
		})

		It("should resolve namespace-scoped Workflow and proceed to workflow plane check", func() {
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{
						Kind: openchoreodevv1alpha1.WorkflowRefKindWorkflow,
						Name: workflowName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			By("First reconcile: sets pending")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("Second reconcile: resolves Workflow, then no WorkflowPlane → WorkflowPlaneNotFound")
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Minute))

			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			cond := meta.FindStatusCondition(resource.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal(string(ReasonWorkflowPlaneNotFound)))
		})
	})

	// ---------------------------------------------------------------------------
	// Conditions persist across status updates
	// ---------------------------------------------------------------------------

	Context("Status conditions persist across reconciles", func() {
		const resourceName = "int-test-conditions-persist"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should persist conditions set by the reconciler", func() {
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			By("Setting pending condition")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("Re-fetching and verifying condition survives")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			cond := meta.FindStatusCondition(resource.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonWorkflowPending)))

			By("Reconciling again to progress past pending — workflow not found")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the condition was updated (not duplicated)")
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			completedConditions := 0
			for _, c := range resource.Status.Conditions {
				if c.Type == string(ConditionWorkflowCompleted) {
					completedConditions++
				}
			}
			Expect(completedConditions).To(Equal(1), "expected exactly one WorkflowCompleted condition")
		})
	})

	// ---------------------------------------------------------------------------
	// Component validation with ClusterComponentType
	// ---------------------------------------------------------------------------

	Context("Component validation with ClusterComponentType", func() {
		const (
			resourceName = "int-test-cct-validation"
			cctName      = "int-val-cct"
			compName     = "int-val-cct-comp"
			workflowName = "int-val-cct-wf"
		)
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		BeforeEach(func() {
			By("Creating a ClusterComponentType with allowedWorkflows")
			cct := &openchoreodevv1alpha1.ClusterComponentType{
				ObjectMeta: metav1.ObjectMeta{Name: cctName},
				Spec: openchoreodevv1alpha1.ClusterComponentTypeSpec{
					WorkloadType: "deployment",
					AllowedWorkflows: []openchoreodevv1alpha1.ClusterWorkflowRef{
						{Kind: openchoreodevv1alpha1.ClusterWorkflowRefKindClusterWorkflow, Name: workflowName},
					},
					Resources: []openchoreodevv1alpha1.ResourceTemplate{
						{ID: "deployment", Template: &runtime.RawExtension{Raw: []byte("{}")}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cct)).To(Succeed())

			By("Creating Component referencing ClusterComponentType")
			comp := &openchoreodevv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: compName, Namespace: "default"},
				Spec: openchoreodevv1alpha1.ComponentSpec{
					Owner: openchoreodevv1alpha1.ComponentOwner{ProjectName: "my-proj"},
					ComponentType: openchoreodevv1alpha1.ComponentTypeRef{
						Kind: openchoreodevv1alpha1.ComponentTypeRefKindClusterComponentType,
						Name: "deployment/" + cctName,
					},
					Workflow: &openchoreodevv1alpha1.ComponentWorkflowConfig{
						Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow,
						Name: workflowName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
			_ = k8sClient.Delete(ctx, &openchoreodevv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: compName, Namespace: "default"},
			})
			_ = k8sClient.Delete(ctx, &openchoreodevv1alpha1.ClusterComponentType{
				ObjectMeta: metav1.ObjectMeta{Name: cctName},
			})
		})

		It("should pass validation for ClusterComponentType and proceed to workflow resolution", func() {
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
					Labels: map[string]string{
						"openchoreo.dev/project":   "my-proj",
						"openchoreo.dev/component": compName,
					},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{
						Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow,
						Name: workflowName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			By("First reconcile: sets pending")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("Second reconcile: validates against ClusterComponentType — should pass and fail on workflow resolution")
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			cond := meta.FindStatusCondition(resource.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(cond).NotTo(BeNil())
			// Should be WorkflowFailed (not found) — NOT ComponentValidationFailed
			Expect(cond.Reason).To(Equal(string(ReasonWorkflowFailed)))
		})
	})

	// ---------------------------------------------------------------------------
	// Status Tasks field persistence
	// ---------------------------------------------------------------------------

	Context("Status Tasks field persistence", func() {
		const resourceName = "int-test-tasks-persist"
		nn := types.NamespacedName{Name: resourceName, Namespace: "default"}

		AfterEach(func() { forceDelete(ctx, nn) })

		It("should persist Tasks in status subresource", func() {
			wfr := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-workflow"},
				},
			}
			Expect(k8sClient.Create(ctx, wfr)).To(Succeed())

			By("Updating status with Tasks")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())

			now := metav1.Now()
			resource.Status.Tasks = []openchoreodevv1alpha1.WorkflowTask{
				{Name: "build", Phase: "Succeeded", StartedAt: &now, CompletedAt: &now},
				{Name: "deploy", Phase: "Running", StartedAt: &now},
			}
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())

			By("Verifying Tasks were persisted")
			Expect(k8sClient.Get(ctx, nn, resource)).To(Succeed())
			Expect(resource.Status.Tasks).To(HaveLen(2))
			Expect(resource.Status.Tasks[0].Name).To(Equal("build"))
			Expect(resource.Status.Tasks[0].Phase).To(Equal("Succeeded"))
			Expect(resource.Status.Tasks[1].Name).To(Equal("deploy"))
			Expect(resource.Status.Tasks[1].Phase).To(Equal("Running"))
		})
	})
})
