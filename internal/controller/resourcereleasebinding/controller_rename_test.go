// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sMocks "github.com/openchoreo/openchoreo/internal/clients/kubernetes/mocks"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
)

// These tests drive the binding and renderedrelease controllers together, with
// envtest as the data plane.
var _ = Describe("ResourceReleaseBinding with an object renamed under the same resource id", func() {
	const (
		prefix      = "rename"
		cmOld       = "rename-config-r1"
		cmNew       = "rename-config-r2"
		cmNamespace = "default"
	)

	var (
		bindingReconciler *Reconciler
		rrReconciler      *renderedrelease.Reconciler
	)

	BeforeEach(func() {
		bindingReconciler = &Reconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Pipeline: resourcepipeline.NewPipeline(),
		}
		planeClients := &k8sMocks.MockPlaneClientProvider{}
		planeClients.EXPECT().DataPlaneClient(mock.Anything).Return(k8sClient, nil).Maybe()
		rrReconciler = &renderedrelease.Reconciler{
			Client:              k8sClient,
			Scheme:              k8sClient.Scheme(),
			PlaneClientProvider: planeClients,
		}
	})

	configMapTemplate := &runtime.RawExtension{Raw: []byte(`{
		"apiVersion": "v1",
		"kind": "ConfigMap",
		"metadata": {"name": "rename-config-r${parameters.revision}", "namespace": "default"},
		"data": {"revision": "${string(parameters.revision)}"}
	}`)}

	makeRelease := func(name, revision string) *openchoreov1alpha1.ResourceRelease {
		release := &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ResourceReleaseSpec{
				Owner: openchoreov1alpha1.ResourceReleaseOwner{
					ProjectName:  "test-project",
					ResourceName: "test-resource",
				},
				ResourceType: openchoreov1alpha1.ResourceReleaseResourceType{
					Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
					Name: "versioned-config",
					Spec: openchoreov1alpha1.ResourceTypeSpec{
						Parameters: &openchoreov1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{
							Raw: []byte(`{"type":"object","properties":{"revision":{"type":"integer"}}}`),
						}},
						Resources: []openchoreov1alpha1.ResourceTypeManifest{
							{ID: "config", Template: configMapTemplate},
						},
					},
				},
				Parameters: &runtime.RawExtension{Raw: []byte(`{"revision":` + revision + `}`)},
			},
		}
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })
		return release
	}

	makeEnv := func() *openchoreov1alpha1.Environment {
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

	reconcileBinding := func(b *openchoreov1alpha1.ResourceReleaseBinding) *openchoreov1alpha1.ResourceReleaseBinding {
		_, err := bindingReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)})
		Expect(err).NotTo(HaveOccurred())
		updated := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), updated)).To(Succeed())
		return updated
	}

	rrKey := func(b *openchoreov1alpha1.ResourceReleaseBinding) client.ObjectKey {
		return client.ObjectKey{Name: makeRenderedReleaseName(b), Namespace: b.Namespace}
	}

	reconcileRR := func(b *openchoreov1alpha1.ResourceReleaseBinding) *openchoreov1alpha1.RenderedRelease {
		_, err := rrReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: rrKey(b)})
		Expect(err).NotTo(HaveOccurred())
		rr := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, rrKey(b), rr)).To(Succeed())
		return rr
	}

	resourcesReady := func(b *openchoreov1alpha1.ResourceReleaseBinding) *metav1.Condition {
		cond := meta.FindStatusCondition(b.Status.Conditions, string(ConditionResourcesReady))
		Expect(cond).NotTo(BeNil())
		return cond
	}

	configMapExists := func(name string) bool {
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: cmNamespace, Name: name}, &corev1.ConfigMap{})
		if apierrors.IsNotFound(err) {
			return false
		}
		Expect(err).NotTo(HaveOccurred())
		return true
	}

	It("deletes the old object and does not read its status as the new one's", func() {
		env := makeEnv()
		rel1 := makeRelease(prefix+"-release-r1", "1")
		rel2 := makeRelease(prefix+"-release-r2", "2")

		b := &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       prefix + "-binding",
				Namespace:  "default",
				Finalizers: []string{ResourceReleaseBindingFinalizer},
			},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "test-project",
					ResourceName: "test-resource",
				},
				Environment:     env.Name,
				ResourceRelease: rel1.Name,
			},
		}
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			rr := &openchoreov1alpha1.RenderedRelease{}
			if err := k8sClient.Get(ctx, rrKey(b), rr); err == nil {
				controllerutil.RemoveFinalizer(rr, renderedrelease.DataPlaneCleanupFinalizer)
				_ = k8sClient.Update(ctx, rr)
				_ = k8sClient.Delete(ctx, rr)
			}
			for _, name := range []string{cmOld, cmNew} {
				_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cmNamespace}})
			}
			latest := &openchoreov1alpha1.ResourceReleaseBinding{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(b), latest); err == nil {
				controllerutil.RemoveFinalizer(latest, ResourceReleaseBindingFinalizer)
				_ = k8sClient.Update(ctx, latest)
				_ = k8sClient.Delete(ctx, latest)
			}
		})

		By("rendering revision 1 and applying its ConfigMap")
		reconcileBinding(b)
		reconcileRR(b) // adds the cleanup finalizer
		rr := reconcileRR(b)
		Expect(configMapExists(cmOld)).To(BeTrue())
		Expect(rr.Status.Resources).To(HaveLen(1))
		Expect(rr.Status.Resources[0].Name).To(Equal(cmOld))
		Expect(resourcesReady(reconcileBinding(b)).Status).To(Equal(metav1.ConditionTrue))

		By("bumping the revision parameter")
		latest := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), latest)).To(Succeed())
		latest.Spec.ResourceRelease = rel2.Name
		Expect(k8sClient.Update(ctx, latest)).To(Succeed())

		By("not reading the old object's status before the renderedrelease controller catches up")
		// This binding reconcile updates the spec to revision 2 while status still
		// describes healthy revision 1 under the same ID.
		cond := resourcesReady(reconcileBinding(b))
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonResourcesProgressing)))

		By("applying the revision 2 ConfigMap and deleting the revision 1 ConfigMap")
		rr = reconcileRR(b)
		Expect(configMapExists(cmNew)).To(BeTrue())
		Expect(configMapExists(cmOld)).To(BeFalse())
		Expect(rr.Status.Resources).To(HaveLen(1))
		Expect(rr.Status.Resources[0].Name).To(Equal(cmNew))
		Expect(resourcesReady(reconcileBinding(b)).Status).To(Equal(metav1.ConditionTrue))
	})
})
