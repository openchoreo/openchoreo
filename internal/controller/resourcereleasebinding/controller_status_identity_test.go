// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("currentStatusEntries", func() {
	manifest := func(id, raw string) openchoreov1alpha1.RenderedManifest {
		return openchoreov1alpha1.RenderedManifest{ID: id, Object: &runtime.RawExtension{Raw: []byte(raw)}}
	}
	configMapStatus := func(id, namespace, name string) openchoreov1alpha1.RenderedManifestStatus {
		return openchoreov1alpha1.RenderedManifestStatus{
			ID: id, Version: "v1", Kind: "ConfigMap", Namespace: namespace, Name: name,
		}
	}
	ids := func(entries []openchoreov1alpha1.RenderedManifestStatus) []string {
		out := make([]string, 0, len(entries))
		for i := range entries {
			out = append(out, entries[i].ID+"/"+entries[i].Name)
		}
		return out
	}
	rrWith := func(spec []openchoreov1alpha1.RenderedManifest, status []openchoreov1alpha1.RenderedManifestStatus) *openchoreov1alpha1.RenderedRelease {
		return &openchoreov1alpha1.RenderedRelease{
			Spec:   openchoreov1alpha1.RenderedReleaseSpec{Resources: spec},
			Status: openchoreov1alpha1.RenderedReleaseStatus{Resources: status},
		}
	}

	It("keeps an entry whose object matches the rendered one", func() {
		rr := rrWith(
			[]openchoreov1alpha1.RenderedManifest{manifest("config",
				`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"config-r1","namespace":"db"}}`)},
			[]openchoreov1alpha1.RenderedManifestStatus{configMapStatus("config", "db", "config-r1")},
		)
		Expect(ids(currentStatusEntries(rr, logr.Discard()))).To(Equal([]string{"config/config-r1"}))
	})

	It("drops an entry left from the object previously rendered under the same id", func() {
		rr := rrWith(
			[]openchoreov1alpha1.RenderedManifest{manifest("config",
				`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"config-r2","namespace":"db"}}`)},
			[]openchoreov1alpha1.RenderedManifestStatus{configMapStatus("config", "db", "config-r1")},
		)
		Expect(currentStatusEntries(rr, logr.Discard())).To(BeEmpty())
	})

	It("drops an entry whose namespace or kind differs", func() {
		rr := rrWith(
			[]openchoreov1alpha1.RenderedManifest{
				manifest("a", `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"a","namespace":"new"}}`),
				manifest("b", `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"b","namespace":"db"}}`),
			},
			[]openchoreov1alpha1.RenderedManifestStatus{configMapStatus("a", "old", "a"), configMapStatus("b", "db", "b")},
		)
		Expect(currentStatusEntries(rr, logr.Discard())).To(BeEmpty())
	})

	It("ignores the API version", func() {
		rr := rrWith(
			[]openchoreov1alpha1.RenderedManifest{manifest("config",
				`{"apiVersion":"v2","kind":"ConfigMap","metadata":{"name":"config","namespace":"db"}}`)},
			[]openchoreov1alpha1.RenderedManifestStatus{configMapStatus("config", "db", "config")},
		)
		Expect(currentStatusEntries(rr, logr.Discard())).To(HaveLen(1))
	})

	It("keeps a cluster-scoped entry whose manifest sets a namespace the server cleared", func() {
		clusterRole := func(id, name string) openchoreov1alpha1.RenderedManifestStatus {
			return openchoreov1alpha1.RenderedManifestStatus{
				ID: id, Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole", Name: name,
			}
		}
		rr := rrWith(
			[]openchoreov1alpha1.RenderedManifest{
				manifest("reader", `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"ClusterRole","metadata":{"name":"reader-r2","namespace":"default"}}`),
				manifest("writer", `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"ClusterRole","metadata":{"name":"writer-r2","namespace":"default"}}`),
			},
			[]openchoreov1alpha1.RenderedManifestStatus{clusterRole("reader", "reader-r2"), clusterRole("writer", "writer-r1")},
		)
		Expect(ids(currentStatusEntries(rr, logr.Discard()))).To(Equal([]string{"reader/reader-r2"}))
	})

	It("keeps entries whose rendered manifest cannot be decoded, or that are no longer rendered", func() {
		rr := rrWith(
			[]openchoreov1alpha1.RenderedManifest{manifest("broken", `not json`)},
			[]openchoreov1alpha1.RenderedManifestStatus{configMapStatus("broken", "db", "x"), configMapStatus("gone", "db", "y")},
		)
		Expect(ids(currentStatusEntries(rr, logr.Discard()))).To(Equal([]string{"broken/x", "gone/y"}))
	})
})
