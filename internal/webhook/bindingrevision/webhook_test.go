// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package bindingrevision

import (
	"encoding/json"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func releaseBindingRevision() *openchoreov1alpha1.ReleaseBindingRevision {
	return &openchoreov1alpha1.ReleaseBindingRevision{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "rbr-", Namespace: "default"},
		Spec: openchoreov1alpha1.ReleaseBindingRevisionSpec{
			Revision:     1,
			Cause:        openchoreov1alpha1.RevisionCauseDeploy,
			SnapshotHash: "hash-1",
			BindingSpec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner:                           openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "proj", ComponentName: "comp"},
				Environment:                     "dev",
				ReleaseName:                     "comp-abc123",
				ComponentTypeEnvironmentConfigs: &runtime.RawExtension{Raw: []byte(`{"replicas":2,"cpu":"100m"}`)},
				TraitEnvironmentConfigs: map[string]runtime.RawExtension{
					"ingress": {Raw: []byte(`{"host":"a.example.com"}`)},
				},
			},
		},
	}
}

func resourceReleaseBindingRevision() *openchoreov1alpha1.ResourceReleaseBindingRevision {
	return &openchoreov1alpha1.ResourceReleaseBindingRevision{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "rrbr-", Namespace: "default"},
		Spec: openchoreov1alpha1.ResourceReleaseBindingRevisionSpec{
			Revision:     1,
			Cause:        openchoreov1alpha1.RevisionCauseDeploy,
			SnapshotHash: "hash-1",
			BindingSpec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner:                          openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "proj", ResourceName: "db"},
				Environment:                    "dev",
				ResourceRelease:                "db-abc123",
				ResourceTypeEnvironmentConfigs: &runtime.RawExtension{Raw: []byte(`{"size":"small"}`)},
			},
		},
	}
}

// releaseBindingRevisionWebhook is the ReleaseBindingRevision validating
// webhook's name, as config/webhook/manifests.yaml registers it. The apiserver
// names the webhook that denied a request in its error.
const (
	releaseBindingRevisionWebhook = "vreleasebindingrevision-v1alpha1.kb.io"
)

// expectRejected asserts that err is the named webhook's denial of a spec
// change. The CRDs' CEL rule rejects with the same message, but as a
// different error that names no webhook, so this fails if the webhook did not
// run.
func expectRejected(err error, webhookName string) {
	GinkgoHelper()
	Expect(err).To(HaveOccurred())
	Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected an Invalid error, got %v", err)
	Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("admission webhook %q denied the request", webhookName)))
	Expect(err.Error()).To(ContainSubstring("spec: Forbidden: " + ImmutableSpecMessage))
}

// update returns the admission request for updating oldObj to newObj. Each is
// a typed revision or a generic map, as the apiserver would serialize it.
func update(oldObj, newObj any) admission.Request {
	GinkgoHelper()
	oldRaw, err := json.Marshal(oldObj)
	Expect(err).NotTo(HaveOccurred())
	newRaw, err := json.Marshal(newObj)
	Expect(err).NotTo(HaveOccurred())
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Update,
		Name:      "rev",
		OldObject: runtime.RawExtension{Raw: oldRaw},
		Object:    runtime.RawExtension{Raw: newRaw},
	}}
}

// asMap returns obj as the generic map the apiserver holds for it.
func asMap(obj any) map[string]any {
	GinkgoHelper()
	raw, err := json.Marshal(obj)
	Expect(err).NotTo(HaveOccurred())
	out := map[string]any{}
	Expect(json.Unmarshal(raw, &out)).To(Succeed())
	return out
}

// withWorkloadOverrides returns obj with bindingSpec.workloadOverrides set to v.
func withWorkloadOverrides(obj map[string]any, v map[string]any) map[string]any {
	obj["spec"].(map[string]any)["bindingSpec"].(map[string]any)["workloadOverrides"] = v
	return obj
}

func expectDenied(resp admission.Response) {
	GinkgoHelper()
	Expect(resp.Allowed).To(BeFalse())
	Expect(resp.Result).NotTo(BeNil())
	Expect(resp.Result.Reason).To(Equal(metav1.StatusReasonInvalid))
	Expect(resp.Result.Code).To(Equal(int32(http.StatusUnprocessableEntity)))
	Expect(resp.Result.Message).To(ContainSubstring(ImmutableSpecMessage))
}

