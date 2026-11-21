// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/projectreleasebinding"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/releaseline"
)

var _ = Describe("ReleaseBinding controller — namespace from the ProjectReleaseBinding", func() {
	const (
		project = "ns-project"
		env     = "ns-env"
		dpName  = "ns-dp"
	)

	// namespacedTemplate renders a Deployment into metadata.namespace, so the
	// release shows which namespace the binding rendered into.
	namespacedTemplate := &runtime.RawExtension{Raw: []byte(`{` +
		`"apiVersion":"apps/v1",` +
		`"kind":"Deployment",` +
		`"metadata":{"name":"api","namespace":"${metadata.namespace}"},` +
		`"spec":{` +
		`"selector":{"matchLabels":{"app":"api"}},` +
		`"template":{` +
		`"metadata":{"labels":{"app":"api"}},` +
		`"spec":{"containers":[{"name":"app","image":"nginx:latest"}]}` +
		`}}}`)}

	BeforeEach(func() {
		dp := dpFixture(dpName)
		Expect(k8sClient.Create(ctx, dp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, dp) })
		environment := envFixture(env, dpName)
		Expect(k8sClient.Create(ctx, environment)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, environment) })
		proj := projectFixture(project)
		Expect(k8sClient.Create(ctx, proj)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, proj) })
	})

	// makeComponent creates a Component, its ComponentRelease and a managed
	// ReleaseBinding for it, and returns the binding.
	makeComponent := func(name string) *openchoreov1alpha1.ReleaseBinding {
		comp := componentFixture(name, project)
		Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, comp) })

		cr := crFixture(name+"-release", project, name)
		cr.Spec.ComponentType.Spec.Resources = []openchoreov1alpha1.ResourceTemplate{
			{ID: "deployment", Template: namespacedTemplate},
		}
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })

		rb := rbFixture(name, project, name, env, cr.Name, true)
		Expect(k8sClient.Create(ctx, rb)).To(Succeed())
		DeferCleanup(func() {
			forceDelete(name)
			forceDeleteRelease(releaseline.RenderedReleaseName(name, "", env))
			forceDeleteRelease(releaseline.RenderedReleaseName(name, "", env) + "-observability")
		})
		return rb
	}

	// makePRB creates a ProjectReleaseBinding for the project and environment.
	// A non-empty reported is written to its status.namespace.
	makePRB := func(name string, mode openchoreov1alpha1.BindingMode, reported string) {
		prb := &openchoreov1alpha1.ProjectReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: project},
				Environment: env,
				Mode:        mode,
			},
		}
		if mode == openchoreov1alpha1.BindingModeObserve {
			prb.Spec.Observe = &openchoreov1alpha1.ProjectReleaseBindingObserve{Namespace: "team-ns"}
		}
		Expect(k8sClient.Create(ctx, prb)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, prb) })
		if reported != "" {
			prb.Status.Namespace = reported
			Expect(k8sClient.Status().Update(ctx, prb)).To(Succeed())
		}
	}

	releaseOf := func(rb *openchoreov1alpha1.ReleaseBinding, suffix string) (*openchoreov1alpha1.RenderedRelease, error) {
		rr := &openchoreov1alpha1.RenderedRelease{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      releaseline.RenderedReleaseName(rb.Spec.Owner.ComponentName, "", env) + suffix,
		}, rr)
		return rr, err
	}

	// renderedNamespace returns the namespace of the release's one Deployment.
	renderedNamespace := func(rb *openchoreov1alpha1.ReleaseBinding) string {
		GinkgoHelper()
		rr, err := releaseOf(rb, "")
		Expect(err).NotTo(HaveOccurred())
		for _, res := range rr.Spec.Resources {
			obj := map[string]any{}
			Expect(json.Unmarshal(res.Object.Raw, &obj)).To(Succeed())
			if obj["kind"] == kindDeployment {
				return obj["metadata"].(map[string]any)["namespace"].(string)
			}
		}
		Fail("the release has no Deployment")
		return ""
	}

	expectHeld := func(rb *openchoreov1alpha1.ReleaseBinding, reason string) {
		GinkgoHelper()
		got := fetchRB(rb.Name)
		for _, condType := range []controller.ConditionType{ConditionReleaseSynced, ConditionResourcesReady} {
			cond := conditionFor(got, string(condType))
			Expect(cond).NotTo(BeNil(), condType)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse), condType)
			Expect(cond.Reason).To(Equal(reason), condType)
		}
		for _, suffix := range []string{"", "-observability"} {
			_, err := releaseOf(rb, suffix)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "no release%s should be created", suffix)
		}
	}

	It("renders into the generated dp-* namespace without a ProjectReleaseBinding", func() {
		rb := makeComponent("ns-none")
		mustReconcile(testReconcilerWithPipeline(), reconcileRequest(rb.Name))
		Expect(renderedNamespace(rb)).To(Equal(dpkubernetes.DataPlaneNamespace(ns, project, env)))
	})

	It("renders into the namespace a managed ProjectReleaseBinding reports", func() {
		makePRB("ns-managed", openchoreov1alpha1.BindingModeManage, "team-runtime")
		rb := makeComponent("ns-managed")
		mustReconcile(testReconcilerWithPipeline(), reconcileRequest(rb.Name))
		Expect(renderedNamespace(rb)).To(Equal("team-runtime"))
	})

	It("waits while a managed ProjectReleaseBinding has not reported its namespace", func() {
		makePRB("ns-pending", openchoreov1alpha1.BindingModeManage, "")
		rb := makeComponent("ns-pending")
		mustReconcile(testReconcilerWithPipeline(), reconcileRequest(rb.Name))
		expectHeld(rb, string(projectreleasebinding.ReasonProjectNamespacePending))
	})

	It("renders nothing under an observed ProjectReleaseBinding", func() {
		makePRB("ns-observed", openchoreov1alpha1.BindingModeObserve, "team-ns")
		rb := makeComponent("ns-observed")
		mustReconcile(testReconcilerWithPipeline(), reconcileRequest(rb.Name))
		expectHeld(rb, string(projectreleasebinding.ReasonProjectObserved))
	})

	It("leaves an existing release alone under an observed ProjectReleaseBinding", func() {
		rb := makeComponent("ns-existing")
		r := testReconcilerWithPipeline()
		mustReconcile(r, reconcileRequest(rb.Name))
		before, err := releaseOf(rb, "")
		Expect(err).NotTo(HaveOccurred())

		makePRB("ns-existing", openchoreov1alpha1.BindingModeObserve, "team-ns")
		mustReconcile(r, reconcileRequest(rb.Name))

		after, err := releaseOf(rb, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(after.Generation).To(Equal(before.Generation))
		Expect(after.DeletionTimestamp).To(BeNil())
		cond := conditionFor(fetchRB(rb.Name), string(ConditionReleaseSynced))
		Expect(cond.Reason).To(Equal(string(projectreleasebinding.ReasonProjectObserved)))
	})

	It("lets Undeploy remove an existing release under an observed ProjectReleaseBinding", func() {
		rb := makeComponent("ns-undeploy")
		r := testReconcilerWithPipeline()
		mustReconcile(r, reconcileRequest(rb.Name))
		_, err := releaseOf(rb, "")
		Expect(err).NotTo(HaveOccurred())

		makePRB("ns-undeploy", openchoreov1alpha1.BindingModeObserve, "team-ns")
		current := fetchRB(rb.Name)
		current.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
		Expect(k8sClient.Update(ctx, current)).To(Succeed())
		mustReconcile(r, reconcileRequest(rb.Name))

		rr, err := releaseOf(rb, "")
		Expect(apierrors.IsNotFound(err) || !rr.DeletionTimestamp.IsZero()).To(BeTrue(),
			"Undeploy deletes the release whatever the ProjectReleaseBinding")
		cond := conditionFor(fetchRB(rb.Name), string(ConditionReleaseSynced))
		Expect(cond.Reason).NotTo(Equal(string(projectreleasebinding.ReasonProjectObserved)))
	})

	It("reports a conflict with several ProjectReleaseBindings", func() {
		makePRB("ns-several-a", openchoreov1alpha1.BindingModeManage, "team-runtime")
		makePRB("ns-several-b", openchoreov1alpha1.BindingModeManage, "team-runtime")
		rb := makeComponent("ns-several")
		mustReconcile(testReconcilerWithPipeline(), reconcileRequest(rb.Name))
		expectHeld(rb, string(projectreleasebinding.ReasonProjectReleaseBindingConflict))
	})
})
