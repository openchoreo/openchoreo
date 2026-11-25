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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// deleteViolations returns the deletes among the requests that break a rule every delete
// follows: it names by UID an object that was listed before, and that a release applied.
// testReconcilerWithDPClient checks it for every spec.
func deleteViolations(requests []*planeRequest) []string {
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
		if r.method != http.MethodDelete || r.status != http.StatusOK || r.before == nil {
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
	}
	return violations
}

// applyViolations returns the writes of one pass of a release that break a rule of the
// checks before them: a pass that refuses writes nothing, and no object the pass read is
// applied over unless the release owns it.
func applyViolations(release *openchoreov1alpha1.RenderedRelease, requests []*planeRequest) []string {
	refused := false
	if cond := findCondition(release, ConditionResourcesApplied); cond != nil {
		refused = cond.Reason == ReasonApplyRefused && cond.ObservedGeneration == release.Generation
	}
	read := sets.New[string]()
	var violations []string
	for _, r := range requests {
		if r.method == http.MethodGet {
			if r.status == http.StatusOK && !isList(r) {
				read.Insert(r.path)
			}
			continue
		}
		if strings.Contains(r.query, "dryRun=All") || r.status >= http.StatusMultipleChoices {
			continue
		}
		violated := func(rule string) {
			violations = append(violations, fmt.Sprintf("%s %s: %s", r.method, r.path, rule))
		}
		if refused {
			violated("written by a pass that refuses")
		}
		if r.method == http.MethodPatch && r.before != nil && read.Has(r.path) && !hasMarksOf(r.before, release) {
			violated("read, not owned, and written anyway")
		}
	}
	return violations
}

// hasMarksOf reports whether a release applied the object, by its labels and its entry.
func hasMarksOf(obj *unstructured.Unstructured, release *openchoreov1alpha1.RenderedRelease) bool {
	return obj.GetLabels()[labels.LabelKeyRenderedReleaseName] == release.Name &&
		obj.GetLabels()[labels.LabelKeyRenderedReleaseNamespace] == release.Namespace && hasApplyEntry(obj)
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
		Expect(applyViolations(release, proxy.requests())).To(BeEmpty())
	})

	It("catch an apply over another release's object", func() {
		applyAs("another")
		proxy.reset()
		read()
		applyAs("mine")
		Expect(applyViolations(release, proxy.requests())).To(ConsistOf(ContainSubstring("read, not owned")))
	})

	It("catch an apply over an object no release applied", func() {
		Expect(k8sClient.Create(ctx, newObj("mine"))).To(Succeed())
		read()
		applyAs("mine")
		Expect(applyViolations(release, proxy.requests())).To(ConsistOf(ContainSubstring("read, not owned")))
	})

	It("leave out an object the pass did not read, which an own namespace adopts", func() {
		applyAs("another")
		proxy.reset()
		applyAs("mine")
		Expect(applyViolations(release, proxy.requests())).To(BeEmpty())
	})

	It("catch any write of a pass that refuses, except a dry run", func() {
		release.Status.Conditions = []metav1.Condition{{Type: ConditionResourcesApplied, Status: metav1.ConditionFalse, Reason: ReasonApplyRefused}}
		Expect(plane.Patch(ctx, newObj("mine"), client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName), client.DryRunAll)).To(Succeed())
		Expect(applyViolations(release, proxy.requests())).To(BeEmpty())
		applyAs("mine")
		Expect(applyViolations(release, proxy.requests())).To(ConsistOf(ContainSubstring("written by a pass that refuses")))
	})
})
