// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// deleteViolations returns the deletes among the requests that break a rule every delete
// follows: it names by UID an object that was listed before, that a release applied, and
// that is neither claimed nor labeled as claimed. Which release applied it is left to the
// cleanup specs. testReconcilerWithDPClient checks it for every spec, without knowing the
// claims.
func deleteViolations(requests []*planeRequest, claims ...openchoreov1alpha1.ObservedResource) []string {
	listed := sets.New[types.UID]()
	var violations []string
	for _, r := range requests {
		if isList(r) && r.status == http.StatusOK {
			list := &unstructured.UnstructuredList{}
			if err := json.Unmarshal(r.answer, list); err != nil {
				violations = append(violations, "unreadable list: "+err.Error())
			}
			for i := range list.Items {
				listed.Insert(list.Items[i].GetUID())
			}
		}
		accepted := r.status >= http.StatusOK && r.status < http.StatusMultipleChoices
		if r.method != http.MethodDelete || !accepted || r.before == nil {
			continue
		}
		violated := func(rule string) { violations = append(violations, fmt.Sprintf("DELETE %s: %s", r.path, rule)) }

		options := &metav1.DeleteOptions{}
		_ = json.Unmarshal(r.body, options)
		if options.Preconditions == nil || options.Preconditions.UID == nil {
			violated("no UID")
		} else if !listed.Has(*options.Preconditions.UID) {
			violated("was not listed")
		}
		if !hasApplyEntry(r.before) {
			violated("no release applied it")
		}
		if _, labeled := r.before.GetLabels()[labels.LabelKeyClaimed]; labeled {
			violated("is labeled as claimed")
		}
		if claim := claimOf(r.before, claims); claim != nil {
			violated("is claimed as " + claim.ID)
		}
	}
	return violations
}

// refusedBy reports whether a pass refused: it ended without an error, and the release says
// ApplyRefused for its generation. A pass that fails after its writes and before the status
// leaves the condition of an earlier refusal behind.
func refusedBy(release *openchoreov1alpha1.RenderedRelease, passErr error) bool {
	cond := findCondition(release, ConditionResourcesApplied)
	return passErr == nil && cond != nil && cond.Reason == ReasonApplyRefused && cond.ObservedGeneration == release.Generation
}

// applyViolations returns the writes of one pass of a release that break a rule of the
// checks before them: a pass that refuses writes nothing, and a pass with a rejected dry run
// writes nothing after it. An object the pass read is applied over only if the release owns
// it, or if it is claimed and a dry run was accepted. Nothing new is created after the first
// object was taken over. Only server-side applies are checked against what the pass read;
// giveBackViolations checks the other writes to an object.
func applyViolations(release *openchoreov1alpha1.RenderedRelease, refused bool, requests []*planeRequest) []string {
	read, dryRun := sets.New[string](), sets.New[string]()
	tookOver, rejected := false, false
	var violations []string
	for _, r := range requests {
		if r.method == http.MethodGet {
			if r.status == http.StatusOK && !isList(r) {
				read.Insert(r.path)
			}
			continue
		}
		isDryRun := strings.Contains(r.query, "dryRun=All")
		if r.status >= http.StatusMultipleChoices {
			rejected = rejected || isDryRun
			continue
		}
		if isDryRun {
			dryRun.Insert(r.path)
			continue
		}
		violated := func(rule string) {
			violations = append(violations, fmt.Sprintf("%s %s: %s", r.method, r.path, rule))
		}
		if refused {
			violated("written by a pass that refuses")
		}
		if rejected {
			violated("written after a dry run was rejected")
		}
		if r.contentType != string(types.ApplyPatchType) {
			continue
		}
		if r.before == nil && tookOver {
			violated("created after an object was taken over")
		}
		if r.before != nil && read.Has(r.path) && !hasMarksOf(r.before, release) {
			tookOver = true
			if claimOf(r.before, release.Spec.ObservedResources) == nil {
				violated("read, not owned, and written anyway")
			} else if !dryRun.Has(r.path) {
				violated("taken over without a dry run")
			}
		}
	}
	return violations
}

// giveBackViolations returns the requests that erase or add the marks of a claimed object
// and change anything else of it.
func giveBackViolations(requests []*planeRequest) []string {
	var violations []string
	for _, r := range requests {
		marks := r.contentType == string(types.JSONPatchType) || strings.Contains(r.query, "fieldManager=renderedrelease-takeover")
		if !marks || r.status != http.StatusOK || r.before == nil || r.after == nil {
			continue
		}
		if diff := cmp.Diff(withoutMarks(r.before), withoutMarks(r.after)); diff != "" {
			violations = append(violations, fmt.Sprintf("%s %s changes more than the marks: %s", r.method, r.path, diff))
		}
	}
	return violations
}

