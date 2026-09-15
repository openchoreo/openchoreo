// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package protocol

import (
	"strings"
	"testing"
)

func TestValidateGVR(t *testing.T) {
	tests := []struct {
		name           string
		group          string
		version        string
		resource       string
		wantErr        bool
		wantFieldInErr string // substring the error must name; ignored when wantErr is false
	}{
		{name: "core group valid", group: "", version: "v1", resource: "secrets"},
		{name: "named group valid", group: "apps", version: "v1", resource: "replicasets"},
		{name: "dotted group and alpha version valid", group: "gateway.networking.k8s.io", version: "v1alpha1", resource: "externalsecrets"},
		{name: "beta version valid", group: "batch", version: "v2beta1", resource: "jobs"},
		{name: "non-kube CRD version names valid", group: "example.com", version: "v1gamma1", resource: "widgets"},
		{name: "preview CRD version valid", group: "example.com", version: "preview1", resource: "widgets"},
		{name: "hyphenated CRD version valid", group: "example.com", version: "v1-alpha1", resource: "widgets"},

		{name: "traversal resource rejected", group: "", version: "v1", resource: "../../../../api/v1/secrets", wantErr: true, wantFieldInErr: "resource"},
		{name: "uppercase resource rejected", group: "", version: "v1", resource: "Pods", wantErr: true, wantFieldInErr: "resource"},
		{name: "dotted resource rejected", group: "", version: "v1", resource: "secrets.core", wantErr: true, wantFieldInErr: "resource"},
		{name: "empty resource rejected", group: "", version: "v1", resource: "", wantErr: true, wantFieldInErr: "resource"},
		{name: "slash in group rejected", group: "apps/", version: "v1", resource: "pods", wantErr: true, wantFieldInErr: "group"},
		{name: "uppercase group rejected", group: "Apps", version: "v1", resource: "pods", wantErr: true, wantFieldInErr: "group"},
		{name: "uppercase version rejected", group: "", version: "V1", resource: "pods", wantErr: true, wantFieldInErr: "version"},
		{name: "slash in version rejected", group: "", version: "v1/", resource: "pods", wantErr: true, wantFieldInErr: "version"},
		{name: "dotted version rejected", group: "", version: "v1.0", resource: "pods", wantErr: true, wantFieldInErr: "version"},
		{name: "empty version rejected", group: "", version: "", resource: "pods", wantErr: true, wantFieldInErr: "version"},
		{name: "digit-leading version rejected", group: "", version: "1v", resource: "pods", wantErr: true, wantFieldInErr: "version"},
		{name: "over-63-byte version rejected", group: "", version: "v" + strings.Repeat("1", 63), resource: "pods", wantErr: true, wantFieldInErr: "version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGVR(tt.group, tt.version, tt.resource)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateGVR(%q,%q,%q) = nil, want error", tt.group, tt.version, tt.resource)
				}
				if !strings.Contains(err.Error(), tt.wantFieldInErr) {
					t.Fatalf("error %q does not name field %q", err.Error(), tt.wantFieldInErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateGVR(%q,%q,%q) = %v, want nil", tt.group, tt.version, tt.resource, err)
			}
		})
	}
}

func TestValidateVersionRules(t *testing.T) {
	// A version name is a DNS-1035 label, not the kube ordering convention: any
	// lowercase label starting with a letter is legal, so arbitrary CRD version
	// names (v1gamma1, preview1, v1-alpha1) pass while slashes, dots, uppercase,
	// a leading digit, and an over-63-byte value are rejected — the last two keep
	// a malformed version out of a REST path.
	for _, bad := range []string{"v1/", "v1/secrets", "v1.0", "V1", "1v", "", "v_1", "v" + strings.Repeat("1", 63)} {
		if err := ValidateVersion(bad); err == nil {
			t.Errorf("ValidateVersion(%q) = nil, want error", bad)
		}
	}
	for _, good := range []string{"v1", "v10", "v1alpha1", "v2beta3", "v1gamma1", "preview1", "v1-alpha1", "v" + strings.Repeat("1", 62)} {
		if err := ValidateVersion(good); err != nil {
			t.Errorf("ValidateVersion(%q) = %v, want nil", good, err)
		}
	}
}
