// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("RenderedRelease guard", func() {
	var f *releaseFixture
	// elsewhere is a namespace that is not the release's own.
	var elsewhere string
	start := func(name string) {
		GinkgoHelper()
		f = newReleaseFixture("guard-"+name, "")
		elsewhere = "elsewhere-" + name
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: elsewhere}})).To(Succeed())
	}

	configMap := func(name, value string) *unstructured.Unstructured {
		obj := unstructuredObj("v1", "ConfigMap", elsewhere, name)
		obj.Object["data"] = map[string]any{"by": value}
		return obj
	}
	// outside creates an object no release applied.
	outside := func(obj *unstructured.Unstructured) *unstructured.Unstructured {
		GinkgoHelper()
		created := obj.DeepCopy()
		Expect(k8sClient.Create(ctx, created, client.FieldOwner("helm"))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj.DeepCopy()) })
		return created
	}
	// appliedBy applies an object the way another release does.
	appliedBy := func(namespace, name string, obj *unstructured.Unstructured) *unstructured.Unstructured {
		GinkgoHelper()
		applied := obj.DeepCopy()
		applied.SetLabels(map[string]string{
			labels.LabelKeyManagedBy:                 ControllerName,
			labels.LabelKeyRenderedReleaseResourceID: "x",
			labels.LabelKeyRenderedReleaseUID:        "another-uid",
			labels.LabelKeyRenderedReleaseName:       name,
			labels.LabelKeyRenderedReleaseNamespace:  namespace,
		})
		Expect(k8sClient.Patch(ctx, applied, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName))).To(Succeed())
		return applied
	}
	expectUnchanged := func(obj *unstructured.Unstructured) {
		GinkgoHelper()
		Expect(f.live(obj).GetResourceVersion()).To(Equal(obj.GetResourceVersion()))
	}
	// expectRefused checks that the pass refused, wrote nothing and comes back.
	expectRefused := func(result reconcile.Result, err error, messages ...string) {
		GinkgoHelper()
		Expect(err).NotTo(HaveOccurred(), "a refusal is not an error")
		Expect(f.writes()).To(BeEmpty())
		Expect(result.RequeueAfter).To(BeNumerically(">=", 10*time.Second))
		Expect(result.RequeueAfter).To(BeNumerically("<", 12*time.Second))
		Expect(result.Priority).To(HaveValue(Equal(requeuePriorityPoll)))

		release := fetchRelease(f.name)
		applied := findCondition(release, ConditionResourcesApplied)
		Expect(applied.Status).To(Equal(metav1.ConditionFalse))
		Expect(applied.Reason).To(Equal(ReasonApplyRefused))
		Expect(applied.ObservedGeneration).To(Equal(release.Generation))
		for _, message := range messages {
			Expect(applied.Message).To(ContainSubstring(message))
		}
		Expect(ApplyFailedCondition(release)).To(HaveField("Message", applied.Message), "the bindings show it")
	}
	expectApplied := func(_ reconcile.Result, err error) {
		GinkgoHelper()
		Expect(err).NotTo(HaveOccurred())
		Expect(findCondition(fetchRelease(f.name), ConditionResourcesApplied).Reason).To(Equal(ReasonApplySucceeded))
	}

	Describe("what it does with an object elsewhere", func() {
		deployment := func(replicas int64) *unstructured.Unstructured {
			obj := unstructuredObj("apps/v1", "Deployment", elsewhere, "api")
			obj.Object["spec"] = map[string]any{
				"replicas": replicas,
				"selector": map[string]any{"matchLabels": map[string]any{"app": "api"}},
				"template": map[string]any{
					"metadata": map[string]any{"labels": map[string]any{"app": "api"}},
					"spec":     map[string]any{"containers": []any{map[string]any{"name": "api", "image": "api:1"}}},
				},
			}
			return obj
		}
		cases := []struct {
			name   string
			exists func(rendered ...*unstructured.Unstructured)
			owned  bool
		}{
			{"missing", func(...*unstructured.Unstructured) {}, true},
			{"its own, with a status manager", func(rendered ...*unstructured.Unstructured) {
				f.render(rendered...)
				f.mustPass()
				live := &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(deployment(1)), live)).To(Succeed())
				live.Status.ObservedGeneration = live.Generation
				Expect(k8sClient.Status().Update(ctx, live, client.FieldOwner("kube-controller-manager"))).To(Succeed())
			}, true},
			{"outside", func(...*unstructured.Unstructured) { outside(deployment(1)) }, false},
			{"another release's", func(...*unstructured.Unstructured) { appliedBy(testNs, "web-prod", deployment(1)) }, false},
			{"of a release with its name in another control plane namespace",
				func(...*unstructured.Unstructured) { appliedBy("globex", f.name, deployment(1)) }, false},
		}
		for i, c := range cases {
			It("guard off, "+c.name+": applies, and reads nothing", func() {
				start(fmt.Sprintf("off-%d", i))
				own := f.object("own") // this turns the guard off
				c.exists(own, deployment(1))
				f.render(own, deployment(2))

				expectApplied(f.pass())
				Expect(f.reads()).To(BeEmpty())
				live := f.live(deployment(1))
				Expect(live.GetLabels()).To(HaveKeyWithValue(labels.LabelKeyRenderedReleaseName, f.name))
				Expect(live.GetLabels()).To(HaveKeyWithValue(labels.LabelKeyRenderedReleaseNamespace, testNs))
				Expect(live.Object).To(HaveKeyWithValue("spec", HaveKeyWithValue("replicas", int64(2))))
			})

			It("guard on, "+c.name+": applies only what it owns, after one read", func() {
				start(fmt.Sprintf("on-%d", i))
				c.exists(deployment(1))
				before := f.live(deployment(1))
				f.render(deployment(2))

				result, err := f.pass()
				Expect(f.reads()).To(HaveLen(1))
				if !c.owned {
					expectRefused(result, err, "Deployment.apps "+elsewhere+"/api")
					expectUnchanged(before)
					return
				}
				expectApplied(result, err)
				Expect(f.live(deployment(1)).Object).To(HaveKeyWithValue("spec", HaveKeyWithValue("replicas", int64(2))))
			})
		}
	})

	Describe("what turns it off", func() {
		clusterRole := func() *unstructured.Unstructured {
			obj := unstructuredObj("rbac.authorization.k8s.io/v1", "ClusterRole", "", "guard-role")
			obj.Object["rules"] = []any{}
			return obj
		}
		clusterWidget := func() *unstructured.Unstructured {
			return unstructuredObj("test.openchoreo.dev/v1", "ClusterWidget", "", "guard-widget")
		}
		ownNamespace := func() *unstructured.Unstructured { return unstructuredObj("v1", "Namespace", "", f.own) }

		// existing is an object outside, so it is adopted with the guard off.
		DescribeTable("with an existing cluster-scoped object",
			func(name string, existing func() *unstructured.Unstructured, rendered func() []*unstructured.Unstructured, refused bool) {
				start(name)
				before := outside(existing())
				f.render(append(rendered(), existing())...)

				result, err := f.pass()
				if refused {
					expectRefused(result, err, describeObject(existing()))
					expectUnchanged(before)
					return
				}
				expectApplied(result, err)
				Expect(f.live(existing()).GetLabels()).To(HaveKeyWithValue(labels.LabelKeyRenderedReleaseName, f.name))
			},
			Entry("an object in the own namespace", "role-own", clusterRole, func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{f.object("own")}
			}, false),
			Entry("the own Namespace", "role-namespace", clusterRole, func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{ownNamespace()}
			}, false),
			Entry("an object in the own namespace, and one elsewhere", "role-both", clusterRole, func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{configMap("a", "release"), f.object("own")}
			}, false),
			Entry("not an object elsewhere", "role-elsewhere", clusterRole, func() []*unstructured.Unstructured {
				return []*unstructured.Unstructured{configMap("a", "release")}
			}, true),
			Entry("not a cluster-scoped object", "role-only", clusterRole, func() []*unstructured.Unstructured { return nil }, true),
			Entry("not a cluster-scoped custom resource", "widget-only", clusterWidget, func() []*unstructured.Unstructured { return nil }, true),
		)
	})

	Describe("a refusal", func() {
		It("writes nothing when the last of three objects is refused, and keeps the inventory", func() {
			start("last-of-three")
			f.render(configMap("a", "1"), configMap("b", "1"))
			f.mustPass()
			a, b := f.live(configMap("a", "")), f.live(configMap("b", ""))
			c := outside(configMap("c", "helm"))
			f.render(configMap("a", "2"), configMap("b", "2"), configMap("c", "2"))

			result, err := f.pass()
			expectRefused(result, err, "ConfigMap "+elsewhere+"/c exists and is not managed by this release")
			expectUnchanged(a)
			expectUnchanged(b)
			expectUnchanged(c)
			Expect(fetchRelease(f.name).Status.Resources).To(HaveLen(2))

			By("The object goes away, and the next pass applies all three")
			Expect(k8sClient.Delete(ctx, configMap("c", ""))).To(Succeed())
			expectApplied(f.pass())
			Expect(fetchRelease(f.name).Status.Resources).To(HaveLen(3))
		})

		It("is over when the next pass applies, even if that pass then fails", func() {
			start("then-fails")
			outside(configMap("c", "helm"))
			f.render(configMap("a", "1"), configMap("c", "1"))
			expectRefused(f.pass())
			Expect(k8sClient.Delete(ctx, configMap("c", ""))).To(Succeed())
			f.proxy.hook(planeHook{match: isList, answer: &planeAnswer{status: http.StatusBadGateway, contentType: "text/plain", body: "no agents"}})

			_, err := f.pass()
			Expect(err).To(MatchError(ContainSubstring("no agents")))
			Expect(f.live(configMap("c", ""))).NotTo(BeNil(), "applied")
		})

		It("cleans up nothing", func() {
			start("no-cleanup")
			f.render(configMap("a", "1"))
			f.mustPass()
			a := f.live(configMap("a", ""))
			outside(configMap("c", "helm"))
			f.render(configMap("c", "1"))

			result, err := f.pass()
			expectRefused(result, err)
			expectUnchanged(a)
		})

		It("names every object and whose it is, but not a release of another control plane namespace", func() {
			start("names")
			outside(configMap("a", "helm"))
			appliedBy(testNs, "web-prod", configMap("b", "1"))
			appliedBy("globex", "api-prod", configMap("c", "1"))
			f.render(configMap("a", "2"), configMap("b", "2"), configMap("c", "2"), configMap("new", "2"))

			result, err := f.pass()
			expectRefused(result, err,
				"ConfigMap "+elsewhere+"/a exists and is not managed by this release",
				"ConfigMap "+elsewhere+"/b belongs to RenderedRelease web-prod",
				"ConfigMap "+elsewhere+"/c belongs to another RenderedRelease")
			message := findCondition(fetchRelease(f.name), ConditionResourcesApplied).Message
			Expect(message).NotTo(ContainSubstring("globex"))
			Expect(message).NotTo(ContainSubstring("api-prod"))
			Expect(message).NotTo(ContainSubstring("/new"))
		})

		It("names ten objects and counts the rest", func() {
			start("many")
			rendered := make([]*unstructured.Unstructured, 0, 12)
			for i := range 12 {
				outside(configMap(fmt.Sprintf("cm-%02d", i), "helm"))
				rendered = append(rendered, configMap(fmt.Sprintf("cm-%02d", i), "2"))
			}
			f.render(rendered...)

			result, err := f.pass()
			expectRefused(result, err, "/cm-09 ", "and 2 more")
			Expect(findCondition(fetchRelease(f.name), ConditionResourcesApplied).Message).NotTo(ContainSubstring("/cm-10 "))
		})

		It("waits for the next change with the progressing interval set to 0", func() {
			start("interval-0")
			outside(configMap("a", "helm"))
			f.render(configMap("a", "1"))
			release := fetchRelease(f.name)
			release.Spec.ProgressingInterval = &metav1.Duration{}
			Expect(k8sClient.Update(ctx, release)).To(Succeed())

			result, err := f.pass()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(findCondition(fetchRelease(f.name), ConditionResourcesApplied).Reason).To(Equal(ReasonApplyRefused))
		})

		It("still publishes what the release observes", func() {
			start("observes")
			outside(configMap("a", "helm"))
			outside(configMap("seen", "helm"))
			f.render(configMap("a", "1"))
			release := fetchRelease(f.name)
			release.Spec.ObservedResources = []openchoreov1alpha1.ObservedResource{
				{ID: "seen", APIVersion: "v1", Kind: "ConfigMap", Namespace: elsewhere, Name: "seen"},
			}
			Expect(k8sClient.Update(ctx, release)).To(Succeed())

			result, err := f.pass()
			expectRefused(result, err)
			Expect(fetchRelease(f.name).Status.ObservedResources).To(HaveLen(1))
		})
	})

	Describe("a read that fails", func() {
		status := func(code int, reason metav1.StatusReason) *planeAnswer {
			return &planeAnswer{status: code, contentType: "application/json", body: fmt.Sprintf(
				`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":%q,"code":%d,"message":"no"}`, reason, code)}
		}
		isRead := func(r *planeRequest) bool { return r.method == http.MethodGet && !isList(r) }

		DescribeTable("fails the apply, and nothing is written",
			func(name string, answer *planeAnswer) {
				start("read-" + name)
				f.render(configMap("a", "1"), configMap("b", "1"))
				f.proxy.hook(planeHook{match: isRead, answer: answer})

				_, err := f.pass()
				Expect(err).To(MatchError(ContainSubstring("ConfigMap " + elsewhere + "/a")))
				Expect(f.writes()).To(BeEmpty())
				Expect(findCondition(fetchRelease(f.name), ConditionResourcesApplied).Reason).To(Equal(ReasonApplyFailed))

				expectApplied(f.pass())
				Expect(f.live(configMap("b", ""))).NotTo(BeNil())
			},
			Entry("403 from the API server", "403", status(http.StatusForbidden, metav1.StatusReasonForbidden)),
			Entry("403 from the gateway", "403-text", &planeAnswer{status: http.StatusForbidden, contentType: "text/plain", body: "Forbidden: Agent not authorized"}),
			Entry("502 from the gateway", "502", &planeAnswer{status: http.StatusBadGateway, contentType: "text/plain", body: "no agents"}),
			Entry("429 from the API server", "429", status(http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests)),
		)

		It("counts as missing when it is a 404, whoever answers it", func() {
			start("read-404-text")
			f.render(configMap("a", "1"))
			notFound := f.proxy.hook(planeHook{match: isRead, answer: &planeAnswer{status: http.StatusNotFound, contentType: "text/plain", body: "404 page not found"}})
			expectApplied(f.pass())
			Expect(notFound.used).To(BeTrue(), "the read was answered so")
			Expect(f.live(configMap("a", ""))).NotTo(BeNil())
		})

		It("changes nothing of an object that exists when its read answers 404", func() {
			start("read-404-exists")
			theirs := outside(configMap("a", "helm"))
			f.render(configMap("a", "1"))
			f.proxy.hook(planeHook{match: isRead, answer: &planeAnswer{status: http.StatusNotFound, contentType: "text/plain", body: "404 page not found"}})

			_, err := f.pass()
			Expect(err).To(MatchError(ContainSubstring("/a appeared while it was being created")))
			expectUnchanged(theirs)
		})

		It("wins over a refusal, since more may be refused", func() {
			start("read-and-refusal")
			outside(configMap("a", "helm"))
			f.render(configMap("a", "1"), configMap("b", "1"))
			f.proxy.hook(planeHook{match: func(r *planeRequest) bool { return isRead(r) && strings.HasSuffix(r.path, "/b") },
				answer: &planeAnswer{status: http.StatusBadGateway, contentType: "text/plain", body: "no agents"}})

			_, err := f.pass()
			Expect(err).To(HaveOccurred())
			Expect(f.writes()).To(BeEmpty())
			Expect(findCondition(fetchRelease(f.name), ConditionResourcesApplied).Reason).To(Equal(ReasonApplyFailed))
		})
	})

	Describe("what happens between the check and the apply", func() {
		// beforeApplying runs a change right before the apply of an object.
		beforeApplying := func(obj *unstructured.Unstructured, change func()) {
			GinkgoHelper()
			f.proxy.hook(planeHook{match: func(r *planeRequest) bool {
				return r.method == http.MethodPatch && strings.HasSuffix(r.path, "/"+obj.GetName())
			}, before: change})
		}
		expectFailed := func(err error, messages ...string) {
			GinkgoHelper()
			applied := findCondition(fetchRelease(f.name), ConditionResourcesApplied)
			Expect(applied.Reason).To(Equal(ReasonApplyFailed))
			for _, message := range messages {
				Expect(err).To(MatchError(ContainSubstring(message)))
				Expect(applied.Message).To(ContainSubstring(message))
			}
		}
		applies := func() []*planeRequest {
			return f.sentThat(func(r *planeRequest) bool { return r.contentType == string(types.ApplyPatchType) })
		}

		It("names what it found by UID, and only that", func() {
			start("uid")
			f.render(configMap("found", "1"))
			f.mustPass()
			found := f.live(configMap("found", ""))
			f.render(configMap("found", "2"), configMap("new", "2"))

			expectApplied(f.pass())
			Expect(applies()).To(HaveLen(2))
			Expect(string(applies()[0].body)).To(ContainSubstring(`"uid":"` + string(found.GetUID()) + `"`))
			Expect(string(applies()[1].body)).NotTo(ContainSubstring(`"uid":`))
		})

		It("leaves its object alone when it is recreated", func() {
			start("recreated")
			f.render(configMap("a", "1"))
			f.mustPass()
			f.render(configMap("a", "2"))
			var recreated *unstructured.Unstructured
			beforeApplying(configMap("a", ""), func() {
				Expect(k8sClient.Delete(ctx, configMap("a", ""))).To(Succeed())
				recreated = outside(configMap("a", "helm"))
			})

			_, err := f.pass()
			expectFailed(err, "failed to apply resource a")
			Expect(applies()[0].status).To(Equal(http.StatusUnprocessableEntity))
			expectUnchanged(recreated)

			By("The next pass sees whose it is")
			result, err := f.pass()
			expectRefused(result, err, "/a exists and is not managed by this release")
		})

		It("creates nothing in place of its object when it is deleted", func() {
			start("deleted")
			f.render(configMap("a", "1"))
			f.mustPass()
			f.render(configMap("a", "2"))
			beforeApplying(configMap("a", ""), func() { Expect(k8sClient.Delete(ctx, configMap("a", ""))).To(Succeed()) })

			_, err := f.pass()
			expectFailed(err, "failed to apply resource a")
			Expect(applies()[0].status).To(Equal(http.StatusConflict))
			Expect(f.live(configMap("a", ""))).To(BeNil())

			By("The next pass creates it")
			expectApplied(f.pass())
			Expect(f.live(configMap("a", ""))).NotTo(BeNil())
		})

		It("applies its object when something else of it changes", func() {
			start("busy")
			f.render(configMap("a", "1"))
			f.mustPass()
			f.render(configMap("a", "2"))
			beforeApplying(configMap("a", ""), func() {
				busy := f.live(configMap("a", ""))
				busy.SetAnnotations(map[string]string{"example.com/refreshed": "now"})
				Expect(k8sClient.Update(ctx, busy, client.FieldOwner("operator"))).To(Succeed())
			})
			expectApplied(f.pass())
			Expect(f.live(configMap("a", "")).GetAnnotations()).To(HaveKeyWithValue("example.com/refreshed", "now"), "it changed")
		})

		withSpec := func(apiVersion, kind, spec string) func() *unstructured.Unstructured {
			return func() *unstructured.Unstructured {
				GinkgoHelper()
				obj := unstructuredObj(apiVersion, kind, elsewhere, "a")
				Expect(json.Unmarshal([]byte(spec), &obj.Object)).To(Succeed())
				obj.SetAPIVersion(apiVersion)
				obj.SetKind(kind)
				obj.SetNamespace(elsewhere)
				obj.SetName("a")
				return obj
			}
		}
		// An object that appears where the check found none is created by someone else in
		// between. Its values are kept: one that differs from the manifest fails the apply,
		// and nothing is written. When none differs, the apply merges into it, and it is
		// given back.
		DescribeTable("keeps an object that appears where there was none",
			func(name string, differs bool, theirs, rendered func() *unstructured.Unstructured) {
				start("appears-" + name)
				f.render(rendered())
				var created *unstructured.Unstructured
				beforeApplying(rendered(), func() { created = outside(theirs()) })

				_, err := f.pass()
				if differs {
					expectFailed(err, describeObject(rendered())+" appeared while it was being created", "conflict")
					expectUnchanged(created)
				} else {
					expectFailed(err, describeObject(rendered())+" appeared while it was being created, and was given back")
				}
				after := f.live(rendered())
				for key := range after.GetLabels() {
					Expect(markLabels.Has(key)).To(BeFalse(), "label %s is left", key)
				}
				Expect(hasApplyEntry(after)).To(BeFalse())
				Expect(after.GetManagedFields()).NotTo(BeEmpty(), "its creator's entry is kept")

				By("The next pass sees whose it is")
				result, err := f.pass()
				expectRefused(result, err, "exists and is not managed by this release")

				By("Deleting the release keeps it")
				f.deleteRelease()
				f.mustPass()
				Expect(f.live(rendered()).GetUID()).To(Equal(after.GetUID()))
			},
			Entry("ConfigMap with other values", "cm", true,
				withSpec("v1", "ConfigMap", `{"data": {"by": "helm", "theirs": "kept"}}`),
				withSpec("v1", "ConfigMap", `{"data": {"by": "release"}}`)),
			Entry("ConfigMap with the same values", "cm-same", false,
				withSpec("v1", "ConfigMap", `{"data": {"by": "release"}}`),
				withSpec("v1", "ConfigMap", `{"data": {"by": "release"}}`)),
			Entry("Deployment with other values", "deployment", true,
				withSpec("apps/v1", "Deployment", `{"spec": {"replicas": 1, "selector": {"matchLabels": {"app": "api"}},
					"template": {"metadata": {"labels": {"app": "api"}}, "spec": {"containers": [{"name": "api", "image": "api:1"}]}}}}`),
				withSpec("apps/v1", "Deployment", `{"spec": {"replicas": 2, "selector": {"matchLabels": {"app": "api"}},
					"template": {"metadata": {"labels": {"app": "api"}}, "spec": {"containers": [{"name": "api", "image": "api:2"}]}}}}`)),
			Entry("PersistentVolumeClaim with the same values", "pvc", false,
				withSpec("v1", "PersistentVolumeClaim", `{"spec": {"accessModes": ["ReadWriteOnce"], "resources": {"requests": {"storage": "1Gi"}}}}`),
				withSpec("v1", "PersistentVolumeClaim", `{"spec": {"accessModes": ["ReadWriteOnce"], "resources": {"requests": {"storage": "1Gi"}}}}`)),
			// The apply would take every field its creator set, and leave it no managedFields
			// entry to tell it apart from a new object.
			Entry("ServiceAccount whose only field the release sets too", "sa", true,
				withSpec("v1", "ServiceAccount", `{"automountServiceAccountToken": false}`),
				withSpec("v1", "ServiceAccount", `{"automountServiceAccountToken": true}`)),
		)

		It("stops at the object that appeared, and keeps what it created before it", func() {
			start("appears-second")
			f.render(configMap("a", "1"), configMap("b", "1"), configMap("c", "1"))
			beforeApplying(configMap("b", ""), func() { outside(configMap("b", "helm")) })

			_, err := f.pass()
			expectFailed(err, "/b appeared")
			Expect(hasApplyEntry(f.live(configMap("a", "")))).To(BeTrue())
			Expect(f.live(configMap("c", ""))).To(BeNil())
			Expect(fetchRelease(f.name).Status.Resources).To(BeEmpty(), "nothing was applied as a whole")
		})

		It("says so when an object that appeared can't be given back", func() {
			start("appears-kept")
			f.render(configMap("a", "1"))
			beforeApplying(configMap("a", ""), func() { outside(configMap("a", "1")) })
			f.proxy.hook(planeHook{match: func(r *planeRequest) bool { return r.contentType == string(types.JSONPatchType) },
				answer: &planeAnswer{status: http.StatusBadGateway, contentType: "text/plain", body: "no agents"}})

			_, err := f.pass()
			expectFailed(err, "/a appeared while it was being created", "failed to give back")
		})

		It("applies an object it renders twice, also when an operator gets to it in between", func() {
			start("twice")
			render := rendering(configMap("a", "1"), configMap("a", "2"))
			f.change(func(spec *openchoreov1alpha1.RenderedReleaseSpec) {
				render(spec)
				spec.Resources[1].ID = "again"
			})
			first := true
			f.proxy.hook(planeHook{match: func(r *planeRequest) bool {
				second := withMethod(http.MethodPatch)(r) && !first
				first = first && !withMethod(http.MethodPatch)(r)
				return second
			}, before: func() {
				created := f.live(configMap("a", ""))
				created.SetAnnotations(map[string]string{"example.com/seen": "yes"})
				Expect(k8sClient.Update(ctx, created, client.FieldOwner("operator"))).To(Succeed())
			}})

			expectApplied(f.pass())
			Expect(f.live(configMap("a", "")).Object).To(HaveKeyWithValue("data", HaveKeyWithValue("by", "2")), "the last one wins")
		})

		It("adopts an object that appears in its own namespace, as it adopts one that was there", func() {
			start("appears-own")
			f.render(f.object("a"))
			beforeApplying(f.object("a"), func() { outside(f.object("a")) })
			expectApplied(f.pass())
			Expect(hasApplyEntry(f.live(f.object("a")))).To(BeTrue())
		})
	})

	It("lets one of two releases that shared an object before it apply, and deleting the other keeps the object", func() {
		start("shared")
		shared := appliedBy(testNs, "release-guard-shared-first", configMap("shared", "1"))
		// The second release had applied it too, so it is in its inventory.
		release := fetchRelease(f.name)
		release.Status.Resources = []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "shared", Version: "v1", Kind: "ConfigMap", Namespace: elsewhere, Name: "shared"},
		}
		Expect(k8sClient.Status().Update(ctx, release)).To(Succeed())
		f.render(configMap("shared", "2"))

		result, err := f.pass()
		expectRefused(result, err, "belongs to RenderedRelease release-guard-shared-first")
		expectUnchanged(shared)

		f.deleteRelease()
		f.mustPass()
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, f.key(), &openchoreov1alpha1.RenderedRelease{}))).To(BeTrue())
		expectUnchanged(shared)
	})

	Describe("on the observability plane", func() {
		startOP := func(name string) {
			GinkgoHelper()
			f = newReleaseFixture("guard-op-"+name, targetPlaneObservabilityPlane)
			elsewhere = "elsewhere-op-" + name
		}
		rule := func(namespace, name string) *unstructured.Unstructured {
			obj := f.object(name)
			obj.SetNamespace(namespace)
			return obj
		}

		It("creates the namespace of a release it lets through", func() {
			startOP("new")
			f.render(rule(elsewhere, "a"))
			expectApplied(f.pass())
			Expect(f.live(rule(elsewhere, "a"))).NotTo(BeNil())
		})

		It("creates no namespace when it refuses", func() {
			startOP("refused")
			taken := "taken-op-refused"
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: taken}})).To(Succeed())
			existing := outside(rule(taken, "a"))
			f.render(rule(elsewhere, "new"), rule(taken, "a"))

			result, err := f.pass()
			expectRefused(result, err, "ObservabilityAlertRule.openchoreo.dev "+taken+"/a")
			expectUnchanged(existing)
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Name: elsewhere}, &corev1.Namespace{}))).To(BeTrue())
		})
	})

	It("does not hold a release in Observe mode, which applies nothing", func() {
		start("observe")
		before := outside(configMap("a", "helm"))
		f.render(configMap("a", "1"))
		setMode(f.name, openchoreov1alpha1.RenderedReleaseModeObserve)

		f.mustPass()
		Expect(f.reads()).To(BeEmpty())
		expectUnchanged(before)
		Expect(findCondition(fetchRelease(f.name), ConditionResourcesApplied).Reason).To(Equal(ReasonObserveMode))
	})
})