// withoutMarks returns the object without what a release and a claim leave on it: the
// entries of the two field managers, the tracking labels and the claimed label.
func withoutMarks(obj *unstructured.Unstructured) map[string]any {
	obj = obj.DeepCopy()
	obj.SetResourceVersion("")
	var entries []metav1.ManagedFieldsEntry
	for _, entry := range obj.GetManagedFields() {
		if entry.Operation != metav1.ManagedFieldsOperationApply ||
			entry.Manager != ControllerName && entry.Manager != "renderedrelease-takeover" {
			entries = append(entries, entry)
		}
	}
	obj.SetManagedFields(entries)
	kept := map[string]string{}
	for key, value := range obj.GetLabels() {
		if !markLabels.Has(key) {
			kept[key] = value
		}
	}
	obj.SetLabels(kept)
	if len(kept) == 0 {
		unstructured.RemoveNestedField(obj.Object, "metadata", "labels")
	}
	return obj.Object
}

var markLabels = sets.New(
	"openchoreo.dev/managed-by",
	"openchoreo.dev/rendered-release-resource-id",
	"openchoreo.dev/rendered-release-uid",
	"openchoreo.dev/rendered-release-name",
	"openchoreo.dev/rendered-release-namespace",
	"openchoreo.dev/claimed",
)

// hasMarksOf reports whether a release applied the object, by its labels and its entry.
func hasMarksOf(obj *unstructured.Unstructured, release *openchoreov1alpha1.RenderedRelease) bool {
	return obj.GetLabels()[labels.LabelKeyRenderedReleaseName] == release.Name &&
		obj.GetLabels()[labels.LabelKeyRenderedReleaseNamespace] == release.Namespace && hasApplyEntry(obj)
}

// claimOf returns the claim on an object, whatever its API version, and nil without one.
func claimOf(obj *unstructured.Unstructured, claims []openchoreov1alpha1.ObservedResource) *openchoreov1alpha1.ObservedResource {
	gvk := obj.GroupVersionKind()
	for i, claim := range claims {
		gv, err := schema.ParseGroupVersion(claim.APIVersion)
		if err == nil && gv.Group == gvk.Group && strings.EqualFold(claim.Kind, gvk.Kind) &&
			claim.Namespace == obj.GetNamespace() && claim.Name == obj.GetName() {
			return &claims[i]
		}
	}
	return nil
}

func isList(r *planeRequest) bool {
	return r.method == http.MethodGet && strings.HasPrefix(r.query, "labelSelector=")
}

func hasApplyEntry(obj *unstructured.Unstructured) bool {
	for _, entry := range obj.GetManagedFields() {
		if entry.Manager == ControllerName && entry.Operation == metav1.ManagedFieldsOperationApply {
			return true
		}
	}
	return false
}

