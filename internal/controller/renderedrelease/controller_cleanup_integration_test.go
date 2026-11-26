// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// A release stops rendering its objects in three places, and all of them clean up the
// same way.
var _ = Describe("RenderedRelease cleanup", func() {
	type place struct {
		name        string
		targetPlane string
		finalizes   bool
	}
	places := []place{
		{name: "a pass"},
		{name: "the finalizer", finalizes: true},
		{name: "the observability plane's finalizer", targetPlane: targetPlaneObservabilityPlane, finalizes: true},
	}

	// leave makes the release stop rendering everything.
	leave := func(p place, f *releaseFixture) {
		GinkgoHelper()
		if p.finalizes {
			f.deleteRelease()
			return
		}
		f.render()
	}
	// expectFailed checks that the last pass reported an error.
	expectFailed := func(p place, f *releaseFixture, err error, message string) {
		GinkgoHelper()
		Expect(err).To(MatchError(ContainSubstring(message)))
		if p.finalizes {
			finalizing := findCondition(fetchRelease(f.name), string(ConditionFinalizing))
			Expect(finalizing.Reason).To(Equal(string(ReasonCleanupFailed)))
			Expect(finalizing.Message).To(ContainSubstring(message))
		}
	}
	// expectDone checks that nothing is left to do.
	expectDone := func(p place, f *releaseFixture) {
		GinkgoHelper()
		f.mustPass()
		Expect(f.deletes()).To(BeEmpty())
		if p.finalizes {
			err := k8sClient.Get(ctx, f.key(), &openchoreov1alpha1.RenderedRelease{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "the finalizer is done, got %v", err)
		}
	}
	// recreate creates an object the release did not apply in the place of a deleted one.
	recreate := func(f *releaseFixture, name string) *unstructured.Unstructured {
		GinkgoHelper()
		obj := f.object(name)
		Expect(k8sClient.Create(ctx, obj, client.FieldOwner("helm"))).To(Succeed())
		return obj
	}

	for i, p := range places {
		Context("in "+p.name, func() {
			var f *releaseFixture
			start := func(name string) {
				GinkgoHelper()
				f = newReleaseFixture(fmt.Sprintf("cleanup-%s-%d", name, i), p.targetPlane)
				f.render(f.object("a"), f.object("b"), f.object("c"))
				f.mustPass()
				for _, name := range []string{"a", "b", "c"} {
					Expect(f.live(f.object(name))).NotTo(BeNil())
				}
				DeferCleanup(func() {
					for _, name := range []string{"a", "b", "c"} {
						_ = k8sClient.Delete(ctx, f.object(name))
					}
				})
			}

			It("keeps an object that is recreated between the list and the delete", func() {
				start("recreated")
				leave(p, f)
				var recreated *unstructured.Unstructured
				f.proxy.hook(planeHook{match: withMethod(http.MethodDelete), before: func() {
					Expect(k8sClient.Delete(ctx, f.object("a"))).To(Succeed())
					recreated = recreate(f, "a")
				}})

				_, err := f.pass()
				expectFailed(p, f, err, "the UID in the precondition")
				Expect(f.live(f.object("a")).GetUID()).To(Equal(recreated.GetUID()))
				Expect(f.live(f.object("b"))).To(BeNil())
				Expect(f.live(f.object("c"))).To(BeNil())

				expectDone(p, f)
				Expect(f.live(f.object("a")).GetResourceVersion()).To(Equal(recreated.GetResourceVersion()), "nobody wrote it")
			})

			It("deletes an object that changes between the list and the delete", func() {
				start("busy")
				leave(p, f)
				changed := f.proxy.hook(planeHook{match: withMethod(http.MethodDelete), before: func() {
					busy := f.live(f.object("a"))
					busy.SetAnnotations(map[string]string{"example.com/refreshed": "now"})
					Expect(k8sClient.Update(ctx, busy, client.FieldOwner("operator"))).To(Succeed())
				}})

				f.mustPass()
				Expect(changed.used).To(BeTrue(), "it changed")
				Expect(f.live(f.object("a"))).To(BeNil())
				expectDone(p, f)
			})

			It("asks once to delete an object a finalizer holds", func() {
				start("held")
				hold := func(finalizers ...string) {
					GinkgoHelper()
					held := f.live(f.object("a"))
					held.SetFinalizers(finalizers)
					Expect(k8sClient.Update(ctx, held, client.FieldOwner("operator"))).To(Succeed())
				}
				hold("example.com/hold")
				DeferCleanup(func() {
					if held := f.live(f.object("a")); held != nil {
						held.SetFinalizers(nil)
						_ = k8sClient.Update(ctx, held)
					}
				})
				leave(p, f)

				f.mustPass()
				Expect(f.deletes()).To(HaveLen(3))
				Expect(f.live(f.object("a")).GetDeletionTimestamp()).NotTo(BeNil())

				f.mustPass()
				Expect(f.deletes()).To(BeEmpty(), "it is being deleted already")
				Expect(f.live(f.object("a"))).NotTo(BeNil())

				hold()
				expectDone(p, f)
			})

			It("tries the rest after a delete fails, and the next pass completes", func() {
				start("one-fails")
				leave(p, f)
				f.proxy.hook(planeHook{match: withMethod(http.MethodDelete),
					answer: &planeAnswer{status: http.StatusForbidden, contentType: "text/plain", body: "Forbidden: no"}})

				_, err := f.pass()
				expectFailed(p, f, err, "failed to delete stale resource a")
				Expect(f.deletes()).To(HaveLen(3))
				Expect(f.live(f.object("a"))).NotTo(BeNil())
				Expect(f.live(f.object("b"))).To(BeNil())
				Expect(f.live(f.object("c"))).To(BeNil())

				f.mustPass()
				Expect(f.live(f.object("a"))).To(BeNil())
				expectDone(p, f)
			})

			It("reports every delete that fails", func() {
				start("all-fail")
				leave(p, f)
				f.proxy.hook(planeHook{match: withMethod(http.MethodDelete), every: true,
					answer: &planeAnswer{status: http.StatusBadGateway, contentType: "text/plain", body: "no agents"}})

				_, err := f.pass()
				expectFailed(p, f, err, "failed to delete stale resource a")
				expectFailed(p, f, err, "failed to delete stale resource c")
				Expect(f.deletes()).To(HaveLen(3))
			})

			for _, status := range []int{http.StatusForbidden, http.StatusBadGateway} {
				It(fmt.Sprintf("deletes nothing when a list answers %d", status), func() {
					start(fmt.Sprintf("list-%d", status))
					leave(p, f)
					f.proxy.hook(planeHook{match: isList,
						answer: &planeAnswer{status: status, contentType: "text/plain", body: "no"}})

					_, err := f.pass()
					expectFailed(p, f, err, "failed to list resources")
					Expect(f.deletes()).To(BeEmpty())
					Expect(fetchRelease(f.name).Finalizers).NotTo(BeEmpty())

					f.mustPass()
					Expect(f.deletes()).To(HaveLen(3))
					expectDone(p, f)
				})
			}

			It("leaves an object whose labels name another release", func() {
				start("relabelled")
				relabelled := f.live(f.object("a"))
				relabelled.SetLabels(withLabel(relabelled.GetLabels(), labels.LabelKeyRenderedReleaseName, "another"))
				Expect(k8sClient.Update(ctx, relabelled, client.FieldOwner("kubectl-edit"))).To(Succeed())
				f.render(f.object("b"), f.object("c"))
				leave(p, f)

				f.mustPass()
				Expect(f.live(f.object("a"))).NotTo(BeNil())
				Expect(f.live(f.object("b"))).To(BeNil())
				expectDone(p, f)
			})
		})
	}

	It("deletes an object once when two API versions list it", func() {
		f := newReleaseFixture("cleanup-two-versions", "")
		hpa := &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{
			"maxReplicas":    int64(3),
			"scaleTargetRef": map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "name": "api"},
		}}}
		hpa.SetAPIVersion("autoscaling/v1") // the controller always lists v2
		hpa.SetKind("HorizontalPodAutoscaler")
		hpa.SetNamespace(f.own)
		hpa.SetName("api")
		f.render(hpa)
		f.mustPass()
		Expect(f.live(hpa)).NotTo(BeNil())

		f.render()
		f.mustPass()
		Expect(f.deletes()).To(HaveLen(1))
		Expect(f.live(hpa)).To(BeNil())
	})

	It("finalizes a release that renders a kind the plane does not serve", func() {
		f := newReleaseFixture("cleanup-unserved", "")
		gizmo := &unstructured.Unstructured{}
		gizmo.SetAPIVersion("nope.example.com/v1")
		gizmo.SetKind("Gizmo")
		gizmo.SetNamespace(f.own)
		gizmo.SetName("g")
		f.render(gizmo)
		_, err := f.pass()
		Expect(err).To(HaveOccurred())

		f.deleteRelease()
		f.mustPass()
		err = k8sClient.Get(ctx, f.key(), &openchoreov1alpha1.RenderedRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "got %v", err)
	})
})
