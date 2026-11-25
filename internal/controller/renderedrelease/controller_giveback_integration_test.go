// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"encoding/json"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("giving an object back", func() {
	const namespace = "give-back"
	var (
		proxy *planeProxy
		plane client.Client
	)
	BeforeEach(func() {
		proxy = newPlaneProxy()
		plane = proxy.client()
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())
	})

	// object returns an object with the given fields next to its metadata.
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
	configMap := func(value string) *unstructured.Unstructured {
		return object("v1", "ConfigMap", "cm", `{"data": {"k": "`+value+`"}}`)
	}
	// outside creates an object no release applied, with a label of its own.
	outside := func(obj *unstructured.Unstructured) {
		GinkgoHelper()
		created := obj.DeepCopy()
		created.SetLabels(map[string]string{"owner": "outside"})
		Expect(k8sClient.Create(ctx, created, client.FieldOwner("helm"))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj.DeepCopy()) })
	}
	// apply applies an object the way a release does, with a label of its template, and
	// returns it as it is on the plane.
	apply := func(obj *unstructured.Unstructured) *unstructured.Unstructured {
		GinkgoHelper()
		applied := obj.DeepCopy()
		applied.SetLabels(map[string]string{
			labels.LabelKeyManagedBy:                 ControllerName,
			labels.LabelKeyRenderedReleaseResourceID: "id",
			labels.LabelKeyRenderedReleaseUID:        "uid",
			labels.LabelKeyRenderedReleaseName:       "give-back",
			labels.LabelKeyRenderedReleaseNamespace:  testNs,
			"app":                                    "template",
		})
		Expect(k8sClient.Patch(ctx, applied, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj.DeepCopy()) })
		return applied
	}
	// claim adds the claimed label the way a takeover does.
	claim := func(obj *unstructured.Unstructured) *unstructured.Unstructured {
		GinkgoHelper()
		claimed := unstructuredObj(obj.GetAPIVersion(), obj.GetKind(), obj.GetNamespace(), obj.GetName())
		claimed.SetLabels(map[string]string{labels.LabelKeyClaimed: "true"})
		Expect(k8sClient.Patch(ctx, claimed, client.Apply, client.FieldOwner("renderedrelease-takeover"))).To(Succeed())
		return claimed
	}
	live := func(obj *unstructured.Unstructured) *unstructured.Unstructured {
		GinkgoHelper()
		read := unstructuredObj(obj.GetAPIVersion(), obj.GetKind(), obj.GetNamespace(), obj.GetName())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), read)).To(Succeed())
		return read
	}
	managers := func(obj *unstructured.Unstructured) []string {
		names := make([]string, 0, len(obj.GetManagedFields()))
		for _, entry := range obj.GetManagedFields() {
			names = append(names, entry.Manager)
		}
		return names
	}
	// expectGivenBack checks that no mark is left, and that nothing else changed.
	expectGivenBack := func(before *unstructured.Unstructured) *unstructured.Unstructured {
		GinkgoHelper()
		after := live(before)
		for key := range after.GetLabels() {
			Expect(markLabels.Has(key)).To(BeFalse(), "label %s is left", key)
		}
		Expect(managers(after)).NotTo(ContainElement(ControllerName))
		Expect(managers(after)).NotTo(ContainElement("renderedrelease-takeover"))
		Expect(withoutMarks(after)).To(Equal(withoutMarks(before)))
		Expect(giveBackViolations(proxy.requests())).To(BeEmpty())
		return after
	}
	isGiveBack := func(r *planeRequest) bool { return r.contentType == string(types.JSONPatchType) }

	podTemplate := `"template": {"metadata": {"labels": {"app": "api"}}, "spec": {"containers": [{"name": "api", "image": "api:1"}]}}`
	DescribeTable("erases the marks and nothing else, and its owner can apply it again",
		func(apiVersion, kind, before, rendered string) {
			theirs := object(apiVersion, kind, "given", before)
			outside(theirs)
			apply(object(apiVersion, kind, "given", rendered))
			claimed := claim(theirs)

			Expect(giveBack(ctx, plane, claimed)).To(Succeed())

			after := expectGivenBack(claimed)
			Expect(after.GetLabels()).To(HaveKeyWithValue("owner", "outside"))
			Expect(after.GetLabels()).To(HaveKeyWithValue("app", "template"), "a label of the template is not a mark")
			Expect(after.GetGeneration()).To(Equal(claimed.GetGeneration()))
			Expect(managers(after)).To(ContainElement("helm"))
			Expect(proxy.requests()).To(HaveLen(1), "it was labeled already")

			Expect(k8sClient.Patch(ctx, theirs, client.Apply, client.FieldOwner("their-tool"))).To(Succeed(),
				"applied without force, so nothing conflicts")
		},
		Entry("Deployment", "apps/v1", "Deployment",
			`{"spec": {"replicas": 1, "selector": {"matchLabels": {"app": "api"}}, `+podTemplate+`}}`,
			`{"spec": {"replicas": 3, "selector": {"matchLabels": {"app": "api"}}, `+podTemplate+`}}`),
		Entry("StatefulSet", "apps/v1", "StatefulSet",
			`{"spec": {"replicas": 1, "serviceName": "api", "selector": {"matchLabels": {"app": "api"}}, `+podTemplate+`}}`,
			`{"spec": {"replicas": 3, "serviceName": "api", "selector": {"matchLabels": {"app": "api"}}, `+podTemplate+`}}`),
		Entry("Job", "batch/v1", "Job",
			`{"spec": {"parallelism": 1, "template": {"spec": {"restartPolicy": "Never", "containers": [{"name": "api", "image": "api:1"}]}}}}`,
			`{"spec": {"parallelism": 2}}`),
		Entry("immutable ConfigMap", "v1", "ConfigMap",
			`{"immutable": true, "data": {"k": "v"}}`,
			`{"immutable": true, "data": {"k": "v"}}`),
		Entry("PersistentVolumeClaim", "v1", "PersistentVolumeClaim",
			`{"spec": {"accessModes": ["ReadWriteOnce"], "resources": {"requests": {"storage": "1Gi"}}}}`,
			`{"spec": {"accessModes": ["ReadWriteOnce"], "resources": {"requests": {"storage": "1Gi"}}}}`),
		Entry("Service", "v1", "Service",
			`{"spec": {"selector": {"app": "api"}, "ports": [{"name": "http", "port": 80}]}}`,
			`{"spec": {"selector": {"app": "other"}, "ports": [{"name": "http", "port": 80}]}}`),
		Entry("custom resource", "test.openchoreo.dev/v1", "Widget",
			`{"spec": {"size": "small", "color": "red"}}`,
			`{"spec": {"size": "large"}}`),
	)

	It("labels an object that isn't labeled first, with nothing but the label", func() {
		outside(configMap("theirs"))
		applied := apply(configMap("release"))

		Expect(giveBack(ctx, plane, applied)).To(Succeed())

		expectGivenBack(applied)
		requests := proxy.requests()
		Expect(requests).To(HaveLen(2))
		Expect(requests[0].query).To(Equal("fieldManager=renderedrelease-takeover"), "without force")
		Expect(requests[0].body).To(MatchJSON(`{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {
			"name": "cm", "namespace": "give-back", "uid": "` + string(applied.GetUID()) + `", "labels": {"openchoreo.dev/claimed": "true"}}}`))
		Expect(isGiveBack(requests[1])).To(BeTrue())
	})

	It("gives back an object only the release ever wrote", func() {
		applied := apply(configMap("release"))

		Expect(giveBack(ctx, plane, applied)).To(Succeed())
		Expect(managers(expectGivenBack(applied))).To(BeEmpty())

		By("Its owner can apply the same values, and needs force for others")
		Expect(k8sClient.Patch(ctx, configMap("release"), client.Apply, client.FieldOwner("their-tool"))).To(Succeed())
	})

	It("leaves an object only the release ever wrote to the first tool that forces another value", func() {
		Expect(giveBack(ctx, plane, apply(configMap("release")))).To(Succeed())

		err := k8sClient.Patch(ctx, configMap("theirs"), client.Apply, client.FieldOwner("their-tool"))
		Expect(apierrors.IsConflict(err)).To(BeTrue(), "got %v", err)
		Expect(err.Error()).To(ContainSubstring("before-first-apply"))
		Expect(k8sClient.Patch(ctx, configMap("theirs"), client.Apply, client.FieldOwner("their-tool"), client.ForceOwnership)).To(Succeed())
	})

	It("counts the label whatever its value is, and removes it", func() {
		theirs := configMap("theirs")
		outside(theirs)
		applied := apply(configMap("release"))
		labeled := live(applied)
		labeled.SetLabels(withLabel(labeled.GetLabels(), labels.LabelKeyClaimed, "yes"))
		Expect(k8sClient.Update(ctx, labeled, client.FieldOwner("kubectl-label"))).To(Succeed())

		Expect(giveBack(ctx, plane, labeled)).To(Succeed())
		Expect(proxy.requests()).To(HaveLen(1), "it is not labeled again")
		after := live(labeled)
		Expect(after.GetLabels()).To(Equal(map[string]string{"owner": "outside", "app": "template"}))
		Expect(managers(after)).To(Equal([]string{"helm"}), "an entry for nothing but the label goes with it")
		Expect(after.Object).To(HaveKeyWithValue("data", labeled.Object["data"]))
	})

	It("gives back an object that lost one of its labels", func() {
		outside(configMap("theirs"))
		apply(configMap("release"))
		claimed := claim(configMap(""))
		claimed.SetLabels(withoutLabel(claimed.GetLabels(), labels.LabelKeyRenderedReleaseUID))
		Expect(k8sClient.Update(ctx, claimed, client.FieldOwner("kubectl-edit"))).To(Succeed())

		Expect(giveBack(ctx, plane, claimed)).To(Succeed())
		expectGivenBack(claimed)
	})

	It("gives back an object that is being deleted", func() {
		held := configMap("theirs")
		held.SetFinalizers([]string{"example.com/hold"})
		outside(held)
		applied := apply(configMap("release"))
		Expect(k8sClient.Delete(ctx, configMap(""))).To(Succeed())
		DeferCleanup(func() {
			left := live(applied)
			left.SetFinalizers(nil)
			Expect(k8sClient.Update(ctx, left)).To(Succeed())
		})

		deleting := live(applied)
		Expect(giveBack(ctx, plane, deleting)).To(Succeed())
		expectGivenBack(deleting)
	})

	Describe("when something happens between the read and the patch", func() {
		var claimed *unstructured.Unstructured
		BeforeEach(func() {
			outside(configMap("theirs"))
			apply(configMap("release"))
			claimed = claim(configMap(""))
		})
		before := func(change func()) { proxy.hook(planeHook{match: isGiveBack, before: change}) }
		// retry gives back what is there now, as the next pass does.
		retry := func() {
			GinkgoHelper()
			now := live(claimed)
			Expect(giveBack(ctx, plane, now)).To(Succeed())
			expectGivenBack(now)
		}

		It("gives back an object that changes all the time", func() {
			before(func() {
				busy := live(claimed)
				busy.SetAnnotations(map[string]string{"example.com/refreshed": "now"})
				Expect(k8sClient.Update(ctx, busy, client.FieldOwner("helm"))).To(Succeed())
			})
			Expect(giveBack(ctx, plane, claimed)).To(Succeed())
			Expect(managers(live(claimed))).To(Equal([]string{"helm"}))
			Expect(live(claimed).GetAnnotations()).To(HaveKey("example.com/refreshed"))
		})

		It("keeps a manager that arrives", func() {
			before(func() {
				arrived := live(claimed)
				arrived.SetAnnotations(map[string]string{"example.com/new": "manager"})
				Expect(k8sClient.Update(ctx, arrived, client.FieldOwner("new-manager"))).To(Succeed())
			})
			Expect(giveBack(ctx, plane, claimed)).To(Succeed())
			Expect(managers(live(claimed))).To(ConsistOf("helm", "new-manager"))
		})

		It("keeps a manager that applies, and completes the next time if its entry moved", func() {
			before(func() {
				annotated := unstructuredObj("v1", "ConfigMap", namespace, "cm")
				annotated.SetAnnotations(map[string]string{"example.com/new": "manager"})
				Expect(k8sClient.Patch(ctx, annotated, client.Apply, client.FieldOwner("a-new-applier"))).To(Succeed())
			})
			if err := giveBack(ctx, plane, claimed); err != nil {
				Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)
				retry()
			}
			Expect(managers(live(claimed))).To(ConsistOf("helm", "a-new-applier"))
		})

		It("leaves an object that is recreated alone", func() {
			var recreated *unstructured.Unstructured
			before(func() {
				Expect(k8sClient.Delete(ctx, configMap(""))).To(Succeed())
				recreated = configMap("new")
				recreated.SetLabels(map[string]string{"openchoreo.dev/claimed": "theirs"})
				Expect(k8sClient.Create(ctx, recreated, client.FieldOwner("helm"))).To(Succeed())
			})
			Expect(giveBack(ctx, plane, claimed)).NotTo(Succeed())
			Expect(proxy.requests()[0].status).To(Equal(http.StatusUnprocessableEntity))
			Expect(live(claimed).GetResourceVersion()).To(Equal(recreated.GetResourceVersion()))
		})

		It("leaves an object that is recreated with its marks alone, as a backup tool restores one", func() {
			var restored *unstructured.Unstructured
			before(func() {
				restored = live(claimed)
				marks := restored.GetManagedFields()
				Expect(k8sClient.Delete(ctx, configMap(""))).To(Succeed())
				restored.SetUID("")
				restored.SetResourceVersion("")
				Expect(k8sClient.Create(ctx, restored, client.FieldOwner("backup"))).To(Succeed())
				restored.SetManagedFields(marks)
				Expect(k8sClient.Update(ctx, restored, client.FieldOwner("backup"))).To(Succeed())
			})
			Expect(apierrors.IsInvalid(giveBack(ctx, plane, claimed))).To(BeTrue())
			now := live(claimed)
			Expect(now.GetResourceVersion()).To(Equal(restored.GetResourceVersion()))
			Expect(managers(now)).To(ContainElement(ControllerName), "its marks were restored")
		})

		It("has nothing to do for an object that is deleted", func() {
			before(func() { Expect(k8sClient.Delete(ctx, configMap(""))).To(Succeed()) })
			Expect(giveBack(ctx, plane, claimed)).To(Succeed())
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(claimed), configMap(""))
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "got %v", err)
		})

		It("leaves an object another release applied alone", func() {
			before(func() {
				another := configMap("release")
				another.SetLabels(withLabel(claimed.GetLabels(), labels.LabelKeyRenderedReleaseName, "another"))
				another.SetLabels(withoutLabel(another.GetLabels(), labels.LabelKeyClaimed))
				Expect(k8sClient.Patch(ctx, another, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName))).To(Succeed())
			})
			Expect(giveBack(ctx, plane, claimed)).NotTo(Succeed())
			Expect(live(claimed).GetLabels()).To(HaveKeyWithValue(labels.LabelKeyRenderedReleaseName, "another"))
			Expect(managers(live(claimed))).To(ContainElement(ControllerName))
		})

		It("completes the next time when a label is removed", func() {
			before(func() {
				edited := live(claimed)
				edited.SetLabels(withoutLabel(edited.GetLabels(), labels.LabelKeyClaimed))
				Expect(k8sClient.Update(ctx, edited, client.FieldOwner("kubectl-edit"))).To(Succeed())
			})
			Expect(apierrors.IsInvalid(giveBack(ctx, plane, claimed))).To(BeTrue())
			retry()
		})
	})

	It("doesn't recreate an object that is deleted before it is labeled", func() {
		applied := apply(configMap("release"))
		Expect(k8sClient.Delete(ctx, configMap(""))).To(Succeed())
		Expect(giveBack(ctx, plane, applied)).NotTo(Succeed())
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(applied), configMap(""))
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "got %v", err)
	})
})
