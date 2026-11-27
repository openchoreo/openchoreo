// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// releaseFixture is a release and its project's own namespace on the plane, reconciled
// through a plane proxy.
type releaseFixture struct {
	name        string
	own         string
	targetPlane string
	proxy       *planeProxy
	r           *Reconciler
	// sent is what the last pass sent.
	sent []*planeRequest
}

// newReleaseFixture creates a release that has its finalizer and renders nothing yet.
// The name is unique in the suite, and an empty target plane is the data plane.
func newReleaseFixture(name, targetPlane string) *releaseFixture {
	GinkgoHelper()
	f := &releaseFixture{name: "release-" + name, targetPlane: targetPlane, proxy: newPlaneProxy()}
	var envName string
	if targetPlane == targetPlaneObservabilityPlane {
		envName = observeTestOPPlane(name)
	} else {
		envName, _ = observeTestPlane(name)
	}
	f.own = dpkubernetes.DataPlaneNamespace(testNs, "test-project", envName)
	Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: f.own}})).To(Succeed())
	f.r = testReconcilerWithDPClient(f.proxy.client(), "", "")

	release := makeMinimalRelease(f.name, envName)
	release.Spec.TargetPlane = targetPlane
	Expect(k8sClient.Create(ctx, release)).To(Succeed())
	DeferCleanup(func() { forceDelete(ctx, f.key()) })
	f.mustPass()
	return f
}

func (f *releaseFixture) key() types.NamespacedName {
	return types.NamespacedName{Name: f.name, Namespace: testNs}
}

// pass reconciles the release once, and checks what it wrote against the rules.
func (f *releaseFixture) pass() (reconcile.Result, error) {
	GinkgoHelper()
	return f.passIn(ctx)
}

// passIn is a pass that ends early when its context is canceled.
func (f *releaseFixture) passIn(passCtx context.Context) (reconcile.Result, error) {
	GinkgoHelper()
	claims := fetchRelease(f.name).Spec.ObservedResources
	seen := len(f.proxy.requests())
	result, err := f.r.Reconcile(passCtx, reconcile.Request{NamespacedName: f.key()})
	f.sent = f.proxy.requests()[seen:]
	Expect(deleteViolations(f.sent, claims...)).To(BeEmpty())
	Expect(giveBackViolations(f.sent)).To(BeEmpty())
	release := &openchoreov1alpha1.RenderedRelease{}
	if getErr := k8sClient.Get(ctx, f.key(), release); getErr == nil {
		Expect(applyViolations(release, refusedBy(release, err), f.sent)).To(BeEmpty())
	} else {
		Expect(client.IgnoreNotFound(getErr)).To(Succeed())
	}
	return result, err
}

func (f *releaseFixture) mustPass() reconcile.Result {
	GinkgoHelper()
	result, err := f.pass()
	Expect(err).NotTo(HaveOccurred())
	return result
}

// change changes the release's spec.
func (f *releaseFixture) change(changes ...func(*openchoreov1alpha1.RenderedReleaseSpec)) {
	GinkgoHelper()
	release := fetchRelease(f.name)
	for _, change := range changes {
		change(&release.Spec)
	}
	Expect(k8sClient.Update(ctx, release)).To(Succeed())
}

// render changes what the release renders. Each object's ID is its name.
func (f *releaseFixture) render(objs ...*unstructured.Unstructured) {
	GinkgoHelper()
	f.change(rendering(objs...))
}

// rendering changes what a release renders. Each object's ID is its name.
func rendering(objs ...*unstructured.Unstructured) func(*openchoreov1alpha1.RenderedReleaseSpec) {
	GinkgoHelper()
	manifests := make([]openchoreov1alpha1.RenderedManifest, 0, len(objs))
	for _, obj := range objs {
		raw, err := obj.MarshalJSON()
		Expect(err).NotTo(HaveOccurred())
		manifests = append(manifests, openchoreov1alpha1.RenderedManifest{ID: obj.GetName(), Object: &runtime.RawExtension{Raw: raw}})
	}
	return func(spec *openchoreov1alpha1.RenderedReleaseSpec) { spec.Resources = manifests }
}

// claiming changes what a release claims. Each claim's ID is the object's name.
func claiming(objs ...*unstructured.Unstructured) func(*openchoreov1alpha1.RenderedReleaseSpec) {
	claims := make([]openchoreov1alpha1.ObservedResource, 0, len(objs))
	for _, obj := range objs {
		claims = append(claims, openchoreov1alpha1.ObservedResource{
			ID: obj.GetName(), APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Namespace: obj.GetNamespace(), Name: obj.GetName(),
		})
	}
	return func(spec *openchoreov1alpha1.RenderedReleaseSpec) { spec.ObservedResources = claims }
}

// inMode changes a release's mode.
func inMode(mode openchoreov1alpha1.RenderedReleaseMode) func(*openchoreov1alpha1.RenderedReleaseSpec) {
	return func(spec *openchoreov1alpha1.RenderedReleaseSpec) { spec.Mode = mode }
}

// deleteRelease deletes the release and runs the pass that marks it as finalizing.
func (f *releaseFixture) deleteRelease() {
	GinkgoHelper()
	Expect(k8sClient.Delete(ctx, fetchRelease(f.name))).To(Succeed())
	f.mustPass()
}

// deletes returns the deletes of the last pass.
func (f *releaseFixture) deletes() []*planeRequest {
	return f.sentThat(func(r *planeRequest) bool { return r.method == http.MethodDelete })
}

// writes returns everything the last pass wrote or tried to.
func (f *releaseFixture) writes() []*planeRequest {
	return f.sentThat(func(r *planeRequest) bool { return r.method != http.MethodGet })
}

// reads returns the objects the last pass read one by one.
func (f *releaseFixture) reads() []*planeRequest {
	return f.sentThat(func(r *planeRequest) bool { return r.method == http.MethodGet && !isList(r) })
}

func (f *releaseFixture) sentThat(match func(*planeRequest) bool) []*planeRequest {
	var requests []*planeRequest
	for _, r := range f.sent {
		if match(r) {
			requests = append(requests, r)
		}
	}
	return requests
}

// object returns an object of a kind the plane's agent may write, in the own namespace:
// a ConfigMap on the data plane, an alert rule on the observability plane.
func (f *releaseFixture) object(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{"data": map[string]any{"by": "release"}}}
	obj.SetAPIVersion("v1")
	obj.SetKind("ConfigMap")
	if f.targetPlane == targetPlaneObservabilityPlane {
		obj = &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{
			"name":      name,
			"source":    map[string]any{"type": "log", "query": "error"},
			"condition": map[string]any{"window": "5m", "interval": "1m", "operator": "gt", "threshold": int64(1)},
			"actions":   map[string]any{"notifications": map[string]any{"channels": []any{"default"}}},
		}}}
		obj.SetAPIVersion("openchoreo.dev/v1alpha1")
		obj.SetKind("ObservabilityAlertRule")
	}
	obj.SetNamespace(f.own)
	obj.SetName(name)
	return obj
}

// live reads an object from the plane, and returns nil when there is none.
func (f *releaseFixture) live(obj *unstructured.Unstructured) *unstructured.Unstructured {
	GinkgoHelper()
	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(obj.GroupVersionKind())
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), live)
	if client.IgnoreNotFound(err) == nil && err != nil {
		return nil
	}
	Expect(err).NotTo(HaveOccurred())
	return live
}
