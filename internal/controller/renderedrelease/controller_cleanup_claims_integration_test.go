// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"fmt"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("RenderedRelease cleanup of claimed objects", func() {
	const (
		manage  = openchoreov1alpha1.RenderedReleaseModeManage
		observe = openchoreov1alpha1.RenderedReleaseModeObserve
	)
	var f *releaseFixture

	// applyFirst makes the release apply objects and returns them as they are on the plane.
	applyFirst := func(objs ...*unstructured.Unstructured) []*unstructured.Unstructured {
		GinkgoHelper()
		f.render(objs...)
		f.mustPass()
		applied := make([]*unstructured.Unstructured, 0, len(objs))
		for _, obj := range objs {
			live := f.live(obj)
			Expect(live).NotTo(BeNil())
			applied = append(applied, live)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj.DeepCopy()) })
		}
		return applied
	}
	label := func(obj *unstructured.Unstructured) {
		GinkgoHelper()
		claimed := unstructuredObj(obj.GetAPIVersion(), obj.GetKind(), obj.GetNamespace(), obj.GetName())
		claimed.SetLabels(map[string]string{labels.LabelKeyClaimed: "true"})
		Expect(k8sClient.Patch(ctx, claimed, client.Apply, client.FieldOwner("renderedrelease-takeover"))).To(Succeed())
	}
	expectGivenBack := func(before *unstructured.Unstructured) {
		GinkgoHelper()
		after := f.live(before)
		Expect(after).NotTo(BeNil(), "the object is kept")
		Expect(after.GetUID()).To(Equal(before.GetUID()))
		for key := range after.GetLabels() {
			Expect(markLabels.Has(key)).To(BeFalse(), "label %s is left", key)
		}
		Expect(hasApplyEntry(after)).To(BeFalse())
		Expect(withoutMarks(after)).To(Equal(withoutMarks(before)))
	}
	expectDeleted := func(obj *unstructured.Unstructured) {
		GinkgoHelper()
		Expect(f.live(obj)).To(BeNil())
	}
	expectUnchanged := func(obj *unstructured.Unstructured) {
		GinkgoHelper()
		Expect(f.live(obj).GetResourceVersion()).To(Equal(obj.GetResourceVersion()))
	}
	released := func() bool {
		return apierrors.IsNotFound(k8sClient.Get(ctx, f.key(), &openchoreov1alpha1.RenderedRelease{}))
	}
	isGiveBack := func(r *planeRequest) bool { return r.contentType == string(types.JSONPatchType) }

	// The ways a release stops rendering an object. Each makes its changes in one update.
	ways := []struct {
		name        string
		targetPlane string
		changes     []func(*openchoreov1alpha1.RenderedReleaseSpec)
		finalizes   bool
	}{
		{name: "the template drops it", changes: []func(*openchoreov1alpha1.RenderedReleaseSpec){rendering()}},
		{name: "the release moves to Observe, with its resources still listed", changes: []func(*openchoreov1alpha1.RenderedReleaseSpec){inMode(observe)}},
		{name: "the release is deleted", finalizes: true},
		{name: "the release is deleted on the observability plane", targetPlane: targetPlaneObservabilityPlane, finalizes: true},
	}
	for i, way := range ways {
		Context("when "+way.name, func() {
			start := func(name string) {
				GinkgoHelper()
				f = newReleaseFixture(fmt.Sprintf("kept-%s-%d", name, i), way.targetPlane)
			}
			leave := func(claims ...*unstructured.Unstructured) {
				GinkgoHelper()
				f.change(append([]func(*openchoreov1alpha1.RenderedReleaseSpec){claiming(claims...)}, way.changes...)...)
				if way.finalizes {
					f.deleteRelease()
				}
			}

			It("gives back what is claimed", func() {
				start("claimed")
				a := applyFirst(f.object("a"))[0]
				leave(f.object("a"))
				f.mustPass()
				expectGivenBack(a)
				Expect(f.writes()).To(HaveLen(2), "the label, and the patch that gives it back")
				Expect(released()).To(Equal(way.finalizes), "nothing to wait for")
			})

			It("gives back what was claimed once, and is labeled", func() {
				start("labeled")
				a := applyFirst(f.object("a"))[0]
				label(a)
				leave()
				f.mustPass()
				expectGivenBack(a)
				Expect(f.writes()).To(HaveLen(1))
			})

			It("deletes what was never claimed", func() {
				start("unclaimed")
				objs := applyFirst(f.object("a"), f.object("b"))
				leave(f.object("another"))
				f.mustPass()
				expectDeleted(objs[0])
				expectDeleted(objs[1])
				Expect(f.writes()).To(HaveLen(2))
			})

			It("deletes the rest next to what it gives back, after it", func() {
				start("both")
				objs := applyFirst(f.object("a"), f.object("b"), f.object("c"))
				leave(f.object("b"))
				f.mustPass()
				expectDeleted(objs[0])
				expectGivenBack(objs[1])
				expectDeleted(objs[2])
				writes := f.writes()
				Expect(writes).To(HaveLen(4))
				Expect(isGiveBack(writes[1])).To(BeTrue())
				Expect(writes[2].method).To(Equal(http.MethodDelete))
			})

			It("never deletes what is claimed while the claim can't be read", func() {
				start("unreadable")
				a := applyFirst(f.object("a"))[0]
				leave(f.object("a"))
				f.proxy.hook(planeHook{match: func(r *planeRequest) bool { return r.method == http.MethodGet && !isList(r) }, every: true,
					answer: &planeAnswer{status: http.StatusBadGateway, contentType: "text/plain", body: "no agents"}})
				f.mustPass()
				expectGivenBack(a)
			})

			It("tries the rest after one give back and one delete fail, and the next pass completes", func() {
				start("failures")
				objs := applyFirst(f.object("a"), f.object("b"), f.object("c"), f.object("d"))
				leave(f.object("a"), f.object("b"))
				no := &planeAnswer{status: http.StatusForbidden, contentType: "text/plain", body: "Forbidden: no"}
				f.proxy.hook(planeHook{match: isGiveBack, answer: no})
				f.proxy.hook(planeHook{match: withMethod(http.MethodDelete), answer: no})

				_, err := f.pass()
				Expect(err).To(MatchError(And(ContainSubstring("failed to give back"), ContainSubstring("failed to delete stale resource c"))))
				Expect(f.live(objs[0]).GetLabels()).To(HaveKey(labels.LabelKeyRenderedReleaseName))
				expectGivenBack(objs[1])
				Expect(f.live(objs[2])).NotTo(BeNil())
				expectDeleted(objs[3])
				if way.finalizes {
					finalizing := findCondition(fetchRelease(f.name), string(ConditionFinalizing))
					Expect(finalizing.Reason).To(Equal(string(ReasonCleanupFailed)))
					Expect(finalizing.Message).To(ContainSubstring("failed to give back"))
				}

				f.mustPass()
				expectGivenBack(objs[0])
				expectDeleted(objs[2])
			})
		})
	}

	Describe("a claimed object that isn't rendered", func() {
		for i, mode := range []openchoreov1alpha1.RenderedReleaseMode{manage, observe} {
			start := func(name string) *unstructured.Unstructured {
				GinkgoHelper()
				f = newReleaseFixture(fmt.Sprintf("claims-%s-%d", name, i), "")
				DeferCleanup(func() { _ = k8sClient.Delete(ctx, f.object("a")) })
				return f.object("a")
			}
			expectOnlyRead := func(health openchoreov1alpha1.HealthStatus) {
				GinkgoHelper()
				Expect(f.writes()).To(BeEmpty())
				observed := fetchRelease(f.name).Status.ObservedResources
				Expect(observed).To(HaveLen(1))
				Expect(observed[0].HealthStatus).To(Equal(health))
			}

			It(fmt.Sprintf("in %s mode: is reported when it is missing", mode), func() {
				a := start("missing")
				f.change(inMode(mode), claiming(a))
				f.mustPass()
				expectOnlyRead(openchoreov1alpha1.HealthStatusUnknown)
			})

			It(fmt.Sprintf("in %s mode: is only read when it is outside", mode), func() {
				a := start("outside")
				Expect(k8sClient.Create(ctx, a, client.FieldOwner("helm"))).To(Succeed())
				f.change(inMode(mode), claiming(a))
				f.mustPass()
				expectOnlyRead(openchoreov1alpha1.HealthStatusHealthy)
				expectUnchanged(a)
			})

			It(fmt.Sprintf("in %s mode: is only read when another release owns it, also while that one claims nothing", mode), func() {
				a := start("another")
				a.SetLabels(map[string]string{
					labels.LabelKeyManagedBy:                ControllerName,
					labels.LabelKeyRenderedReleaseUID:       "another",
					labels.LabelKeyRenderedReleaseName:      "another",
					labels.LabelKeyRenderedReleaseNamespace: testNs,
				})
				Expect(k8sClient.Patch(ctx, a, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName))).To(Succeed())
				f.change(inMode(mode), claiming(f.object("a")))
				f.mustPass()
				expectOnlyRead(openchoreov1alpha1.HealthStatusHealthy)
				expectUnchanged(a)
			})

			It(fmt.Sprintf("in %s mode: is given back once when the release applied it", mode), func() {
				a := applyFirst(start("own"))[0]
				f.change(inMode(mode), rendering(), claiming(a))
				f.mustPass()
				expectGivenBack(a)
				Expect(fetchRelease(f.name).Status.Resources).To(BeEmpty())

				By("The next pass only reads it")
				f.mustPass()
				expectOnlyRead(openchoreov1alpha1.HealthStatusHealthy)
			})

			It(fmt.Sprintf("in %s mode: is given back when an earlier release of this name applied it", mode), func() {
				a := applyFirst(start("recreated"))[0]
				envName := fetchRelease(f.name).Spec.EnvironmentName
				forceDelete(ctx, f.key())
				Expect(k8sClient.Create(ctx, makeMinimalRelease(f.name, envName))).To(Succeed())
				f.mustPass() // adds the finalizer
				f.change(inMode(mode), claiming(a))
				f.mustPass()
				expectGivenBack(a)
			})
		}
	})

	It("keeps what an earlier release of this name applied when it is deleted before its first apply", func() {
		f = newReleaseFixture("kept-recreated", "")
		a := applyFirst(f.object("a"))[0]
		envName := fetchRelease(f.name).Spec.EnvironmentName
		forceDelete(ctx, f.key())
		Expect(k8sClient.Create(ctx, makeMinimalRelease(f.name, envName))).To(Succeed())
		f.mustPass() // adds the finalizer
		f.deleteRelease()
		f.mustPass()
		Expect(released()).To(BeTrue())
		expectUnchanged(a)
	})

	Describe("a claim is matched by object", func() {
		hpa := func(version string) *unstructured.Unstructured {
			obj := unstructuredObj("autoscaling/"+version, "HorizontalPodAutoscaler", f.own, "api")
			obj.Object["spec"] = map[string]any{
				"maxReplicas":    int64(3),
				"scaleTargetRef": map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "name": "api"},
			}
			return obj
		}
		leave := func(claim *unstructured.Unstructured) {
			GinkgoHelper()
			f.change(inMode(observe), claiming(claim))
			f.mustPass()
		}

		It("in another API version, once", func() {
			f = newReleaseFixture("match-version", "")
			applied := applyFirst(hpa("v2"))[0] // it is listed as v2 and its claim is read as v1
			leave(hpa("v1"))
			expectGivenBack(applied)
			Expect(f.sentThat(isGiveBack)).To(HaveLen(1))
		})

		It("in an API version the plane does not serve", func() {
			f = newReleaseFixture("match-unserved", "")
			applied := applyFirst(hpa("v2"))[0]
			leave(hpa("v2beta1"))
			expectGivenBack(applied)
		})

		It("in any case of the kind", func() {
			f = newReleaseFixture("match-case", "")
			applied := applyFirst(f.object("a"))[0]
			claim := f.object("a")
			claim.SetKind("CONFIGMAP")
			leave(claim)
			expectGivenBack(applied)
		})

		It("but never a cluster-scoped object by a claim with a namespace", func() {
			f = newReleaseFixture("match-scope", "")
			role := unstructuredObj("rbac.authorization.k8s.io/v1", "ClusterRole", "", "match-scope")
			role.Object["rules"] = []any{}
			applied := applyFirst(role)[0]
			claim := unstructuredObj("rbac.authorization.k8s.io/v1", "ClusterRole", f.own, "match-scope")
			leave(claim)
			expectDeleted(applied)
		})

		It("and a cluster-scoped object by one without", func() {
			f = newReleaseFixture("match-cluster", "")
			role := unstructuredObj("rbac.authorization.k8s.io/v1", "ClusterRole", "", "match-cluster")
			role.Object["rules"] = []any{}
			applied := applyFirst(role)[0]
			leave(role)
			expectGivenBack(applied)
		})

		It("of a kind that is not listed, by reading the claim", func() {
			f = newReleaseFixture("match-unlisted", "")
			widget := unstructuredObj("test.openchoreo.dev/v1", "Widget", f.own, "w")
			widget.Object["spec"] = map[string]any{"size": "small"}
			applied := applyFirst(widget)[0]
			release := fetchRelease(f.name)
			release.Status.Resources = nil // as after a pass that failed
			Expect(k8sClient.Status().Update(ctx, release)).To(Succeed())
			leave(widget)
			expectGivenBack(applied)
		})

		It("and a kind the plane does not serve blocks neither a pass nor the finalizer", func() {
			f = newReleaseFixture("match-nothing", "")
			applied := applyFirst(f.object("a"))[0]
			f.change(claiming(unstructuredObj("nope.example.com/v1", "Gizmo", f.own, "g")))
			f.mustPass()
			expectUnchanged(applied)
			f.deleteRelease()
			f.mustPass()
			expectDeleted(applied)
			f.mustPass()
			Expect(released()).To(BeTrue())
		})
	})

	Describe("an object renamed under its ID", func() {
		// renamed renders an object named b under the ID a.
		renamed := func() func(*openchoreov1alpha1.RenderedReleaseSpec) {
			GinkgoHelper()
			render := rendering(f.object("b"))
			return func(spec *openchoreov1alpha1.RenderedReleaseSpec) {
				render(spec)
				spec.Resources[0].ID = "a"
			}
		}

		It("is given back under its old name when it is claimed", func() {
			f = newReleaseFixture("renamed-claimed", "")
			applied := applyFirst(f.object("a"))[0]
			f.change(renamed(), claiming(f.object("a")))
			f.mustPass()
			expectGivenBack(applied)
			Expect(f.live(f.object("b"))).NotTo(BeNil())
		})

		It("is kept as it is under its old name when it is not", func() {
			f = newReleaseFixture("renamed", "")
			applied := applyFirst(f.object("a"))[0]
			f.change(renamed())
			f.mustPass()
			expectUnchanged(applied)
		})
	})

	It("gives back nothing of what it still renders, whatever its ID", func() {
		f = newReleaseFixture("kept-new-id", "")
		a := applyFirst(f.object("a"))[0]
		label(a)
		render := rendering(f.object("a"))
		f.change(func(spec *openchoreov1alpha1.RenderedReleaseSpec) {
			render(spec)
			spec.Resources[0].ID = "new-id"
		})
		f.mustPass()
		Expect(f.live(a).GetLabels()).To(HaveKeyWithValue(labels.LabelKeyRenderedReleaseResourceID, "new-id"))
		Expect(f.live(a).GetLabels()).To(HaveKey(labels.LabelKeyClaimed))
		Expect(f.sentThat(isGiveBack)).To(BeEmpty())
		Expect(f.deletes()).To(BeEmpty())
	})

	It("gives back nothing while it refuses to apply", func() {
		f = newReleaseFixture("kept-refused", "")
		elsewhere := unstructuredObj("v1", "ConfigMap", testNs, "kept-refused")
		a := applyFirst(elsewhere)[0]
		label(a)
		a = f.live(a)
		taken := unstructuredObj("v1", "ConfigMap", testNs, "kept-refused-taken")
		Expect(k8sClient.Create(ctx, taken, client.FieldOwner("helm"))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, taken) })
		f.render(unstructuredObj("v1", "ConfigMap", testNs, "kept-refused-taken"))
		f.mustPass()
		Expect(findCondition(fetchRelease(f.name), ConditionResourcesApplied).Reason).To(Equal(ReasonApplyRefused))
		expectUnchanged(a)
	})

	It("says what it gave back", func() {
		f = newReleaseFixture("kept-names", "")
		applyFirst(f.object("a"))
		f.change(inMode(observe), claiming(f.object("a")))
		f.proxy.hook(planeHook{match: isGiveBack, answer: &planeAnswer{status: http.StatusForbidden, contentType: "text/plain", body: "no"}})
		_, err := f.pass()
		Expect(err).To(MatchError(ContainSubstring("failed to give back ConfigMap " + f.own + "/a")))
		Expect(strings.Count(err.Error(), "failed to give back")).To(Equal(1))
	})
})