var _ = Describe("the rules for deletes", func() {
	const name = "delete-rules"
	var (
		proxy *planeProxy
		plane client.Client
		obj   *unstructured.Unstructured
	)
	applied := client.MatchingLabels{labels.LabelKeyManagedBy: ControllerName}

	newObj := func() *unstructured.Unstructured {
		o := &unstructured.Unstructured{}
		o.SetAPIVersion("v1")
		o.SetKind("ConfigMap")
		o.SetNamespace(testNs)
		o.SetName(name)
		o.SetLabels(applied)
		return o
	}
	list := func() {
		GinkgoHelper()
		l := &unstructured.UnstructuredList{}
		l.SetAPIVersion("v1")
		l.SetKind("ConfigMapList")
		Expect(plane.List(ctx, l, applied)).To(Succeed())
	}

	BeforeEach(func() {
		proxy = newPlaneProxy()
		plane = proxy.client()
		obj = newObj()
		Expect(plane.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, newObj()) })
	})

	It("pass a delete by UID of an object that was listed and applied", func() {
		list()
		Expect(plane.Delete(ctx, obj, client.Preconditions{UID: ptr.To(obj.GetUID())})).To(Succeed())
		Expect(deleteViolations(proxy.requests())).To(BeEmpty())
	})

	It("catch a delete without the UID", func() {
		list()
		Expect(plane.Delete(ctx, obj)).To(Succeed())
		Expect(deleteViolations(proxy.requests())).To(ConsistOf(ContainSubstring("no UID")))
	})

	It("catch a delete of an object that was not listed", func() {
		Expect(plane.Delete(ctx, obj, client.Preconditions{UID: ptr.To(obj.GetUID())})).To(Succeed())
		Expect(deleteViolations(proxy.requests())).To(ConsistOf(ContainSubstring("was not listed")))
	})

	It("catch a delete of an object no release applied", func() {
		Expect(k8sClient.Delete(ctx, newObj())).To(Succeed())
		obj = newObj()
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		list()
		Expect(plane.Delete(ctx, obj, client.Preconditions{UID: ptr.To(obj.GetUID())})).To(Succeed())
		Expect(deleteViolations(proxy.requests())).To(ConsistOf(ContainSubstring("no release applied it")))
	})

	It("catch a delete of an object that is claimed, or labeled as claimed", func() {
		claimed := newObj()
		claimed.SetLabels(map[string]string{labels.LabelKeyClaimed: "yes"})
		Expect(plane.Patch(ctx, claimed, client.Apply, client.FieldOwner("kubectl"))).To(Succeed())
		list()
		Expect(plane.Delete(ctx, obj, client.Preconditions{UID: ptr.To(obj.GetUID())})).To(Succeed())
		claim := openchoreov1alpha1.ObservedResource{ID: "mine", APIVersion: "v1", Kind: "CONFIGMAP", Namespace: testNs, Name: name}
		another := openchoreov1alpha1.ObservedResource{ID: "another", APIVersion: "v1", Kind: "ConfigMap", Namespace: testNs, Name: "x"}
		Expect(deleteViolations(proxy.requests(), claim, another)).To(ConsistOf(
			ContainSubstring("is labeled as claimed"), ContainSubstring("is claimed as mine")))
	})

	It("check a delete the plane accepted to finish later, too", func() {
		claimed := newObj()
		claimed.SetLabels(map[string]string{labels.LabelKeyClaimed: "yes"})
		Expect(plane.Patch(ctx, claimed, client.Apply, client.FieldOwner("kubectl"))).To(Succeed())
		list()
		Expect(plane.Delete(ctx, obj, client.Preconditions{UID: ptr.To(obj.GetUID())})).To(Succeed())
		requests := proxy.requests()
		accepted := *requests[len(requests)-1]
		accepted.status = http.StatusAccepted
		requests[len(requests)-1] = &accepted
		Expect(deleteViolations(requests)).To(ConsistOf(ContainSubstring("is labeled as claimed")))
	})

	It("leave out a delete the plane rejected", func() {
		list()
		err := plane.Delete(ctx, obj, client.Preconditions{UID: ptr.To(types.UID("another"))})
		Expect(err).To(HaveOccurred())
		Expect(deleteViolations(proxy.requests())).To(BeEmpty())
	})
})

var _ = Describe("the rules for applies", func() {
	const name = "apply-rules"
	var (
		proxy   *planeProxy
		plane   client.Client
		release *openchoreov1alpha1.RenderedRelease
	)
	newObj := func(releaseName string) *unstructured.Unstructured {
		o := unstructuredObj("v1", "ConfigMap", testNs, name)
		o.SetLabels(map[string]string{labels.LabelKeyRenderedReleaseName: releaseName, labels.LabelKeyRenderedReleaseNamespace: testNs})
		return o
	}
	applyAs := func(releaseName string) {
		GinkgoHelper()
		Expect(plane.Patch(ctx, newObj(releaseName), client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName))).To(Succeed())
	}
	read := func() {
		GinkgoHelper()
		Expect(plane.Get(ctx, client.ObjectKey{Namespace: testNs, Name: name}, newObj(""))).To(Succeed())
	}

	BeforeEach(func() {
		proxy = newPlaneProxy()
		plane = proxy.client()
		release = makeMinimalRelease("mine", "env")
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, newObj("")) })
	})

	It("pass a new object, and one the release owns", func() {
		applyAs("mine")
		read()
		applyAs("mine")
		Expect(applyViolations(release, false, proxy.requests())).To(BeEmpty())
	})

	It("catch an apply over another release's object", func() {
		applyAs("another")
		proxy.reset()
		read()
		applyAs("mine")
		Expect(applyViolations(release, false, proxy.requests())).To(ConsistOf(ContainSubstring("read, not owned")))
	})

	It("catch an apply over an object no release applied", func() {
		Expect(k8sClient.Create(ctx, newObj("mine"))).To(Succeed())
		read()
		applyAs("mine")
		Expect(applyViolations(release, false, proxy.requests())).To(ConsistOf(ContainSubstring("read, not owned")))
	})

	It("pass an object that is claimed, after a dry run, and catch it without one", func() {
		release.Spec.ObservedResources = []openchoreov1alpha1.ObservedResource{{ID: "mine", APIVersion: "v1", Kind: "ConfigMap", Namespace: testNs, Name: name}}
		Expect(k8sClient.Create(ctx, newObj("mine"))).To(Succeed())
		read()
		dryRun := newObj("mine")
		Expect(plane.Patch(ctx, dryRun, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName), client.DryRunAll)).To(Succeed())
		applyAs("mine")
		requests := proxy.requests()
		Expect(applyViolations(release, false, requests)).To(BeEmpty())
		Expect(applyViolations(release, false, slices.Delete(requests, 1, 2))).To(ConsistOf(ContainSubstring("without a dry run")))
	})

	It("catch an object that is created after one was taken over", func() {
		release.Spec.ObservedResources = []openchoreov1alpha1.ObservedResource{{ID: "mine", APIVersion: "v1", Kind: "ConfigMap", Namespace: testNs, Name: name}}
		Expect(k8sClient.Create(ctx, newObj("mine"))).To(Succeed())
		read()
		Expect(plane.Patch(ctx, newObj("mine"), client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName), client.DryRunAll)).To(Succeed())
		applyAs("mine")
		created := newObj("mine")
		created.SetName("apply-rules-created")
		Expect(plane.Patch(ctx, created, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, created) })
		Expect(applyViolations(release, false, proxy.requests())).To(ConsistOf(ContainSubstring("created after an object was taken over")))
	})

	It("leave out an object the pass did not read, which an own namespace adopts", func() {
		applyAs("another")
		proxy.reset()
		applyAs("mine")
		Expect(applyViolations(release, false, proxy.requests())).To(BeEmpty())
	})

	It("catch any write after a dry run was rejected", func() {
		release.Spec.ObservedResources = []openchoreov1alpha1.ObservedResource{{ID: "mine", APIVersion: "v1", Kind: "ConfigMap", Namespace: testNs, Name: name}}
		Expect(k8sClient.Create(ctx, newObj("mine"))).To(Succeed())
		read()
		rejected := newObj("mine")
		rejected.SetUID("another")
		Expect(plane.Patch(ctx, rejected, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName), client.DryRunAll)).NotTo(Succeed())
		created := newObj("mine")
		created.SetName("apply-rules-created")
		Expect(plane.Patch(ctx, created, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, created) })
		Expect(applyViolations(release, false, proxy.requests())).To(ConsistOf(ContainSubstring("written after a dry run was rejected")))
	})

	It("catch any write of a pass that refuses, except a dry run", func() {
		release.Status.Conditions = []metav1.Condition{{Type: ConditionResourcesApplied, Status: metav1.ConditionFalse, Reason: ReasonApplyRefused}}
		Expect(refusedBy(release, nil)).To(BeTrue())
		Expect(refusedBy(release, errors.New("a list failed"))).To(BeFalse(), "a refusal is not an error")
		Expect(plane.Patch(ctx, newObj("mine"), client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName), client.DryRunAll)).To(Succeed())
		Expect(applyViolations(release, true, proxy.requests())).To(BeEmpty())
		applyAs("mine")
		Expect(applyViolations(release, true, proxy.requests())).To(ConsistOf(ContainSubstring("written by a pass that refuses")))
	})
})

