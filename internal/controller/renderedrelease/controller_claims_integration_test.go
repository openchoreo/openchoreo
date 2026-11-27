// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("RenderedRelease that renders what it claims", func() {
	var f *releaseFixture
	// namespace is where the release renders: its own namespace, or one that isn't, with
	// the guard on.
	var namespace string
	start := func(name string, guarded bool) {
		GinkgoHelper()
		f = newReleaseFixture(fmt.Sprintf("claim-%s-%t", name, guarded), "")
		namespace = f.own
		if guarded {
			namespace = fmt.Sprintf("claim-%s-elsewhere", name)
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())
		}
	}

	configMap := func(value string) *unstructured.Unstructured {
		obj := unstructuredObj("v1", "ConfigMap", namespace, "a")
		obj.Object["data"] = map[string]any{"by": value}
		return obj
	}
	outside := func(obj *unstructured.Unstructured) *unstructured.Unstructured {
		GinkgoHelper()
		created := obj.DeepCopy()
		Expect(k8sClient.Create(ctx, created, client.FieldOwner("helm"))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj.DeepCopy()) })
		return created
	}
	appliedBy := func(releaseName string, obj *unstructured.Unstructured) *unstructured.Unstructured {
		GinkgoHelper()
		applied := obj.DeepCopy()
		applied.SetLabels(map[string]string{
			labels.LabelKeyManagedBy:                 ControllerName,
			labels.LabelKeyRenderedReleaseResourceID: "x",
			labels.LabelKeyRenderedReleaseUID:        "another-uid",
			labels.LabelKeyRenderedReleaseName:       releaseName,
			labels.LabelKeyRenderedReleaseNamespace:  testNs,
		})
		Expect(k8sClient.Patch(ctx, applied, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName))).To(Succeed())
		return applied
	}
	// applyUnclaimed makes the release apply an object it doesn't claim yet.
	applyUnclaimed := func(obj *unstructured.Unstructured) *unstructured.Unstructured {
		GinkgoHelper()
		f.change(rendering(obj), claiming())
		f.mustPass()
		return f.live(obj)
	}
	expectUnchanged := func(obj *unstructured.Unstructured) {
		GinkgoHelper()
		Expect(f.live(obj).GetResourceVersion()).To(Equal(obj.GetResourceVersion()))
	}
	// expectApplied checks that the pass applied the object, and returns it.
	expectApplied := func(value string) *unstructured.Unstructured {
		GinkgoHelper()
		release := fetchRelease(f.name)
		Expect(findCondition(release, ConditionResourcesApplied).Reason).To(Equal(ReasonApplySucceeded))
		Expect(release.Status.Resources).To(HaveLen(1))
		live := f.live(configMap(""))
		Expect(live.Object).To(HaveKeyWithValue("data", HaveKeyWithValue("by", value)))
		Expect(live.GetLabels()).To(HaveKeyWithValue(labels.LabelKeyRenderedReleaseName, f.name))
		return live
	}
	isLabelRequest := func(r *planeRequest) bool { return strings.Contains(r.query, "fieldManager=renderedrelease-takeover") }
	// describe names the requests a pass sent.
	describe := func(requests []*planeRequest) []string {
		var sent []string
		for _, r := range requests {
			switch {
			case r.method == http.MethodGet:
				sent = append(sent, "read")
			case strings.Contains(r.query, "dryRun=All") && strings.Contains(string(r.body), `"uid":`):
				sent = append(sent, "dry run by UID")
			case isLabelRequest(r) && strings.Contains(string(r.body), `"uid":`):
				sent = append(sent, "label by UID")
			case r.contentType == string(types.ApplyPatchType) && strings.Contains(string(r.body), `"uid":`):
				sent = append(sent, "apply by UID")
			case r.contentType == string(types.ApplyPatchType):
				sent = append(sent, "create")
			default:
				sent = append(sent, r.method+" "+r.contentType)
			}
		}
		return sent
	}
	sent := func() []string {
		return describe(f.sentThat(func(r *planeRequest) bool { return !isList(r) }))
	}

	for _, guarded := range []bool{false, true} {
		where := map[bool]string{false: "in its own namespace", true: "elsewhere"}[guarded]

		It(where+": creates it when it is missing, and labels it at once", func() {
			start("missing", guarded)
			f.change(rendering(configMap("release")), claiming(configMap("")))

			f.mustPass()
			Expect(sent()).To(Equal([]string{"read", "read", "create", "label by UID"}), "the claim's read, the check, the apply, the label")
			Expect(expectApplied("release").GetLabels()).To(HaveKeyWithValue(labels.LabelKeyClaimed, "true"))
			Expect(fetchRelease(f.name).Status.ObservedResources).To(HaveLen(1))

			f.mustPass()
			Expect(sent()).To(Equal([]string{"read", "read", "apply by UID"}))
			Expect(fetchRelease(f.name).Status.ObservedResources[0].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusHealthy))
		})

		It(where+": labels on the next pass what it created but could not label", func() {
			start("label-after-create-fails", guarded)
			f.change(rendering(configMap("release")), claiming(configMap("")))
			f.proxy.hook(planeHook{match: isLabelRequest, answer: &planeAnswer{status: http.StatusBadGateway, contentType: "text/plain", body: "no agents"}})

			_, err := f.pass()
			Expect(err).To(MatchError(ContainSubstring("failed to label")))
			Expect(f.live(configMap("")).GetLabels()).NotTo(HaveKey(labels.LabelKeyClaimed))

			f.mustPass()
			Expect(sent()).To(Equal([]string{"read", "read", "label by UID", "apply by UID"}))
			Expect(expectApplied("release").GetLabels()).To(HaveKeyWithValue(labels.LabelKeyClaimed, "true"))
		})

		It(where+": labels what it applied when it is claimed later, and goes on applying it", func() {
			start("later", guarded)
			before := applyUnclaimed(configMap("release"))
			f.change(rendering(configMap("changed")), claiming(configMap("")))

			f.mustPass()
			Expect(sent()).To(Equal([]string{"read", "read", "label by UID", "apply by UID"}))
			after := expectApplied("changed")
			Expect(after.GetUID()).To(Equal(before.GetUID()))
			Expect(after.GetLabels()).To(HaveKeyWithValue(labels.LabelKeyClaimed, "true"))
		})

		It(where+": refuses what another release applied", func() {
			start("another", guarded)
			theirs := appliedBy("web-prod", configMap("theirs"))
			f.change(rendering(configMap("release")), claiming(configMap("")))

			f.mustPass()
			Expect(f.writes()).To(BeEmpty())
			expectUnchanged(theirs)
			applied := findCondition(fetchRelease(f.name), ConditionResourcesApplied)
			Expect(applied.Reason).To(Equal(ReasonApplyRefused))
			Expect(applied.Message).To(ContainSubstring("ConfigMap " + namespace + "/a belongs to RenderedRelease web-prod"))
			Expect(fetchRelease(f.name).Status.ObservedResources).To(HaveLen(1), "it is still read")
		})

		It(where+": takes over what no release applied", func() {
			start("outside", guarded)
			theirs := configMap("theirs")
			theirs.Object["data"] = map[string]any{"by": "theirs", "theirs": "kept"}
			theirs.SetLabels(map[string]string{"app.kubernetes.io/managed-by": "Helm"})
			before := outside(theirs)
			f.change(rendering(configMap("release")), claiming(configMap("")))

			f.mustPass()
			Expect(sent()).To(Equal([]string{"read", "read", "dry run by UID", "label by UID", "apply by UID"}))
			after := expectApplied("release")
			Expect(after.GetUID()).To(Equal(before.GetUID()))
			Expect(after.GetLabels()).To(HaveKeyWithValue(labels.LabelKeyClaimed, "true"))
			Expect(after.GetLabels()).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "Helm"), "what the release doesn't set stays")
			Expect(after.Object).To(HaveKeyWithValue("data", HaveKeyWithValue("theirs", "kept")))
			status := fetchRelease(f.name).Status
			Expect(status.Resources).To(HaveLen(1))
			Expect(status.ObservedResources).To(HaveLen(1), "it is in both lists")

			f.mustPass()
			Expect(sent()).To(Equal([]string{"read", "read", "apply by UID"}), "only the first pass takes it over")
		})

		It(where+": leaves what appears while it creates it, and takes it over on the next pass", func() {
			start("appears", guarded)
			f.change(rendering(configMap("release")), claiming(configMap("")))
			var theirs *unstructured.Unstructured
			f.proxy.hook(planeHook{match: withMethod(http.MethodPatch), before: func() { theirs = outside(configMap("theirs")) }})

			_, err := f.pass()
			Expect(err).To(MatchError(ContainSubstring("appeared while it was being created")))
			expectUnchanged(theirs)

			f.mustPass()
			Expect(sent()).To(Equal([]string{"read", "read", "dry run by UID", "label by UID", "apply by UID"}))
			Expect(expectApplied("release").GetUID()).To(Equal(theirs.GetUID()))
		})

		for _, answer := range []*planeAnswer{
			{status: http.StatusForbidden, contentType: "application/json",
				body: `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403,"message":"no"}`},
			{status: http.StatusForbidden, contentType: "text/plain", body: "Forbidden: Agent not authorized"},
			{status: http.StatusBadGateway, contentType: "text/plain", body: "no agents"},
			{status: http.StatusTooManyRequests, contentType: "application/json",
				body: `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"TooManyRequests","code":429,"message":"no"}`},
		} {
			It(fmt.Sprintf("%s: writes nothing while it can't be read (%d %s)", where, answer.status, answer.contentType), func() {
				start(fmt.Sprintf("read-%d-%s", answer.status, strings.ReplaceAll(answer.contentType, "/", "-")), guarded)
				f.change(rendering(configMap("release")), claiming(configMap("")))
				f.proxy.hook(planeHook{match: func(r *planeRequest) bool { return r.method == http.MethodGet && !isList(r) }, every: true, answer: answer})

				_, err := f.pass()
				Expect(err).To(MatchError(ContainSubstring("failed to check ConfigMap " + namespace + "/a")))
				Expect(f.writes()).To(BeEmpty())
				Expect(findCondition(fetchRelease(f.name), ConditionResourcesApplied).Reason).To(Equal(ReasonApplyFailed))
			})
		}
	}

	It("reads only what it claims in its own namespace", func() {
		start("unclaimed-next-to-it", false)
		other := f.object("other")
		f.change(rendering(configMap("release"), other), claiming(configMap("")))
		f.mustPass()
		Expect(sent()).To(Equal([]string{"read", "read", "create", "label by UID", "create"}))
		Expect(f.reads()[1].path).To(HaveSuffix("/a"))
	})

	It("does not apply it when it can't be labeled", func() {
		start("label-fails", false)
		applyUnclaimed(configMap("release"))
		f.change(rendering(configMap("changed")), claiming(configMap("")))
		f.proxy.hook(planeHook{match: isLabelRequest, answer: &planeAnswer{status: http.StatusForbidden, contentType: "text/plain", body: "Forbidden: no"}})

		_, err := f.pass()
		Expect(err).To(MatchError(ContainSubstring("failed to label ConfigMap " + namespace + "/a as claimed")))
		Expect(f.live(configMap("")).Object).To(HaveKeyWithValue("data", HaveKeyWithValue("by", "release")))

		f.mustPass()
		expectApplied("changed")
	})

	It("leaves it alone when it is recreated before it is labeled", func() {
		start("label-recreated", false)
		applyUnclaimed(configMap("release"))
		f.change(rendering(configMap("changed")), claiming(configMap("")))
		var recreated *unstructured.Unstructured
		f.proxy.hook(planeHook{match: isLabelRequest, before: func() {
			Expect(k8sClient.Delete(ctx, configMap(""))).To(Succeed())
			recreated = outside(configMap("theirs"))
		}})

		_, err := f.pass()
		Expect(err).To(HaveOccurred())
		expectUnchanged(recreated)
	})

	It("does not label it again when the label has another value", func() {
		start("label-value", false)
		applied := applyUnclaimed(configMap("release"))
		applied.SetLabels(withLabel(applied.GetLabels(), labels.LabelKeyClaimed, "mine"))
		Expect(k8sClient.Update(ctx, applied, client.FieldOwner("kubectl-label"))).To(Succeed())
		f.change(claiming(configMap("")))
		f.mustPass()
		Expect(sent()).To(Equal([]string{"read", "read", "apply by UID"}))
		Expect(expectApplied("release").GetLabels()).To(HaveKeyWithValue(labels.LabelKeyClaimed, "mine"))
	})

	Describe("the label", func() {
		// claimed makes the release apply an object that is claimed and labeled.
		claimed := func(name string) *unstructured.Unstructured {
			GinkgoHelper()
			start("label-"+name, false)
			applyUnclaimed(configMap("release"))
			f.change(claiming(configMap("")))
			f.mustPass()
			live := f.live(configMap(""))
			Expect(live.GetLabels()).To(HaveKey(labels.LabelKeyClaimed))
			return live
		}

		It("stays when the claim is removed, and the object goes on being applied", func() {
			claimed("stays")
			f.change(rendering(configMap("changed")), claiming())
			f.mustPass()
			Expect(expectApplied("changed").GetLabels()).To(HaveKey(labels.LabelKeyClaimed))
		})

		It("protects the object after the claim is gone", func() {
			before := claimed("protects")
			f.change(claiming())
			f.mustPass()
			f.change(rendering())
			f.mustPass()
			after := f.live(before)
			Expect(after.GetUID()).To(Equal(before.GetUID()))
			Expect(hasApplyEntry(after)).To(BeFalse(), "given back")
		})

		It("protects it no longer once it is removed as well", func() {
			before := claimed("removed")
			f.change(claiming())
			unlabeled := f.live(before)
			unlabeled.SetLabels(withoutLabel(unlabeled.GetLabels(), labels.LabelKeyClaimed))
			Expect(k8sClient.Update(ctx, unlabeled, client.FieldOwner("kubectl-edit"))).To(Succeed())
			f.mustPass()
			Expect(f.live(before).GetLabels()).NotTo(HaveKey(labels.LabelKeyClaimed), "without a claim it is not added")
			f.change(rendering())
			f.mustPass()
			Expect(f.live(before)).To(BeNil())
		})

		It("comes back while the object is still claimed", func() {
			before := claimed("comes-back")
			unlabeled := f.live(before)
			unlabeled.SetLabels(withoutLabel(unlabeled.GetLabels(), labels.LabelKeyClaimed))
			Expect(k8sClient.Update(ctx, unlabeled, client.FieldOwner("kubectl-edit"))).To(Succeed())
			f.mustPass()
			Expect(f.live(before).GetLabels()).To(HaveKey(labels.LabelKeyClaimed))
		})
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
		// expectLabeled renders and claims an object that was applied before.
		expectLabeled := func(rendered, claim *unstructured.Unstructured, want bool) {
			GinkgoHelper()
			f.render(rendered)
			f.mustPass()
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rendered) })
			f.change(claiming(claim))
			f.mustPass()
			_, labeled := f.live(rendered).GetLabels()[labels.LabelKeyClaimed]
			Expect(labeled).To(Equal(want))
		}

		It("in another API version", func() {
			start("match-version", false)
			expectLabeled(hpa("v2"), hpa("v1"), true)
			Expect(f.sentThat(isLabelRequest)[0].path).To(ContainSubstring("/autoscaling/v2/"), "it is checked as it is rendered")
		})

		It("in an API version the plane does not serve, so a dropped version doesn't block the release", func() {
			start("match-unserved", false)
			expectLabeled(hpa("v2"), hpa("v2beta1"), true)
		})

		It("in any case of the kind", func() {
			start("match-case", false)
			claim := configMap("")
			claim.SetKind("CONFIGMAP")
			expectLabeled(configMap("release"), claim, true)
		})

		It("but never a cluster-scoped object by a claim with a namespace, even if its manifest sets one", func() {
			start("match-scope", false)
			role := unstructuredObj("rbac.authorization.k8s.io/v1", "ClusterRole", f.own, "claim-match-scope")
			role.Object["rules"] = []any{}
			expectLabeled(role, role, false)
		})

		It("and a cluster-scoped object by a claim without one, even if its manifest sets one", func() {
			start("match-cluster", false)
			role := unstructuredObj("rbac.authorization.k8s.io/v1", "ClusterRole", f.own, "claim-match-cluster")
			role.Object["rules"] = []any{}
			claim := unstructuredObj("rbac.authorization.k8s.io/v1", "ClusterRole", "", "claim-match-cluster")
			expectLabeled(role, claim, true)
		})

		It("and not an object of the same name in another namespace", func() {
			start("match-namespace", false)
			claim := configMap("")
			claim.SetNamespace(testNs)
			expectLabeled(configMap("release"), claim, false)
		})
	})

	Describe("the next check", func() {
		missing := func() *unstructured.Unstructured {
			return unstructuredObj("v1", "ConfigMap", f.own, "missing")
		}

		It("comes at the normal interval with a claim that is missing in Manage mode", func() {
			start("interval-manage", false)
			f.change(rendering(configMap("release")), claiming(missing()))
			f.mustPass()
			result := f.mustPass()
			Expect(result.RequeueAfter).To(BeNumerically(">=", time.Minute))
			read := findCondition(fetchRelease(f.name), string(ConditionObservedResourcesRead))
			Expect(read.Status).To(Equal(metav1.ConditionFalse), "it is still reported")
			Expect(read.Reason).To(Equal(string(ReasonObservedResourceNotFound)))
		})

		// The claims are read before the apply, so one the pass creates is reported missing
		// until the next check.
		It("comes soon after it creates an object it claims", func() {
			start("interval-created", false)
			f.change(rendering(configMap("release")), claiming(configMap("")))
			result := f.mustPass()
			Expect(f.live(configMap(""))).NotTo(BeNil())
			Expect(result.RequeueAfter).To(BeNumerically(">=", 10*time.Second))
			Expect(result.RequeueAfter).To(BeNumerically("<", 12*time.Second))
		})

		It("comes soon with a claim that is missing in Observe mode", func() {
			start("interval-observe", false)
			f.change(inMode(openchoreov1alpha1.RenderedReleaseModeObserve), claiming(missing()))
			result := f.mustPass()
			Expect(result.RequeueAfter).To(BeNumerically(">=", 10*time.Second))
			Expect(result.RequeueAfter).To(BeNumerically("<", 12*time.Second))
		})

		It("comes soon with a claim that can't be read in Manage mode", func() {
			start("interval-unreadable", false)
			f.change(rendering(configMap("release")), claiming(missing()))
			f.proxy.hook(planeHook{match: func(r *planeRequest) bool { return strings.HasSuffix(r.path, "/missing") }, every: true,
				answer: &planeAnswer{status: http.StatusBadGateway, contentType: "text/plain", body: "no agents"}})
			result := f.mustPass()
			Expect(result.RequeueAfter).To(BeNumerically(">=", 10*time.Second))
			Expect(result.RequeueAfter).To(BeNumerically("<", 12*time.Second))
		})

		It("comes soon with a claim that is Progressing next to one that is missing in Manage mode", func() {
			start("interval-progressing", false)
			deployment := teamDeploymentAt("claim-interval-progressing", 1)
			claim := unstructuredObj("apps/v1", "Deployment", testNs, deployment.Name)
			f.change(rendering(configMap("release")), claiming(missing(), claim))
			result := f.mustPass()
			Expect(result.RequeueAfter).To(BeNumerically(">=", 10*time.Second))
			Expect(result.RequeueAfter).To(BeNumerically("<", 12*time.Second))
		})
	})
})
