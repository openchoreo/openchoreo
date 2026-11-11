// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

func newProject(name string) *openchoreov1alpha1.Project {
	return &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
			Type:                  openchoreov1alpha1.ProjectTypeRef{Name: "default"},
		},
	}
}

var _ = Describe("AutoDeploy", func() {
	adCtx := context.Background()

	newPipeline := func() *openchoreov1alpha1.DeploymentPipeline {
		return &openchoreov1alpha1.DeploymentPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
			Spec: openchoreov1alpha1.DeploymentPipelineSpec{
				PromotionPaths: []openchoreov1alpha1.PromotionPath{{
					SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: "dev"},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: "staging"}},
				}},
			},
		}
	}

	newAutoDeployEndpoint := func(autoDeploy bool) *openchoreov1alpha1.Endpoint {
		ep := newEndpoint("ep-auto", "p-auto", clusterTypeRef("cet-auto"),
			componentBackend("greeter", "greeter", "http", 1))
		ep.Spec.AutoDeploy = autoDeploy
		return ep
	}

	buildClient := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(objs...).
			WithStatusSubresource(&openchoreov1alpha1.Endpoint{}).
			WithIndex(&openchoreov1alpha1.EndpointReleaseBinding{},
				controller.IndexKeyEndpointReleaseBindingOwnerEndpointName,
				controller.IndexEndpointReleaseBindingOwner).
			Build()
	}

	// settle reconciles until the Endpoint stops changing: the first passes set
	// owner references, later ones cut the release and deploy it.
	settle := func(cli client.Client, ep *openchoreov1alpha1.Endpoint) *openchoreov1alpha1.Endpoint {
		r := &Reconciler{Client: cli, Scheme: scheme.Scheme}
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(ep)}
		for range 3 {
			_, err := r.Reconcile(adCtx, req)
			Expect(err).NotTo(HaveOccurred())
		}
		updated := &openchoreov1alpha1.Endpoint{}
		Expect(cli.Get(adCtx, req.NamespacedName, updated)).To(Succeed())
		return updated
	}

	bindings := func(cli client.Client) []openchoreov1alpha1.EndpointReleaseBinding {
		var list openchoreov1alpha1.EndpointReleaseBindingList
		Expect(cli.List(adCtx, &list, client.InNamespace("default"))).To(Succeed())
		return list.Items
	}

	It("deploys the latest release to the first environment and follows new releases", func() {
		ep := newAutoDeployEndpoint(true)
		cli := buildClient(newClusterEndpointType("cet-auto", ""), newWorkload("p-auto", "greeter", "http"),
			newProject("p-auto"), newPipeline(), ep)

		first := settle(cli, ep)
		Expect(first.Status.LatestRelease).NotTo(BeNil())
		got := bindings(cli)
		Expect(got).To(HaveLen(1))
		Expect(got[0].Name).To(Equal("ep-auto-dev"))
		Expect(got[0].Spec.Environment).To(Equal("dev"))
		Expect(got[0].Spec.Owner).To(Equal(openchoreov1alpha1.EndpointReleaseBindingOwner{
			ProjectName: "p-auto", EndpointName: ep.Name,
		}))
		Expect(got[0].Spec.EndpointRelease).To(Equal(first.Status.LatestRelease.Name))

		first.Spec.BackendRefs[0].Weight = ptr.To[int32](7)
		Expect(cli.Update(adCtx, first)).To(Succeed())

		second := settle(cli, ep)
		Expect(second.Status.LatestRelease.Name).NotTo(Equal(first.Status.LatestRelease.Name))
		got = bindings(cli)
		Expect(got).To(HaveLen(1))
		Expect(got[0].Spec.EndpointRelease).To(Equal(second.Status.LatestRelease.Name))
	})

	It("creates no binding without autoDeploy", func() {
		ep := newAutoDeployEndpoint(false)
		cli := buildClient(newClusterEndpointType("cet-auto", ""), newWorkload("p-auto", "greeter", "http"),
			newProject("p-auto"), newPipeline(), ep)

		Expect(settle(cli, ep).Status.LatestRelease).NotTo(BeNil())
		Expect(bindings(cli)).To(BeEmpty())
	})

	It("reports a missing deployment pipeline without failing the release", func() {
		ep := newAutoDeployEndpoint(true)
		cli := buildClient(newClusterEndpointType("cet-auto", ""), newWorkload("p-auto", "greeter", "http"),
			newProject("p-auto"), ep)

		updated := settle(cli, ep)
		Expect(updated.Status.LatestRelease).NotTo(BeNil())
		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionReady))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonAutoDeployFailed)))
		Expect(cond.Message).To(ContainSubstring(`DeploymentPipeline "default" not found`))
		Expect(bindings(cli)).To(BeEmpty())
	})
})
