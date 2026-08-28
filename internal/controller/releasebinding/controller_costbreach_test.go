// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	componentpipeline "github.com/openchoreo/openchoreo/internal/pipeline/component"
)

// hostileExpr costs far more than any per-expression limit: 2,000 outer
// iterations each building a 2,000-element list. The engine's cost guard stops
// it, which is exactly the terminal failure these tests drive.
const hostileExpr = "${size(lists.range(2000).map(x, lists.range(2000).map(y, x + y)))}"

// costLimitSentinel is the sentinel text of ErrCostLimitExceeded. Asserting on it
// rather than a loose "cost" substring keeps the specs pinned to the per-expression
// limit they actually trip.
const costLimitSentinel = "cel expression cost limit exceeded"

// overForEachCap is one item past the renderer's maxForEachItems fan-out cap. The cap is
// unexported, so the number is restated here; the spec below fails loudly if the two drift
// apart, because a forEach within the cap renders successfully.
const overForEachCap = 101

// runawayTemplate is minimalTemplate with the hostile expression hidden in a
// label, so rendering the resource trips the cost guard.
var runawayTemplate = &runtime.RawExtension{
	Raw: []byte(`{` +
		`"apiVersion":"apps/v1",` +
		`"kind":"Deployment",` +
		`"metadata":{"name":"test-deployment","labels":{"runaway":"` + hostileExpr + `"}},` +
		`"spec":{` +
		`"selector":{"matchLabels":{"app":"test"}},` +
		`"template":{` +
		`"metadata":{"labels":{"app":"test"}},` +
		`"spec":{"containers":[{"name":"app","image":"nginx:latest"}]}` +
		`}}}`,
	),
}

// A CEL cost breach must be visible on the object, not only in the logs: these specs
// pin that a breach surfaces as ReleaseSynced=False carrying the guard's message, and
// that re-pinning the binding at a cheap snapshot clears it.
var _ = Describe("ReleaseBinding controller — CEL cost breach", func() {
	const (
		rbName      = "rb-cost-breach"
		projectName = "cost-proj"
		compName    = "cost-comp"
		envName     = "cost-env"
		dpName      = "cost-dp"
	)

	// runawayCRFixture is crFixture with the runaway template swapped in.
	runawayCRFixture := func(name string) *openchoreov1alpha1.ComponentRelease {
		cr := crFixture(name, projectName, compName)
		cr.Spec.ComponentType.Spec.Resources = []openchoreov1alpha1.ResourceTemplate{
			{ID: "deployment", Template: runawayTemplate},
		}
		return cr
	}

	// createSupportingResources creates everything the reconcile chain resolves
	// before it renders: Environment, DataPlane, Component, Project.
	createSupportingResources := func() {
		Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, dpFixture(dpName)) })
		Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, envFixture(envName, dpName)) })
		Expect(k8sClient.Create(ctx, componentFixture(compName, projectName))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, componentFixture(compName, projectName)) })
		Expect(k8sClient.Create(ctx, projectFixture(projectName))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, projectFixture(projectName)) })
	}

	AfterEach(func() { forceDelete(rbName) })

	// reconcileExpectingRenderError reconciles and asserts the render failure reached the
	// caller as an error, which is what puts the binding on the reconciler's usual backoff.
	reconcileExpectingRenderError := func(r *Reconciler) {
		GinkgoHelper()
		_, err := r.Reconcile(ctx, reconcileRequest(rbName))
		Expect(err).To(HaveOccurred())
	}

	It("marks ReleaseSynced=False with the cost message when rendering breaches the cost guard", func() {
		createSupportingResources()
		cr := runawayCRFixture("cost-breach-release")
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })
		Expect(k8sClient.Create(ctx,
			rbFixture(rbName, projectName, compName, envName, cr.Name, true),
		)).To(Succeed())

		reconcileExpectingRenderError(testReconcilerWithPipeline())

		cond := conditionFor(fetchRB(rbName), string(ConditionReleaseSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonRenderingFailed)))
		Expect(cond.Message).To(ContainSubstring(costLimitSentinel))
	})

	// The forEach fan-out cap trips on the item count alone, before any sub-render runs.
	It("reports the fan-out cap when a forEach exceeds it", func() {
		createSupportingResources()

		cr := crFixture("foreach-breach-release", projectName, compName)
		// Append rather than replace: the ComponentRelease webhook requires the primary
		// resource whose id matches workloadType, and the fan-out is an extra resource
		// alongside it - which is also the realistic shape.
		cr.Spec.ComponentType.Spec.Resources = append(cr.Spec.ComponentType.Spec.Resources,
			openchoreov1alpha1.ResourceTemplate{
				ID:      "fanout",
				ForEach: fmt.Sprintf("${lists.range(%d)}", overForEachCap),
				Var:     "item",
				Template: &runtime.RawExtension{Raw: []byte(
					`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm-${item}"}}`)},
			})
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })
		Expect(k8sClient.Create(ctx,
			rbFixture(rbName, projectName, compName, envName, cr.Name, true),
		)).To(Succeed())

		reconcileExpectingRenderError(testReconcilerWithPipeline())

		cond := conditionFor(fetchRB(rbName), string(ConditionReleaseSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonRenderingFailed)))
		Expect(cond.Message).To(ContainSubstring("exceeding the limit of"))
	})

	// Recovery: a ComponentRelease is immutable, so the fix is to re-pin the
	// binding at a cheap snapshot, which re-enqueues it through the normal watch.
	It("recovers on the next reconcile once the binding is re-pinned to a cheap snapshot", func() {
		createSupportingResources()
		bad := runawayCRFixture("cost-recovery-bad")
		Expect(k8sClient.Create(ctx, bad)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bad) })
		good := crFixture("cost-recovery-good", projectName, compName)
		Expect(k8sClient.Create(ctx, good)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, good) })
		Expect(k8sClient.Create(ctx,
			rbFixture(rbName, projectName, compName, envName, bad.Name, true),
		)).To(Succeed())

		r := testReconcilerWithPipeline()
		reconcileExpectingRenderError(r)

		rb := fetchRB(rbName)
		rb.Spec.ReleaseName = good.Name
		Expect(k8sClient.Update(ctx, rb)).To(Succeed())

		mustReconcile(r, reconcileRequest(rbName))

		cond := conditionFor(fetchRB(rbName), string(ConditionReleaseSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Reason).NotTo(Equal(string(ReasonRenderingFailed)))
	})

	// An expired render deadline leaves nothing rendered either, so it must reach the
	// object the same way a cost breach does.
	It("marks ReleaseSynced=False when the render deadline expires", func() {
		createSupportingResources()
		cr := crFixture("deadline-release", projectName, compName)
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })
		Expect(k8sClient.Create(ctx,
			rbFixture(rbName, projectName, compName, envName, cr.Name, true),
		)).To(Succeed())

		// A one-nanosecond deadline has expired before the first expression is evaluated,
		// so the render aborts no matter how cheap the template is. The deadline lives on
		// the pipeline, derived per call inside Render, so it is configured at
		// construction rather than on the reconciler field.
		r := testReconcilerWithPipeline()
		r.Pipeline = componentpipeline.NewPipeline(componentpipeline.WithRenderTimeout(time.Nanosecond))

		reconcileExpectingRenderError(r)

		cond := conditionFor(fetchRB(rbName), string(ConditionReleaseSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonRenderingFailed)))
	})
})
