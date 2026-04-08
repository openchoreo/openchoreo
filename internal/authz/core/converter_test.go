// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"reflect"
	"testing"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

func TestGetAuthzSubjectContext(t *testing.T) {
	tests := []struct {
		name    string
		authCtx *auth.SubjectContext
		want    *SubjectContext
	}{
		{
			name:    "nil input returns nil",
			authCtx: nil,
			want:    nil,
		},
		{
			name: "fully populated context is converted",
			authCtx: &auth.SubjectContext{
				ID:                "user-123",
				Type:              "user",
				EntitlementClaim:  "groups",
				EntitlementValues: []string{"admin", "dev"},
			},
			want: &SubjectContext{
				Type:              "user",
				EntitlementClaim:  "groups",
				EntitlementValues: []string{"admin", "dev"},
			},
		},
		{
			name: "empty fields are preserved",
			authCtx: &auth.SubjectContext{
				Type:              "",
				EntitlementClaim:  "",
				EntitlementValues: nil,
			},
			want: &SubjectContext{
				Type:              "",
				EntitlementClaim:  "",
				EntitlementValues: nil,
			},
		},
		{
			name: "empty slice is preserved distinct from nil",
			authCtx: &auth.SubjectContext{
				Type:              "service_account",
				EntitlementClaim:  "scopes",
				EntitlementValues: []string{},
			},
			want: &SubjectContext{
				Type:              "service_account",
				EntitlementClaim:  "scopes",
				EntitlementValues: []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAuthzSubjectContext(tt.authCtx)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetAuthzSubjectContext() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
