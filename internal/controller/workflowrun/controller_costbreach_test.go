// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sMocks "github.com/openchoreo/openchoreo/internal/clients/kubernetes/mocks"
	workflowpipeline "github.com/openchoreo/openchoreo/internal/pipeline/workflow"
)

// hostileExpr costs far more than any per-expression limit: 2,000 outer
// iterations each building a 2,000-element list. The engine's cost guard stops
// it, which is exactly the terminal failure these tests drive.
const hostileExpr = "${size(lists.range(2000).map(x, lists.range(2000).map(y, x + y)))}"

// costLimitSentinel is the sentinel text of ErrCostLimitExceeded. Asserting on it
// rather than a loose "cost" substring keeps the specs pinned to the per-expression
// limit they actually trip.
const costLimitSentinel = "cel expression cost limit exceeded"

// Before this change a render failure was logged and requeued with no condition, so a
// WorkflowRun whose template trips the CEL cost guard told an operator nothing. These
// specs pin the replacement: WorkflowCompleted=False/WorkflowRenderingFailed carrying
// the cost message, from either render entry point.
var _ = Describe("WorkflowRun controller — CEL cost breach", func() {
	ctx := context.Background()

	const (
		workflowName = "cost-breach-wf"
		runName      = "cost-breach-wfr"
	)
	nn := types.NamespacedName{Name: runName, Namespace: "default"}

	// runTemplate builds a minimal Argo Workflow body. The runaway variant
	// hides the hostile expression in a label so rendering trips the guard.
	runTemplate := func(runaway bool) *runtime.RawExtension {
		metadata := map[string]any{
			"name":      "${metadata.workflowRunName}",
			"namespace": "${metadata.namespace}",
		}
		if runaway {
			metadata["labels"] = map[string]any{"runaway": hostileExpr}
		}
		raw, err := json.Marshal(map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Workflow",
			"metadata":   metadata,
			"spec": map[string]any{
				"entrypoint":         "main",
				"serviceAccountName": "wf-sa",
				"templates": []any{map[string]any{
					"name":      "main",
					"container": map[string]any{"image": "alpine", "command": []any{"echo", "hi"}},
				}},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		return &runtime.RawExtension{Raw: raw}
	}

	// newReconciler wires the envtest control-plane client to a workflow-plane
	// client that is never written to: every spec here fails before submission.
	newReconciler := func() *Reconciler {
		provider := &k8sMocks.MockWorkflowPlaneClientProvider{}
		provider.EXPECT().ClusterWorkflowPlaneClient(mock.Anything).
			Return(fake.NewClientBuilder().WithScheme(k8sClient.Scheme()).Build(), nil)
		return &Reconciler{
			Client:              k8sClient,
			Scheme:              k8sClient.Scheme(),
			PlaneClientProvider: provider,
			Pipeline:            workflowpipeline.NewPipeline(),
		}
	}

	// createWorkflow creates the ClusterWorkflow the run points at.
	createWorkflow := func(externalRefs []openchoreodevv1alpha1.ExternalRef, runaway bool) {
		cwf := &openchoreodevv1alpha1.ClusterWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: workflowName},
			Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
				RunTemplate:  runTemplate(runaway),
				ExternalRefs: externalRefs,
			},
		}
		Expect(k8sClient.Create(ctx, cwf)).To(Succeed())
		DeferCleanup(func() { forceDeleteClusterWorkflow(ctx, workflowName) })
	}

	// createRun creates the WorkflowRun with the cleanup finalizer and the
	// pending condition already recorded, so the next reconcile goes straight
	// to workflow resolution and rendering.
	createRun := func() {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:       runName,
				Namespace:  "default",
				Finalizers: []string{WorkflowRunCleanupFinalizer},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: workflowName},
			},
		}
		Expect(k8sClient.Create(ctx, wfr)).To(Succeed())
		DeferCleanup(func() { forceDelete(ctx, nn) })

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
	}

	// The workflow plane is resolved before rendering; with no ref on the
	// workflow the reconciler looks for the "default" ClusterWorkflowPlane.
	BeforeEach(func() {
		cwp := &openchoreodevv1alpha1.ClusterWorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec: openchoreodevv1alpha1.ClusterWorkflowPlaneSpec{
				PlaneID: "cost-breach-plane",
				ClusterAgent: openchoreodevv1alpha1.ClusterAgentConfig{
					ClientCA: openchoreodevv1alpha1.ValueFrom{Value: "test-ca"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cwp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cwp) })
	})

	fetchRun := func() *openchoreodevv1alpha1.WorkflowRun {
		wfr := &openchoreodevv1alpha1.WorkflowRun{}
		Expect(k8sClient.Get(ctx, nn, wfr)).To(Succeed())
		return wfr
	}

	expectRenderingFailed := func(wfr *openchoreodevv1alpha1.WorkflowRun) {
		cond := meta.FindStatusCondition(wfr.Status.Conditions, string(ConditionWorkflowCompleted))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonWorkflowRenderingFailed)))
		Expect(cond.Message).To(ContainSubstring(costLimitSentinel))
	}

	It("records WorkflowRenderingFailed when the run template breaches the cost guard", func() {
		createWorkflow(nil, true)
		createRun()

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		expectRenderingFailed(fetchRun())
	})

	It("records WorkflowRenderingFailed when an externalRef name breaches the cost guard", func() {
		createWorkflow([]openchoreodevv1alpha1.ExternalRef{{
			ID:         "git-creds",
			APIVersion: "openchoreo.dev/v1alpha1",
			Kind:       "SecretReference",
			Name:       hostileExpr,
		}}, false)
		createRun()

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		expectRenderingFailed(fetchRun())
	})

	// Recovery for an UNWATCHED input: the reconciler watches WorkflowRun, not the
	// (Cluster)Workflow whose spec it renders. Editing the workflow fires no event, so
	// the requeue the reconciler already does is what picks the fix up.
	It("recovers once the unwatched ClusterWorkflow spec is fixed", func() {
		createWorkflow(nil, true)
		createRun()

		_, err := newReconciler().Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		expectRenderingFailed(fetchRun())

		By("Fixing the ClusterWorkflow spec without touching the WorkflowRun")
		cwf := &openchoreodevv1alpha1.ClusterWorkflow{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: workflowName}, cwf)).To(Succeed())
		cwf.Spec.RunTemplate = runTemplate(false)
		Expect(k8sClient.Update(ctx, cwf)).To(Succeed())

		By("Running the reconcile the requeue would trigger")
		_, err = newReconciler().Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		// The run is submitted, which is what recovery means here. The
		// WorkflowCompleted reason still reads WorkflowRenderingFailed until
		// the next status sync overwrites it — the same way the existing
		// resolution-failure reasons linger after their cause clears.
		Expect(fetchRun().Status.RunReference).NotTo(BeNil())
	})
})
