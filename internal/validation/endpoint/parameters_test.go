// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestValidateValues(t *testing.T) {
	section := func(s string) *v1alpha1.SchemaSection {
		return &v1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(s)}}
	}
	raw := func(s string) *runtime.RawExtension {
		return &runtime.RawExtension{Raw: []byte(s)}
	}
	const schema = `{"type":"object","required":["pathPrefix","host"],"properties":{` +
		`"pathPrefix":{"type":"string","default":"/"},"host":{"type":"string"}}}`

	tests := []struct {
		name    string
		section *v1alpha1.SchemaSection
		values  *runtime.RawExtension
		wantErr string
	}{
		{name: "nil section accepts anything", section: nil, values: raw(`{"x":1}`)},
		{name: "empty section accepts anything", section: &v1alpha1.SchemaSection{}, values: raw(`{"x":1}`)},
		{name: "valid values", section: section(schema), values: raw(`{"pathPrefix":"/a","host":"h"}`)},
		{name: "required field satisfied by default", section: section(schema), values: raw(`{"host":"h"}`)},
		{name: "required field without default missing", section: section(schema), values: nil, wantErr: "host"},
		{name: "wrong type", section: section(schema), values: raw(`{"host":1}`), wantErr: "host"},
		{name: "unparseable values", section: section(schema), values: raw(`{`), wantErr: "failed to parse values"},
		{name: "unparseable schema", section: section(`{"type":`), values: nil, wantErr: "invalid schema"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateValues(tt.section, tt.values)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