var _ = Describe("binding revision webhook", func() {
	Context("ReleaseBindingRevision updates", func() {
		g := NewSpecGuard(openchoreov1alpha1.GroupVersion.WithKind("ReleaseBindingRevision").GroupKind())

		DescribeTable("rejects any change to the spec",
			func(mutate func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec)) {
				oldRev := releaseBindingRevision()
				newRev := oldRev.DeepCopy()
				mutate(&newRev.Spec)
				expectDenied(g.Handle(ctx, update(oldRev, newRev)))
			},
			// The case CEL misses: same number of keys, one key replaced.
			Entry("a config key replaced by another", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
				s.BindingSpec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"count":2,"cpu":"100m"}`)}
			}),
			Entry("a trait config key replaced by another", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
				s.BindingSpec.TraitEnvironmentConfigs["ingress"] = runtime.RawExtension{Raw: []byte(`{"path":"a.example.com"}`)}
			}),
			Entry("a config value changed", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
				s.BindingSpec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"replicas":3,"cpu":"100m"}`)}
			}),
			Entry("a config key added", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
				s.BindingSpec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"replicas":2,"cpu":"100m","memory":"1Gi"}`)}
			}),
			Entry("the configs removed", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) {
				s.BindingSpec.ComponentTypeEnvironmentConfigs = nil
			}),
			Entry("the release", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) { s.BindingSpec.ReleaseName = "comp-def456" }),
			Entry("the revision number", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) { s.Revision = 2 }),
			Entry("the change message", func(s *openchoreov1alpha1.ReleaseBindingRevisionSpec) { s.Message = "edited" }),
		)

		// Fields the CRD preserves but the Go type does not declare: a typed
		// decode drops them, so only the admitted object shows the change.
		It("rejects an unknown workloadOverrides key replaced by another", func() {
			oldObj := withWorkloadOverrides(asMap(releaseBindingRevision()), map[string]any{"x": 1})
			newObj := withWorkloadOverrides(asMap(releaseBindingRevision()), map[string]any{"y": 1})
			expectDenied(g.Handle(ctx, update(oldObj, newObj)))
		})

		It("rejects an unknown workloadOverrides key removed", func() {
			oldObj := withWorkloadOverrides(asMap(releaseBindingRevision()), map[string]any{"x": 1})
			newObj := withWorkloadOverrides(asMap(releaseBindingRevision()), map[string]any{})
			expectDenied(g.Handle(ctx, update(oldObj, newObj)))
		})

		It("accepts the same spec with its configs' keys in another order", func() {
			oldRev := releaseBindingRevision()
			newRev := oldRev.DeepCopy()
			newRev.Spec.BindingSpec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{ "cpu": "100m", "replicas": 2 }`)}
			Expect(g.Handle(ctx, update(oldRev, newRev)).Allowed).To(BeTrue())
		})

		It("accepts metadata and status changes, keeping unknown fields", func() {
			oldObj := withWorkloadOverrides(asMap(releaseBindingRevision()), map[string]any{"x": 1})
			newObj := withWorkloadOverrides(asMap(releaseBindingRevision()), map[string]any{"x": 1})
			newObj["metadata"].(map[string]any)["labels"] = map[string]any{"openchoreo.dev/release-binding": "comp-dev"}
			newObj["metadata"].(map[string]any)["annotations"] = map[string]any{"openchoreo.dev/binding-generation": "3"}
			newObj["status"] = map[string]any{"active": true}
			Expect(g.Handle(ctx, update(oldObj, newObj)).Allowed).To(BeTrue())
		})

		It("allows requests other than updates", func() {
			req := update(releaseBindingRevision(), releaseBindingRevision())
			req.Operation = admissionv1.Create
			req.OldObject = runtime.RawExtension{}
			Expect(g.Handle(ctx, req).Allowed).To(BeTrue())
		})

		It("refuses an update it cannot decode", func() {
			req := update(releaseBindingRevision(), releaseBindingRevision())
			req.Object = runtime.RawExtension{Raw: []byte(`{`)}
			resp := g.Handle(ctx, req)
			Expect(resp.Allowed).To(BeFalse())
			Expect(resp.Result.Code).To(Equal(int32(http.StatusBadRequest)))
		})
	})

	Context("ResourceReleaseBindingRevision updates", func() {
		g := NewSpecGuard(openchoreov1alpha1.GroupVersion.WithKind("ResourceReleaseBindingRevision").GroupKind())

		It("rejects a config key replaced by another", func() {
			oldRev := resourceReleaseBindingRevision()
			newRev := oldRev.DeepCopy()
			newRev.Spec.BindingSpec.ResourceTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"tier":"small"}`)}
			expectDenied(g.Handle(ctx, update(oldRev, newRev)))
		})

		It("accepts metadata changes", func() {
			oldRev := resourceReleaseBindingRevision()
			newRev := oldRev.DeepCopy()
			newRev.Labels = map[string]string{"openchoreo.dev/resource-release-binding": "db-dev"}
			Expect(g.Handle(ctx, update(oldRev, newRev)).Allowed).To(BeTrue())
		})
	})

	// Against a real apiserver with the webhook registered: the key
	// replacement the CRD's CEL rule lets through is rejected.
	Context("installed on an apiserver", func() {
		BeforeEach(func() {
			if k8sClient == nil {
				Skip("envtest apiserver not available")
			}
		})

		It("rejects a config key replacement in a ReleaseBindingRevision", func() {
			rev := releaseBindingRevision()
			Expect(k8sClient.Create(ctx, rev)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rev) })

			updated := rev.DeepCopy()
			updated.Spec.BindingSpec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"count":2,"cpu":"100m"}`)}
			expectRejected(k8sClient.Update(ctx, updated), releaseBindingRevisionWebhook)

			stored := &openchoreov1alpha1.ReleaseBindingRevision{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rev), stored)).To(Succeed())
			Expect(string(stored.Spec.BindingSpec.ComponentTypeEnvironmentConfigs.Raw)).To(MatchJSON(`{"replicas":2,"cpu":"100m"}`))
		})

		It("still accepts the controller's metadata and status writes", func() {
			rev := releaseBindingRevision()
			Expect(k8sClient.Create(ctx, rev)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rev) })

			rev.Labels = map[string]string{"openchoreo.dev/release-binding": "comp-dev"}
			Expect(k8sClient.Update(ctx, rev)).To(Succeed())
			rev.Status.Active = true
			rev.Status.Health = openchoreov1alpha1.RevisionHealthHealthy
			Expect(k8sClient.Status().Update(ctx, rev)).To(Succeed())
		})

		It("rejects an unknown workloadOverrides key replacement written through the apiserver", func() {
			obj := &unstructured.Unstructured{Object: withWorkloadOverrides(asMap(releaseBindingRevision()), map[string]any{"x": int64(1)})}
			obj.SetAPIVersion(openchoreov1alpha1.GroupVersion.String())
			obj.SetKind("ReleaseBindingRevision")
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })

			updated := obj.DeepCopy()
			Expect(unstructured.SetNestedMap(updated.Object, map[string]any{"y": int64(1)}, "spec", "bindingSpec", "workloadOverrides")).To(Succeed())
			expectRejected(k8sClient.Update(ctx, updated), releaseBindingRevisionWebhook)

			stored := &unstructured.Unstructured{}
			stored.SetGroupVersionKind(obj.GroupVersionKind())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), stored)).To(Succeed())
			overrides, _, _ := unstructured.NestedMap(stored.Object, "spec", "bindingSpec", "workloadOverrides")
			Expect(overrides).To(Equal(map[string]any{"x": int64(1)}))
		})

		// resourceTypeEnvironmentConfigs is an untyped field, which CEL compares
		// in full, so here the CEL rule rejects the key replacement. The apiserver
		// runs CEL before validating webhooks, so the update never reaches the
		// webhook; the webhook's own decision is covered by the SpecGuard cases
		// above.
		It("rejects a config key replacement in a ResourceReleaseBindingRevision by its CEL rule", func() {
			rev := resourceReleaseBindingRevision()
			Expect(k8sClient.Create(ctx, rev)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rev) })

			updated := rev.DeepCopy()
			updated.Spec.BindingSpec.ResourceTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"tier":"small"}`)}
			err := k8sClient.Update(ctx, updated)
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected an Invalid error, got %v", err)
			Expect(err.Error()).To(ContainSubstring("spec: Invalid value: " + ImmutableSpecMessage))
			Expect(err.Error()).NotTo(ContainSubstring("admission webhook"))

			stored := &openchoreov1alpha1.ResourceReleaseBindingRevision{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rev), stored)).To(Succeed())
			Expect(string(stored.Spec.BindingSpec.ResourceTypeEnvironmentConfigs.Raw)).To(MatchJSON(`{"size":"small"}`))
		})
	})
})
