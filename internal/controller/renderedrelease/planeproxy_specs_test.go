// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
)

var _ = Describe("plane proxy", func() {
	const (
		cmName = "plane-proxy"
		cmPath = "/api/v1/namespaces/" + testNs + "/configmaps/" + cmName
	)
	var (
		proxy *planeProxy
		plane client.Client
	)

	newCM := func(value string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: testNs},
			Data:       map[string]string{"key": value},
		}
	}
	apply := func(value string) error {
		return plane.Patch(ctx, newCM(value), client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName))
	}
	liveValue := func() string {
		GinkgoHelper()
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: testNs, Name: cmName}, cm)).To(Succeed())
		return cm.Data["key"]
	}

	BeforeEach(func() {
		proxy = newPlaneProxy()
		plane = proxy.client()
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, newCM("")) })
	})

	It("hands out the client production uses", func() {
		Expect(plane).To(BeAssignableToTypeOf(&kubernetesClient.ProxyClient{}))
	})

	It("forwards requests and records each with its body, its answer and the object around it", func() {
		Expect(apply("one")).To(Succeed())
		Expect(apply("two")).To(Succeed())
		cm := &corev1.ConfigMap{}
		Expect(plane.Get(ctx, client.ObjectKey{Namespace: testNs, Name: cmName}, cm)).To(Succeed())
		Expect(cm.Data).To(HaveKeyWithValue("key", "two"))
		Expect(plane.Delete(ctx, newCM(""))).To(Succeed())

		requests := proxy.requests()
		Expect(requests).To(HaveLen(4))
		create, update, get, del := requests[0], requests[1], requests[2], requests[3]

		Expect(create.method).To(Equal(http.MethodPatch))
		Expect(create.path).To(Equal(cmPath))
		Expect(create.query).To(Equal("fieldManager=" + ControllerName + "&force=true"))
		Expect(create.contentType).To(Equal("application/apply-patch+yaml"))
		Expect(string(create.body)).To(ContainSubstring(`"key":"one"`))
		Expect(create.status).To(Equal(http.StatusCreated))
		Expect(string(create.answer)).To(ContainSubstring(`"uid"`))
		Expect(create.before).To(BeNil())
		Expect(create.after.Object).To(HaveKeyWithValue("data", HaveKeyWithValue("key", "one")))

		Expect(update.status).To(Equal(http.StatusOK))
		Expect(update.before.GetUID()).To(Equal(create.after.GetUID()))
		Expect(update.before.Object).To(HaveKeyWithValue("data", HaveKeyWithValue("key", "one")))
		Expect(update.after.Object).To(HaveKeyWithValue("data", HaveKeyWithValue("key", "two")))

		Expect(get.method).To(Equal(http.MethodGet))
		Expect(get.before).To(BeNil(), "a read changes nothing, so nothing is recorded around it")
		Expect(string(get.answer)).To(ContainSubstring(`"key":"two"`))

		Expect(del.method).To(Equal(http.MethodDelete))
		Expect(del.body).To(BeEmpty())
		Expect(del.before).NotTo(BeNil())
		Expect(del.after).To(BeNil())
	})

	It("passes the API server's errors on", func() {
		err := plane.Get(ctx, client.ObjectKey{Namespace: testNs, Name: "plane-proxy-missing"}, &corev1.ConfigMap{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "got %v", err)
	})

	It("stores nothing in a dry run", func() {
		Expect(k8sClient.Create(ctx, newCM("outside"))).To(Succeed())

		dryRun := newCM("release")
		Expect(plane.Patch(ctx, dryRun, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName),
			client.DryRunAll)).To(Succeed())

		Expect(dryRun.Data).To(HaveKeyWithValue("key", "release"), "the answer shows what would be stored")
		Expect(liveValue()).To(Equal("outside"))
	})

	It("rejects in a dry run what it would reject for real", func() {
		fixed := newCM("outside")
		fixed.Immutable = ptr.To(true)
		Expect(k8sClient.Create(ctx, fixed)).To(Succeed())

		err := plane.Patch(ctx, newCM("release"), client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName), client.DryRunAll)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)
	})

	It("deletes an object only while it is the one that was read", func() {
		read := newCM("release")
		Expect(k8sClient.Create(ctx, read)).To(Succeed())
		Expect(k8sClient.Delete(ctx, newCM(""))).To(Succeed())
		recreated := newCM("outside")
		Expect(k8sClient.Create(ctx, recreated)).To(Succeed())

		err := plane.Delete(ctx, newCM(""), client.Preconditions{UID: ptr.To(read.UID)})
		Expect(apierrors.IsConflict(err)).To(BeTrue(), "got %v", err)
		Expect(liveValue()).To(Equal("outside"))

		Expect(plane.Delete(ctx, newCM(""), client.Preconditions{UID: ptr.To(recreated.UID)})).To(Succeed())
		Expect(plane.Delete(ctx, newCM(""), client.Preconditions{UID: ptr.To(recreated.UID)})).To(Succeed(),
			"an object that is gone is deleted")
	})

	It("runs a hook before it forwards the request, on the first match only", func() {
		ran := 0
		proxy.hook(planeHook{match: withMethod(http.MethodPatch), before: func() {
			ran++
			Expect(k8sClient.Create(ctx, newCM("outside"))).To(Succeed())
		}})

		Expect(apply("release")).To(Succeed())
		Expect(apply("release")).To(Succeed())

		Expect(ran).To(Equal(1))
		Expect(proxy.requests()[0].before.Object).To(HaveKeyWithValue("data", HaveKeyWithValue("key", "outside")),
			"the hook ran before the object was recorded")
	})

	It("runs a hook after the write landed, before the answer is sent", func() {
		ran := false
		proxy.hook(planeHook{match: withMethod(http.MethodPatch), after: func() {
			Expect(liveValue()).To(Equal("release"))
			ran = true
		}})
		Expect(apply("release")).To(Succeed())
		Expect(ran).To(BeTrue())
	})

	It("answers instead of forwarding, on every match when asked to", func() {
		proxy.hook(planeHook{match: withMethod(http.MethodPatch), every: true,
			answer: &planeAnswer{status: http.StatusForbidden, contentType: "text/plain", body: "Forbidden: Agent not authorized"}})

		for range 2 {
			err := apply("release")
			Expect(apierrors.IsForbidden(err)).To(BeTrue(), "got %v", err)
		}
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(newCM("")), newCM("")))).To(BeTrue())
		Expect(proxy.requests()).To(HaveLen(2), "an answered request is recorded too")
	})

	It("answers a read of an object that exists as if it were missing", func() {
		Expect(apply("release")).To(Succeed())
		proxy.hook(planeHook{match: withMethod(http.MethodGet),
			answer: &planeAnswer{status: http.StatusNotFound, contentType: "text/plain", body: "404 page not found"}})

		err := plane.Get(ctx, client.ObjectKeyFromObject(newCM("")), newCM(""))
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "got %v", err)
		Expect(liveValue()).To(Equal("release"))
	})

	It("loses the answer of a write that landed", func() {
		proxy.hook(planeHook{match: withMethod(http.MethodPatch), lose: true})
		Expect(apply("release")).To(MatchError(ContainSubstring("502")))
		Expect(liveValue()).To(Equal("release"))
	})

	It("forgets its requests and hooks on reset", func() {
		proxy.hook(planeHook{match: withMethod(http.MethodPatch), every: true, lose: true})
		Expect(apply("release")).NotTo(Succeed())
		proxy.reset()
		Expect(proxy.requests()).To(BeEmpty())
		Expect(apply("release")).To(Succeed())
	})
})
