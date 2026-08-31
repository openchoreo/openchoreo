// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
)

// hostileExpr costs far more than any per-expression limit: 2,000 outer
// iterations each building a 2,000-element list. The engine's cost guard stops
// it, which is exactly the terminal failure these tests drive.
const hostileExpr = "${size(lists.range(2000).map(x, lists.range(2000).map(y, x + y)))}"

const (
	costLimitSentinel  = "cel expression cost limit exceeded"
	costBudgetSentinel = "template render cost budget exceeded"
)

// A CEL cost breach must be visible on the object rather than only in the logs: these
// specs pin that a breach surfaces on the right condition axis — Synced for manifests,
// OutputsResolved for outputs, ResourcesReady for readyWhen — carrying the guard's
// message, with no error returned.
var _ = Describe("ResourceReleaseBinding controller — CEL cost breach", func() {
	var reconciler *Reconciler

	BeforeEach(func() {
		reconciler = &Reconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Pipeline: resourcepipeline.NewPipeline(),
		}
	})

	cheapTemplate := func(name string) *runtime.RawExtension {
		return &runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"` + name + `"}}`),
		}
	}

	// hostileTemplate embeds the runaway expression in a manifest body, so the
	// breach happens during RenderManifests.
	hostileTemplate := func(name string) *runtime.RawExtension {
		return &runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"` + name +
				`"},"data":{"runaway":"` + hostileExpr + `"}}`),
		}
	}

	// budgetBurningTemplate breaches the reconcile's cumulative cost BUDGET
	// rather than the per-expression limit: 250 distinct expressions, each a
	// few thousand cost units — comfortably legal on its own — that together
	// outspend the budget. The expressions differ from one another on purpose,
	// so which one happens to tip the total over is a real question and the
	// message-determinism assertion below is not vacuous.
	budgetBurningTemplate := func(name string) *runtime.RawExtension {
		data := make(map[string]string, 250)
		for i := range 250 {
			data[fmt.Sprintf("field%03d", i)] = fmt.Sprintf("${size(lists.range(%d))}", 9750+i)
		}
		body := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": name},
			"data":       data,
		}
		raw, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		return &runtime.RawExtension{Raw: raw}
	}

	newRelease := func(name string, spec openchoreov1alpha1.ResourceTypeSpec) *openchoreov1alpha1.ResourceRelease {
		return &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ResourceReleaseSpec{
				Owner: openchoreov1alpha1.ResourceReleaseOwner{
					ProjectName:  "test-project",
					ResourceName: "test-resource",
				},
				ResourceType: openchoreov1alpha1.ResourceReleaseResourceType{
					Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
					Name: "mysql",
					Spec: spec,
				},
			},
		}
	}

	createRelease := func(release *openchoreov1alpha1.ResourceRelease) {
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })
	}

	makeBinding := func(name, releaseName, env string) *openchoreov1alpha1.ResourceReleaseBinding {
		b := &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				Finalizers: []string{ResourceReleaseBindingFinalizer},
			},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "test-project",
					ResourceName: "test-resource",
				},
				Environment:     env,
				ResourceRelease: releaseName,
			},
		}
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			// Best-effort cleanup of any emitted RenderedRelease. Specs whose breach
			// happens after manifest rendering (outputs, readyWhen) emit one, as does
			// the recovery spec once it re-pins to a cheap snapshot. Left behind, they
			// break sibling specs that count the RenderedReleases in the namespace.
			// Scoped to this binding's derived name rather than deleting everything in
			// the namespace, so cleanup cannot reach another spec's object.
			rr := &openchoreov1alpha1.RenderedRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      makeRenderedReleaseName(b),
					Namespace: b.Namespace,
				},
			}
			_ = k8sClient.Delete(ctx, rr)

			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(b), b)
			b.Finalizers = nil
			_ = k8sClient.Update(ctx, b)
			_ = k8sClient.Delete(ctx, b)
		})
		return b
	}

	makeEnv := func(prefix string) *openchoreov1alpha1.Environment {
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
		return env
	}

	reconcileBinding := func(b *openchoreov1alpha1.ResourceReleaseBinding) (reconcile.Result, *openchoreov1alpha1.ResourceReleaseBinding) {
		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(b),
		})
		Expect(err).NotTo(HaveOccurred())
		updated := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), updated)).To(Succeed())
		return result, updated
	}

	It("marks Synced=False with the cost message when manifest rendering breaches the cost guard", func() {
		env := makeEnv("cost-manifests")
		createRelease(newRelease("cost-manifests-release", openchoreov1alpha1.ResourceTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: hostileTemplate("cost-claim")},
			},
		}))
		binding := makeBinding("cost-manifests-binding", "cost-manifests-release", env.Name)

		_, updated := reconcileBinding(binding)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonRenderingFailed)))
		Expect(cond.Message).To(ContainSubstring(costLimitSentinel))
	})

	// A render the deadline aborted leaves nothing rendered either, so it must reach the
	// object the same way a cost breach does.
	It("marks Synced=False when the render deadline aborts manifest rendering", func() {
		// The deadline now lives on the pipeline, derived per call inside RenderManifests,
		// so it is set at construction rather than on the reconciler field.
		reconciler.Pipeline = resourcepipeline.NewPipeline(resourcepipeline.WithRenderTimeout(time.Nanosecond))

		env := makeEnv("deadline-manifests")
		createRelease(newRelease("deadline-manifests-release", openchoreov1alpha1.ResourceTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: cheapTemplate("deadline-claim")},
			},
		}))
		binding := makeBinding("deadline-manifests-binding", "deadline-manifests-release", env.Name)

		_, updated := reconcileBinding(binding)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonRenderingFailed)))
	})

	It("reports a breach in output resolution on the OutputsResolved condition", func() {
		env := makeEnv("cost-outputs")
		createRelease(newRelease("cost-outputs-release", openchoreov1alpha1.ResourceTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: cheapTemplate("cost-outputs-claim")},
			},
			Outputs: []openchoreov1alpha1.ResourceTypeOutput{
				{Name: "host", Value: hostileExpr},
			},
		}))
		binding := makeBinding("cost-outputs-binding", "cost-outputs-release", env.Name)

		_, updated := reconcileBinding(binding)

		// Manifest rendering succeeded, so Synced stays True; only the outputs
		// axis reports the breach.
		synced := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(synced).NotTo(BeNil())
		Expect(synced.Status).To(Equal(metav1.ConditionTrue))

		outputs := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionOutputsResolved))
		Expect(outputs).NotTo(BeNil())
		Expect(outputs.Status).To(Equal(metav1.ConditionFalse))
		Expect(outputs.Reason).To(Equal(string(ReasonOutputResolutionFailed)))
		Expect(outputs.Message).To(ContainSubstring(costLimitSentinel))
	})

	It("reports a breach in a readyWhen evaluation on the ResourcesReady condition", func() {
		env := makeEnv("cost-readywhen")
		createRelease(newRelease("cost-readywhen-release", openchoreov1alpha1.ResourceTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTypeManifest{
				{
					ID:        "claim",
					Template:  cheapTemplate("cost-readywhen-claim"),
					ReadyWhen: hostileExpr,
				},
			},
		}))
		binding := makeBinding("cost-readywhen-binding", "cost-readywhen-release", env.Name)

		_, updated := reconcileBinding(binding)

		ready := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionResourcesReady))
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Message).To(ContainSubstring(costLimitSentinel))
	})

	// The condition message must be byte-identical across reconciles of an unchanged
	// object. If it drifts, the status write it provokes is itself a watch event that
	// re-triggers the reconcile immediately, so a failing object reconciles in a tight
	// loop. A budget breach is the case that used to drift, twice over: the message
	// embedded the live spend, and the expression it named followed the render walk's
	// randomized map order.
	It("produces a byte-identical condition message on every reconcile of a budget breach", func() {
		env := makeEnv("cost-budget")
		createRelease(newRelease("cost-budget-release", openchoreov1alpha1.ResourceTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: budgetBurningTemplate("cost-budget-claim")},
			},
		}))
		binding := makeBinding("cost-budget-binding", "cost-budget-release", env.Name)

		_, updated := reconcileBinding(binding)

		first := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(first).NotTo(BeNil())
		Expect(first.Status).To(Equal(metav1.ConditionFalse))
		Expect(first.Reason).To(Equal(string(ReasonRenderingFailed)))
		Expect(first.Message).To(ContainSubstring(costBudgetSentinel),
			"the fixture must exhaust the cumulative budget, not trip the per-expression limit")

		for range 5 {
			_, updated = reconcileBinding(binding)
			again := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
			Expect(again).NotTo(BeNil())
			Expect(again.Message).To(Equal(first.Message))
			// A drifting message would also bump LastTransitionTime's sibling
			// bookkeeping; the message equality above is the load-bearing check.
		}
	})

	// Recovery: a ResourceRelease is immutable, so the fix is to re-pin the binding at a
	// cheap snapshot, which re-enqueues it through the normal watch.
	It("recovers on the next reconcile once the binding is re-pinned to a cheap snapshot", func() {
		env := makeEnv("cost-recovery")
		createRelease(newRelease("cost-recovery-bad", openchoreov1alpha1.ResourceTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: hostileTemplate("cost-recovery-claim")},
			},
		}))
		createRelease(newRelease("cost-recovery-good", openchoreov1alpha1.ResourceTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: cheapTemplate("cost-recovery-claim")},
			},
		}))
		binding := makeBinding("cost-recovery-binding", "cost-recovery-bad", env.Name)

		_, breached := reconcileBinding(binding)
		Expect(meta.IsStatusConditionFalse(breached.Status.Conditions, string(ConditionSynced))).To(BeTrue())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
		binding.Spec.ResourceRelease = "cost-recovery-good"
		Expect(k8sClient.Update(ctx, binding)).To(Succeed())

		_, updated := reconcileBinding(binding)

		synced := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(synced).NotTo(BeNil())
		Expect(synced.Status).To(Equal(metav1.ConditionTrue))
	})
})
