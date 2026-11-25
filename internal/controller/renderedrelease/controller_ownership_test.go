// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func TestOwnerOf(t *testing.T) {
	release := &openchoreov1alpha1.RenderedRelease{ObjectMeta: metav1.ObjectMeta{Name: "api-prod", Namespace: "acme", UID: "uid-1"}}
	applied := func() *unstructured.Unstructured {
		obj := unstructuredObj("v1", "ConfigMap", "payments", "settings")
		obj.SetLabels(map[string]string{
			labels.LabelKeyManagedBy:                 ControllerName,
			labels.LabelKeyRenderedReleaseResourceID: "settings",
			labels.LabelKeyRenderedReleaseUID:        "uid-1",
			labels.LabelKeyRenderedReleaseName:       "api-prod",
			labels.LabelKeyRenderedReleaseNamespace:  "acme",
		})
		markAppliedByController(obj)
		return obj
	}
	set := func(key, value string) func(*unstructured.Unstructured) {
		return func(obj *unstructured.Unstructured) { obj.SetLabels(withLabel(obj.GetLabels(), key, value)) }
	}
	unset := func(keys ...string) func(*unstructured.Unstructured) {
		return func(obj *unstructured.Unstructured) {
			for _, key := range keys {
				obj.SetLabels(withoutLabel(obj.GetLabels(), key))
			}
		}
	}

	tests := []struct {
		name   string
		change func(*unstructured.Unstructured)
		want   owner
	}{
		{"applied by this release", func(*unstructured.Unstructured) {}, ownedByRelease},
		{"applied by this release, written by others too", func(obj *unstructured.Unstructured) {
			obj.SetManagedFields(append([]metav1.ManagedFieldsEntry{
				{Manager: "helm", Operation: metav1.ManagedFieldsOperationUpdate},
				{Manager: "argocd-controller", Operation: metav1.ManagedFieldsOperationApply},
			}, obj.GetManagedFields()...))
		}, ownedByRelease},
		{"applied by an earlier release of this name, such as before a control plane rebuild",
			set(labels.LabelKeyRenderedReleaseUID, "uid-0"), ownedByRelease},
		{"applied by this release, without the labels it is only looked up by",
			unset(labels.LabelKeyManagedBy, labels.LabelKeyRenderedReleaseUID, labels.LabelKeyRenderedReleaseResourceID), ownedByRelease},

		{"applied by another release", set(labels.LabelKeyRenderedReleaseName, "web-prod"), ownedByAnotherRelease},
		{"applied by a release of this name in another control plane namespace",
			set(labels.LabelKeyRenderedReleaseNamespace, "globex"), ownedByAnotherRelease},

		{"never touched by a release", func(obj *unstructured.Unstructured) {
			obj.SetLabels(nil)
			obj.SetManagedFields(nil)
		}, ownedOutside},
		{"both labels removed", unset(labels.LabelKeyRenderedReleaseName, labels.LabelKeyRenderedReleaseNamespace), ownedOutside},
		{"the name label removed", unset(labels.LabelKeyRenderedReleaseName), ownedOutside},
		{"the namespace label removed", unset(labels.LabelKeyRenderedReleaseNamespace), ownedOutside},
		{"all labels removed, as kubectl replace does", func(obj *unstructured.Unstructured) { obj.SetLabels(nil) }, ownedOutside},
		{"managedFields cleared", func(obj *unstructured.Unstructured) { obj.SetManagedFields(nil) }, ownedOutside},
		{"this release's labels copied by an operator", func(obj *unstructured.Unstructured) {
			setManagedBy(obj, esoFieldManager, metav1.ManagedFieldsOperationApply)
		}, ownedOutside},
		{"another release's labels copied by an operator", func(obj *unstructured.Unstructured) {
			obj.SetLabels(withLabel(obj.GetLabels(), labels.LabelKeyRenderedReleaseName, "web-prod"))
			setManagedBy(obj, esoFieldManager, metav1.ManagedFieldsOperationUpdate)
		}, ownedOutside},
		{"updated, not applied, by this controller", func(obj *unstructured.Unstructured) {
			setManagedBy(obj, ControllerName, metav1.ManagedFieldsOperationUpdate)
		}, ownedOutside},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := applied()
			tt.change(obj)
			if got := ownerOf(obj, release); got != tt.want {
				t.Errorf("ownerOf = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGuardIsOn(t *testing.T) {
	// ProxyClient knows the scope of built-in kinds only, and takes the rest for namespaced.
	plane, err := kubernetesClient.NewProxyClient("https://gateway.test", "dataplane/test", "_cluster", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	release := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "api-prod", Namespace: "acme"},
		Spec: openchoreov1alpha1.RenderedReleaseSpec{
			Owner:           openchoreov1alpha1.RenderedReleaseOwner{ProjectName: "shop", ComponentName: "api"},
			EnvironmentName: "prod",
		},
	}
	own := dpkubernetes.DataPlaneNamespace("acme", "shop", "prod")
	if own != "dp-acme-shop-prod-"+own[len(own)-8:] {
		t.Fatalf("unexpected own namespace %s", own)
	}
	inNamespace := func(namespace string) *unstructured.Unstructured {
		return unstructuredObj("apps/v1", "Deployment", namespace, "api")
	}
	namespace := func(name string) *unstructured.Unstructured { return unstructuredObj("v1", "Namespace", "", name) }
	clusterRole := unstructuredObj("rbac.authorization.k8s.io/v1", "ClusterRole", "", "reader")
	volume := unstructuredObj("v1", "PersistentVolume", "", "orders")
	customClusterScoped := unstructuredObj("example.com/v1", "ClusterWidget", "", "w")

	tests := []struct {
		name     string
		rendered []*unstructured.Unstructured
		want     bool
	}{
		{"into its own namespace", []*unstructured.Unstructured{inNamespace(own)}, false},
		{"its own Namespace", []*unstructured.Unstructured{namespace(own)}, false},
		{"into its own and another namespace", []*unstructured.Unstructured{inNamespace("payments"), inNamespace(own)}, false},
		{"into its own namespace and cluster-scoped objects",
			[]*unstructured.Unstructured{clusterRole, volume, customClusterScoped, namespace("extra"), inNamespace(own)}, false},

		{"into a namespace that existed before", []*unstructured.Unstructured{inNamespace("payments")}, true},
		{"another Namespace", []*unstructured.Unstructured{namespace("payments")}, true},
		{"cluster-scoped objects only", []*unstructured.Unstructured{clusterRole, volume}, true},
		{"a cluster-scoped custom resource only", []*unstructured.Unstructured{customClusterScoped}, true},
		{"a cluster-scoped object whose manifest sets its own namespace",
			[]*unstructured.Unstructured{unstructuredObj("rbac.authorization.k8s.io/v1", "ClusterRole", own, "reader")}, true},
		{"another kind that is also called Namespace", []*unstructured.Unstructured{unstructuredObj("example.com/v1", "Namespace", "", own)}, true},
		{"nothing", nil, true},

		{"into a namespace that only looks like a generated one", []*unstructured.Unstructured{inNamespace("dp-analytics")}, true},
		{"into a namespace whose name starts like its own", []*unstructured.Unstructured{inNamespace(own + "-2")}, true},
		{"into a namespace whose name is cut off from its own", []*unstructured.Unstructured{inNamespace(own[:len(own)-1])}, true},
		{"into another project's own namespace",
			[]*unstructured.Unstructured{inNamespace(dpkubernetes.DataPlaneNamespace("acme", "billing", "prod"))}, true},
		{"into its project's namespace of another environment",
			[]*unstructured.Unstructured{inNamespace(dpkubernetes.DataPlaneNamespace("acme", "shop", "staging"))}, true},
		{"into a namespace of the same project in another control plane namespace",
			[]*unstructured.Unstructured{inNamespace(dpkubernetes.DataPlaneNamespace("globex", "shop", "prod"))}, true},
	}
	for _, tt := range tests {
		t.Run("renders "+tt.name, func(t *testing.T) {
			if got := guardIsOn(release, tt.rendered, plane); got != tt.want {
				t.Errorf("guardIsOn = %v, want %v", got, tt.want)
			}
		})
	}
}
