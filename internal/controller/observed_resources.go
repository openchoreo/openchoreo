// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// groupHashLength is how many hex characters of the API group's hash an
// observed resource id carries.
const groupHashLength = 8

// ObservedResourceID returns the RenderedRelease id of an observed object:
// <kind>-<name> for the core group and <kind>-<name>-<hash of the group> for
// other groups. Objects of the same kind and name in two groups get different
// ids, and one object listed under two versions gets the same id. An id longer
// than 63 characters is shortened with a hash.
func ObservedResourceID(apiVersion, kind, name string) string {
	gv, _ := schema.ParseGroupVersion(apiVersion)
	kind = strings.ToLower(kind)

	if gv.Group == "" {
		id := kind + "-" + name
		if len(id) <= dpkubernetes.MaxLabelNameLength {
			return id
		}
		return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxLabelNameLength, kind, name)
	}

	sum := sha256.Sum256([]byte(gv.Group))
	id := kind + "-" + name + "-" + hex.EncodeToString(sum[:])[:groupHashLength]
	if len(id) <= dpkubernetes.MaxLabelNameLength {
		return id
	}
	return dpkubernetes.GenerateK8sNameWithExtraHashInput(dpkubernetes.MaxLabelNameLength, gv.Group, kind, name)
}

// ObservedResources appends the claims to base as observed resources in
// namespace. A claim whose id is already in the list, because it names an
// object listed before under another version, is left out and returned in
// skipped.
func ObservedResources(
	base []openchoreov1alpha1.ObservedResource,
	namespace string,
	claims []openchoreov1alpha1.ObservedResourceClaim,
) (resources []openchoreov1alpha1.ObservedResource, skipped []openchoreov1alpha1.ObservedResourceClaim) {
	resources = make([]openchoreov1alpha1.ObservedResource, 0, len(base)+len(claims))
	ids := make(map[string]bool, len(base)+len(claims))
	for _, r := range base {
		resources = append(resources, r)
		ids[r.ID] = true
	}
	for _, c := range claims {
		id := ObservedResourceID(c.APIVersion, c.Kind, c.Name)
		if ids[id] {
			skipped = append(skipped, c)
			continue
		}
		ids[id] = true
		resources = append(resources, openchoreov1alpha1.ObservedResource{
			ID:         id,
			APIVersion: c.APIVersion,
			Kind:       c.Kind,
			Name:       c.Name,
			Namespace:  namespace,
		})
	}
	return resources, skipped
}
