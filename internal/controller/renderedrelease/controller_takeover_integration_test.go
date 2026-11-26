// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("RenderedRelease takeover", func() {
	const (
		manage  = openchoreov1alpha1.RenderedReleaseModeManage
		observe = openchoreov1alpha1.RenderedReleaseModeObserve
	)
	var (
		f *releaseFixture
		// namespace existed before, so the guard is on.
		namespace string
	)
	start := func(name string) {
		GinkgoHelper()
		f = newReleaseFixture("takeover-"+name, "")
		namespace = "existing-" + name
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())
	}

	object := func(apiVersion, kind, name, fields string) *unstructured.Unstructured {
		GinkgoHelper()
		obj := &unstructured.Unstructured{Object: map[string]any{}}
		Expect(json.Unmarshal([]byte(fields), &obj.Object)).To(Succeed())
		obj.SetAPIVersion(apiVersion)
		obj.SetKind(kind)
		obj.SetNamespace(namespace)
		obj.SetName(name)
		return obj
	}
	configMap := func(name, value string) *unstructured.Unstructured {
		return object("v1", "ConfigMap", name, `{"data": {"by": "`+value+`"}}`)
	}
	workload := func(kind, app string) *unstructured.Unstructured {
		return object("apps/v1", kind, "api", fmt.Sprintf(`{"spec": {"replicas": 1, "serviceName": "api", "selector": {"matchLabels": {"app": %[1]q}},
			"template": {"metadata": {"labels": {"app": %[1]q}}, "spec": {"containers": [{"name": "api", "image": "api:1"}]}}}}`, app))
	}
	deployment := func(app string) *unstructured.Unstructured {
		obj := workload("Deployment", app)
		unstructured.RemoveNestedField(obj.Object, "spec", "serviceName")
		return obj
	}
	service := func(app string) *unstructured.Unstructured {
		return object("v1", "Service", "svc", `{"spec": {"selector": {"app": "`+app+`"}, "ports": [{"name": "http", "port": 80}]}}`)
	}
	outside := func(obj *unstructured.Unstructured) *unstructured.Unstructured {
		GinkgoHelper()
		created := obj.DeepCopy()
		Expect(k8sClient.Create(ctx, created, client.FieldOwner("helm"))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj.DeepCopy()) })
		return created
	}
	expectUnchanged := func(objs ...*unstructured.Unstructured) {
		GinkgoHelper()
		for _, obj := range objs {
			Expect(f.live(obj).GetResourceVersion()).To(Equal(obj.GetResourceVersion()), describeObject(obj))
		}
	}
	condition := func() *metav1.Condition {
		return findCondition(fetchRelease(f.name), ConditionResourcesApplied)
	}
	expectTakenOver := func(obj *unstructured.Unstructured) {
		GinkgoHelper()
		live := f.live(obj)
		Expect(live.GetUID()).To(Equal(obj.GetUID()))
		Expect(hasMarksOf(live, fetchRelease(f.name))).To(BeTrue(), "%s is the release's", describeObject(obj))
		Expect(live.GetLabels()).To(HaveKey(labels.LabelKeyClaimed))
	}
	expectGivenBack := func(obj *unstructured.Unstructured) {
		GinkgoHelper()
		live := f.live(obj)
		Expect(live).NotTo(BeNil())
		Expect(live.GetUID()).To(Equal(obj.GetUID()))
		Expect(hasApplyEntry(live)).To(BeFalse())
		for key := range live.GetLabels() {
			Expect(markLabels.Has(key)).To(BeFalse(), "label %s is left", key)
		}
	}
	isDryRun := func(r *planeRequest) bool { return strings.Contains(r.query, "dryRun=All") }
	isLabelRequest := func(r *planeRequest) bool { return strings.Contains(r.query, "fieldManager=renderedrelease-takeover") }
	stored := func() []*planeRequest {
		return f.sentThat(func(r *planeRequest) bool { return r.method != http.MethodGet && !isDryRun(r) })
	}
	path := func(requests []*planeRequest) []string {
		paths := make([]string, 0, len(requests))
		for _, r := range requests {
			paths = append(paths, r.path[strings.LastIndex(r.path[:strings.LastIndex(r.path, "/")], "/")+1:])
		}
		return paths
	}

	It("writes new objects first, then each taken over object after its label", func() {
		start("order")
		svc, dep := outside(service("api")), outside(deployment("api"))
		f.change(rendering(service("api"), deployment("api"), configMap("new", "release")), claiming(service(""), deployment("")))
		f.mustPass()

		Expect(path(f.sentThat(isDryRun))).To(Equal([]string{"services/svc", "deployments/api"}))
		all := f.writes()
		Expect(isDryRun(all[0]) && isDryRun(all[1])).To(BeTrue(), "both dry runs come before the first write")
		writes := stored()
		Expect(path(writes)).To(Equal([]string{"configmaps/new", "services/svc", "services/svc", "deployments/api", "deployments/api"}))
		Expect(isLabelRequest(writes[1])).To(BeTrue())
		Expect(isLabelRequest(writes[3])).To(BeTrue())
		expectTakenOver(svc)
		expectTakenOver(dep)
		Expect(fetchRelease(f.name).Status.Resources).To(HaveExactElements(
			HaveField("Kind", "Service"), HaveField("Kind", "Deployment"), HaveField("Kind", "ConfigMap")), "in the rendered order")
	})

	It("writes nothing when a dry run is rejected, and names every rejected object", func() {
		start("rejected")
		svc, dep := outside(service("api")), outside(deployment("api"))
		fixed := outside(object("v1", "ConfigMap", "fixed", `{"immutable": true, "data": {"by": "helm"}}`))
		f.change(
			rendering(service("other"), deployment("other"), object("v1", "ConfigMap", "fixed", `{"data": {"by": "release"}}`), configMap("new", "release")),
			claiming(service(""), deployment(""), configMap("fixed", "")))

		_, err := f.pass()
		Expect(err).To(HaveOccurred())
		Expect(stored()).To(BeEmpty())
		expectUnchanged(svc, dep, fixed)
		Expect(f.live(configMap("new", ""))).To(BeNil())
		Expect(condition().Reason).To(Equal(ReasonApplyFailed))
		Expect(condition().Message).To(ContainSubstring("Deployment.apps " + namespace + "/api can't be taken over"))
		Expect(condition().Message).To(ContainSubstring("field is immutable"))
		Expect(condition().Message).To(ContainSubstring("ConfigMap " + namespace + "/fixed can't be taken over"))
		Expect(condition().Message).NotTo(ContainSubstring("Service"))
		Expect(fetchRelease(f.name).Status.ObservedResources).To(HaveLen(3), "the claims are still read")

		By("With the template fixed, all of it is taken over")
		f.change(rendering(service("api"), deployment("api"), object("v1", "ConfigMap", "fixed", `{"immutable": true, "data": {"by": "helm"}}`)))
		f.mustPass()
		expectTakenOver(svc)
		expectTakenOver(dep)
		expectTakenOver(fixed)
	})

	It("leaves a Service alone when a new StatefulSet is rejected", func() {
		start("service")
		svc := outside(service("theirs"))
		rejected := workload("StatefulSet", "api")
		Expect(unstructured.SetNestedField(rejected.Object, "other", "spec", "template", "metadata", "labels", "app")).To(Succeed())
		f.change(rendering(service("api"), rejected), claiming(service("")))

		_, err := f.pass()
		Expect(err).To(MatchError(ContainSubstring("selector")))
		expectUnchanged(svc)
	})

	It("writes nothing when one object is refused, and runs no dry run", func() {
		start("refused")
		svc := outside(service("theirs"))
		other := outside(configMap("other", "helm"))
		f.change(rendering(service("api"), configMap("other", "release")), claiming(service("")))

		f.mustPass()
		Expect(f.writes()).To(BeEmpty())
		expectUnchanged(svc, other)
		Expect(condition().Reason).To(Equal(ReasonApplyRefused))
		Expect(condition().Message).To(ContainSubstring("/other exists and is not managed by this release"))
	})

	Describe("what happens between the check and the label", func() {
		take := func(name string) *unstructured.Unstructured {
			GinkgoHelper()
			start(name)
			theirs := outside(configMap("a", "helm"))
			f.change(rendering(configMap("a", "release")), claiming(configMap("a", "")))
			return theirs
		}
		beforeLabel := func(change func()) { f.proxy.hook(planeHook{match: isLabelRequest, before: change}) }

		It("leaves an object that is recreated alone, and takes it over the next time", func() {
			take("label-recreated")
			var recreated *unstructured.Unstructured
			beforeLabel(func() {
				Expect(k8sClient.Delete(ctx, configMap("a", ""))).To(Succeed())
				recreated = outside(configMap("a", "helm again"))
			})
			_, err := f.pass()
			Expect(err).To(HaveOccurred())
			expectUnchanged(recreated)

			f.mustPass()
			expectTakenOver(recreated)
		})

		It("creates nothing in place of an object that is deleted", func() {
			take("label-deleted")
			beforeLabel(func() { Expect(k8sClient.Delete(ctx, configMap("a", ""))).To(Succeed()) })
			_, err := f.pass()
			Expect(err).To(HaveOccurred())
			Expect(f.live(configMap("a", ""))).To(BeNil())
		})

		It("leaves an object alone that is recreated after its label", func() {
			take("apply-recreated")
			var recreated *unstructured.Unstructured
			f.proxy.hook(planeHook{match: func(r *planeRequest) bool { return r.method == http.MethodPatch && !isDryRun(r) && !isLabelRequest(r) },
				before: func() {
					Expect(k8sClient.Delete(ctx, configMap("a", ""))).To(Succeed())
					recreated = outside(configMap("a", "helm again"))
				}})
			_, err := f.pass()
			Expect(err).To(HaveOccurred())
			expectUnchanged(recreated)
		})

		It("takes over an object that changes all the time", func() {
			theirs := take("busy")
			beforeLabel(func() {
				busy := f.live(theirs)
				busy.SetAnnotations(map[string]string{"example.com/refreshed": "now"})
				Expect(k8sClient.Update(ctx, busy, client.FieldOwner("helm"))).To(Succeed())
			})
			f.mustPass()
			expectTakenOver(theirs)
			Expect(f.live(theirs).GetAnnotations()).To(HaveKeyWithValue("example.com/refreshed", "now"), "it changed")
		})

		It("leaves a label that does nothing when the claim goes before the apply", func() {
			theirs := take("label-only")
			f.proxy.hook(planeHook{match: func(r *planeRequest) bool { return r.method == http.MethodPatch && !isDryRun(r) && !isLabelRequest(r) },
				answer: &planeAnswer{status: http.StatusBadGateway, contentType: "text/plain", body: "no agents"}})
			_, err := f.pass()
			Expect(err).To(HaveOccurred())
			f.change(rendering(), claiming())
			f.mustPass()
			live := f.live(theirs)
			Expect(hasApplyEntry(live)).To(BeFalse())
			Expect(live.GetLabels()).To(HaveKey(labels.LabelKeyClaimed))
			Expect(live.Object).To(HaveKeyWithValue("data", theirs.Object["data"]))
		})
	})

	Describe("moving between the modes", func() {
		It("takes over an object it observed, and gives it back", func() {
			start("there-and-back")
			theirs := outside(deployment("api"))
			f.change(inMode(observe), rendering(deployment("api")), claiming(deployment("")))
			f.mustPass()
			expectUnchanged(theirs)

			f.change(inMode(manage))
			f.mustPass()
			expectTakenOver(theirs)
			generation := f.live(theirs).GetGeneration()

			f.change(inMode(observe))
			f.mustPass()
			expectGivenBack(theirs)
			Expect(f.live(theirs).GetGeneration()).To(Equal(generation), "nothing rolls out")

			f.change(inMode(manage))
			f.mustPass()
			expectTakenOver(theirs)
		})

		It("creates an object under another name next to the one it observes", func() {
			start("side-by-side")
			theirs := outside(configMap("theirs", "helm"))
			f.change(inMode(observe), rendering(configMap("new", "release")), claiming(configMap("theirs", "")))
			f.mustPass()
			f.change(inMode(manage))
			f.mustPass()
			expectUnchanged(theirs)
			Expect(f.live(configMap("new", ""))).NotTo(BeNil())
			Expect(fetchRelease(f.name).Status.ObservedResources).To(HaveLen(1))

			f.change(inMode(observe))
			f.mustPass()
			expectUnchanged(theirs)
			Expect(f.live(configMap("new", ""))).To(BeNil())
		})

		It("gives back what it took over when it is deleted, also after the claim is gone", func() {
			start("deleted")
			theirs := outside(configMap("a", "helm"))
			f.change(rendering(configMap("a", "release")), claiming(configMap("a", "")))
			f.mustPass()
			expectTakenOver(theirs)
			f.change(claiming())
			f.mustPass()
			f.deleteRelease()
			f.mustPass()
			expectGivenBack(theirs)
		})
	})

	Describe("an object that kubectl replace writes", func() {
		replace := func(theirs *unstructured.Unstructured) {
			GinkgoHelper()
			replaced := f.live(theirs)
			replaced.SetLabels(nil)
			replaced.Object["data"] = map[string]any{"by": "kubectl replace"}
			Expect(k8sClient.Update(ctx, replaced, client.FieldOwner("kubectl-replace"))).To(Succeed())
		}

		It("is taken over again while it is claimed", func() {
			start("replace-claimed")
			theirs := outside(configMap("a", "helm"))
			f.change(rendering(configMap("a", "release")), claiming(configMap("a", "")))
			f.mustPass()
			replace(theirs)
			f.mustPass()
			expectTakenOver(theirs)
			Expect(f.sentThat(isDryRun)).To(HaveLen(1))
			Expect(f.live(theirs).Object).To(HaveKeyWithValue("data", HaveKeyWithValue("by", "release")))
		})

		It("is refused once it isn't", func() {
			start("replace-unclaimed")
			theirs := outside(configMap("a", "helm"))
			f.change(rendering(configMap("a", "release")), claiming(configMap("a", "")))
			f.mustPass()
			f.change(claiming())
			replace(theirs)
			f.mustPass()
			Expect(f.writes()).To(BeEmpty())
			Expect(condition().Reason).To(Equal(ReasonApplyRefused))
		})
	})

	It("is handed from one release to another, and the first only reads it from then on", func() {
		start("handover")
		theirs := outside(configMap("a", "helm"))
		render, claim := rendering(configMap("a", "release")), claiming(configMap("a", ""))
		first := f
		second := newReleaseFixture("takeover-handover-second", "")
		pass := func(release *releaseFixture, reason string) {
			GinkgoHelper()
			f = release
			f.mustPass()
			Expect(condition().Reason).To(Equal(reason))
		}

		first.change(render, claim)
		pass(first, ReasonApplySucceeded)
		expectTakenOver(theirs)

		By("The second one claims and renders it too")
		second.change(render, claim)
		pass(second, ReasonApplyRefused)
		Expect(condition().Message).To(ContainSubstring("belongs to RenderedRelease " + first.name))
		Expect(f.writes()).To(BeEmpty())

		By("The first one gives it back, and the second takes it over")
		first.change(inMode(observe))
		pass(first, ReasonObserveMode)
		expectGivenBack(theirs)
		pass(second, ReasonApplySucceeded)
		expectTakenOver(theirs)
		taken := f.live(theirs)

		By("The first one still claims it, and only reads it, also when it is deleted")
		pass(first, ReasonObserveMode)
		Expect(f.writes()).To(BeEmpty())
		first.deleteRelease()
		Expect(first.writes()).To(BeEmpty())
		first.mustPass()
		Expect(first.writes()).To(BeEmpty())
		expectUnchanged(taken)
	})

})
