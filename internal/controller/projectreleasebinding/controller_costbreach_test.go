// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	projectpipeline "github.com/openchoreo/openchoreo/internal/pipeline/project"
)

// hostileExpr costs far more than any per-expression limit: 2,000 outer
// iterations each building a 2,000-element list. The engine's cost guard stops
// it, which is exactly the terminal failure these tests drive.
const hostileExpr = "${size(lists.range(2000).map(x, lists.range(2000).map(y, x + y)))}"

// costLimitSentinel is the sentinel text of ErrCostLimitExceeded. Asserting on it
// rather than a loose "cost" substring keeps the specs pinned to the per-expression
// limit they actually trip.
const costLimitSentinel = "cel expression cost limit exceeded"

// A CEL cost breach must be visible on the object rather than only in the logs:
// these specs pin that a breach surfaces as Synced=False carrying the cost message,
// with no error returned, and that a cheap snapshot recovers on the next reconcile.
var _ = Describe("ProjectReleaseBinding controller — CEL cost breach", func() {
	var reconciler *Reconciler

	BeforeEach(func() {
		reconciler = &Reconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Pipeline: projectpipeline.NewPipeline(),
		}
	})

	// namespaceTemplate renders the cell Namespace the project pipeline
	// mandates. The runaway variant hides the hostile expression in a label so
	// the breach happens inside Render.
	namespaceTemplate := func(runaway bool) *runtime.RawExtension {
		body := `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"${metadata.namespace}"`
		if runaway {
			body += `,"labels":{"runaway":"` + hostileExpr + `"}`
		}
		return &runtime.RawExtension{Raw: []byte(body + `}}`)}
	}

	createRelease := func(name string, runaway bool) {
		release := &openchoreov1alpha1.ProjectRelease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ProjectReleaseSpec{
				Owner: openchoreov1alpha1.ProjectReleaseOwner{ProjectName: "cost-project"},
				ProjectType: openchoreov1alpha1.ProjectReleaseProjectType{
					Kind: openchoreov1alpha1.ProjectTypeRefKindProjectType,
					Name: "default",
					Spec: openchoreov1alpha1.ProjectTypeSpec{
						Resources: []openchoreov1alpha1.ResourceTemplate{
							{ID: "cell-namespace", Template: namespaceTemplate(runaway)},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })
	}

	// makeSupportingResources creates the Project, DataPlane, and Environment
	// the reconcile chain resolves before it renders.
	makeSupportingResources := func(prefix string) string {
		project := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: "cost-project", Namespace: "default"},
			Spec: openchoreov1alpha1.ProjectSpec{
				DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
				Type:                  openchoreov1alpha1.ProjectTypeRef{Name: "default"},
			},
		}
		if err := k8sClient.Create(ctx, project); err != nil {
			// Shared across specs in this Describe; the first creator wins.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(project), project)).To(Succeed())
		}

		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: prefix + "-dp", Namespace: "default"},
		}
		Expect(k8sClient.Create(ctx, dp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, dp) })

		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: prefix + "-env", Namespace: "default"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: dp.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, env)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, env) })
		return env.Name
	}

	makeBinding := func(name, releaseName, env string) *openchoreov1alpha1.ProjectReleaseBinding {
		b := &openchoreov1alpha1.ProjectReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				Finalizers: []string{ProjectReleaseBindingFinalizer},
			},
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner:          openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: "cost-project"},
				Environment:    env,
				ProjectRelease: releaseName,
			},
		}
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(b), b)
			b.Finalizers = nil
			_ = k8sClient.Update(ctx, b)
			_ = k8sClient.Delete(ctx, b)
		})
		return b
	}

	reconcileBinding := func(b *openchoreov1alpha1.ProjectReleaseBinding) (reconcile.Result, *openchoreov1alpha1.ProjectReleaseBinding) {
		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(b),
		})
		Expect(err).NotTo(HaveOccurred())
		updated := &openchoreov1alpha1.ProjectReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), updated)).To(Succeed())
		return result, updated
	}

	It("marks Synced=False with the cost message when rendering breaches the cost guard", func() {
		envName := makeSupportingResources("cost-render")
		createRelease("cost-render-release", true)
		binding := makeBinding("cost-render-binding", "cost-render-release", envName)

		_, updated := reconcileBinding(binding)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonRenderingFailed)))
		Expect(cond.Message).To(ContainSubstring(costLimitSentinel))
	})

	// A render the deadline aborted leaves nothing rendered either, so it must reach the
	// object the same way a cost breach does.
	It("marks Synced=False when the render deadline aborts rendering", func() {
		// The deadline now lives on the pipeline, derived per call inside Render, so it
		// is set at construction rather than on the reconciler field.
		reconciler.Pipeline = projectpipeline.NewPipeline(projectpipeline.WithRenderTimeout(time.Nanosecond))

		envName := makeSupportingResources("deadline-render")
		createRelease("deadline-render-release", false)
		binding := makeBinding("deadline-render-binding", "deadline-render-release", envName)

		_, updated := reconcileBinding(binding)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonRenderingFailed)))
	})

	// Recovery: a ProjectRelease is immutable, so the fix is to re-pin the
	// binding at a cheap snapshot, which re-enqueues it through the normal watch.
	It("recovers on the next reconcile once the binding is re-pinned to a cheap snapshot", func() {
		envName := makeSupportingResources("cost-recovery")
		createRelease("cost-recovery-bad", true)
		createRelease("cost-recovery-good", false)
		binding := makeBinding("cost-recovery-binding", "cost-recovery-bad", envName)

		_, breached := reconcileBinding(binding)
		Expect(meta.IsStatusConditionFalse(breached.Status.Conditions, string(ConditionSynced))).To(BeTrue())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
		binding.Spec.ProjectRelease = "cost-recovery-good"
		Expect(k8sClient.Update(ctx, binding)).To(Succeed())

		_, updated := reconcileBinding(binding)

		synced := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(synced).NotTo(BeNil())
		Expect(synced.Status).To(Equal(metav1.ConditionTrue))
	})
})
