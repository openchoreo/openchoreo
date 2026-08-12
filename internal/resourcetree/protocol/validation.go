// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package protocol

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// ValidateGVR validates a group/version/resource triple as clean Kubernetes
// syntax, returning the first defect with the offending field named so a caller
// can prefix its own context. It is the single guard both the config boundary and
// the agent wire boundary run so a resource string can never smuggle path-cleaning
// segments (../) into a REST path or slip past the core-Secret strip by spelling.
func ValidateGVR(group, version, resource string) error {
	if err := ValidateGroup(group); err != nil {
		return fmt.Errorf("group %w", err)
	}
	if err := ValidateVersion(version); err != nil {
		return fmt.Errorf("version %w", err)
	}
	if err := ValidateResource(resource); err != nil {
		return fmt.Errorf("resource %w", err)
	}
	return nil
}

// ValidateGroup accepts the empty core API group and, for anything else, a
// DNS-1123 subdomain: lowercase, dots allowed (apps, gateway.networking.k8s.io).
func ValidateGroup(group string) error {
	if group == "" {
		return nil
	}
	if errs := validation.IsDNS1123Subdomain(group); len(errs) > 0 {
		return fmt.Errorf("%q is not a valid API group: %s", group, strings.Join(errs, "; "))
	}
	return nil
}

// ValidateVersion requires the version to be a DNS-1035 label, which is what a
// CRD API version name must be: lowercase, starting with a letter, alphanumeric
// plus '-', at most 63 bytes. That accepts v1, v1alpha1, v2beta1, v1gamma1,
// preview1, v1-alpha1 while still rejecting V1, v1/, v1.0, empty, and any value
// with a slash, dot, or uppercase.
func ValidateVersion(version string) error {
	if errs := validation.IsDNS1035Label(version); len(errs) > 0 {
		return fmt.Errorf("%q is not a valid API version: %s", version, strings.Join(errs, "; "))
	}
	return nil
}

// ValidateResource requires the plural REST resource name to be a DNS-1123
// label: lowercase alphanumeric plus '-', non-empty, no dots and no '/'. This is
// what rejects a traversal string such as "../../../../api/v1/secrets".
func ValidateResource(resource string) error {
	if errs := validation.IsDNS1123Label(resource); len(errs) > 0 {
		return fmt.Errorf("%q is not a valid resource name: %s", resource, strings.Join(errs, "; "))
	}
	return nil
}