var _ = Describe("the rules for marks", func() {
	var (
		proxy *planeProxy
		plane client.Client
		obj   *unstructured.Unstructured
	)
	BeforeEach(func() {
		proxy = newPlaneProxy()
		plane = proxy.client()
		obj = unstructuredObj("v1", "ConfigMap", testNs, "mark-rules")
		obj.SetLabels(map[string]string{labels.LabelKeyManagedBy: ControllerName, "app": "api"})
		obj.Object["data"] = map[string]any{"k": "v"}
		Expect(plane.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName))).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, unstructuredObj("v1", "ConfigMap", testNs, "mark-rules")) })
	})
	patch := func(ops string) {
		GinkgoHelper()
		Expect(plane.Patch(ctx, obj, client.RawPatch(types.JSONPatchType, []byte(ops)))).To(Succeed())
	}

	It("pass a patch that only removes a mark", func() {
		patch(`[{"op": "remove", "path": "/metadata/labels/openchoreo.dev~1managed-by"}]`)
		Expect(giveBackViolations(proxy.requests())).To(BeEmpty())
	})

	It("catch a patch that also removes another label, or changes the data", func() {
		patch(`[{"op": "remove", "path": "/metadata/labels/app"}]`)
		patch(`[{"op": "replace", "path": "/data/k", "value": "x"}]`)
		Expect(giveBackViolations(proxy.requests())).To(HaveLen(2))
	})

	It("catch a claim that writes more than its label", func() {
		claim := unstructuredObj("v1", "ConfigMap", testNs, "mark-rules")
		claim.SetLabels(map[string]string{labels.LabelKeyClaimed: "true"})
		Expect(plane.Patch(ctx, claim, client.Apply, client.FieldOwner("renderedrelease-takeover"))).To(Succeed())
		Expect(giveBackViolations(proxy.requests())).To(BeEmpty())

		claim = unstructuredObj("v1", "ConfigMap", testNs, "mark-rules")
		claim.SetLabels(map[string]string{labels.LabelKeyClaimed: "true", "extra": "label"})
		Expect(plane.Patch(ctx, claim, client.Apply, client.FieldOwner("renderedrelease-takeover"))).To(Succeed())
		Expect(giveBackViolations(proxy.requests())).To(HaveLen(1))
	})
})
