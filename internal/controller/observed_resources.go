// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// ObservedResourceID returns the RenderedRelease id of an observed object:
// <kind>-<name>-<hash>, shortened to 63 characters. The hash covers the API
// version, kind, namespace and name, joined by "/" so the parts can't run into
// each other. Different objects get different ids, and so does one object
// listed under two API versions.
func ObservedResourceID(apiVersion, kind, namespace, name string) string {
	hashInput := strings.Join([]string{apiVersion, kind, namespace, name}, "/")
	return dpkubernetes.GenerateK8sNameWithHashInput(dpkubernetes.MaxLabelNameLength, hashInput,
		strings.ToLower(kind), name)
}

// ObservedResources returns the claims as RenderedRelease observed resources in
// namespace.
func ObservedResources(
	namespace string,
	claims []openchoreov1alpha1.ObservedResourceClaim,
) []openchoreov1alpha1.ObservedResource {
	resources := make([]openchoreov1alpha1.ObservedResource, 0, len(claims))
	for _, c := range claims {
		resources = append(resources, openchoreov1alpha1.ObservedResource{
			ID:         ObservedResourceID(c.APIVersion, c.Kind, namespace, c.Name),
			APIVersion: c.APIVersion,
			Kind:       c.Kind,
			Name:       c.Name,
			Namespace:  namespace,
		})
	}
	return resources
}
