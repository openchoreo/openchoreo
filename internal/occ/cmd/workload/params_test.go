// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateParams_GetNamespace(t *testing.T) {
	p := CreateParams{NamespaceName: "org-a"}
	assert.Equal(t, "org-a", p.GetNamespace())
}

func TestCreateParams_GetNamespace_Empty(t *testing.T) {
	p := CreateParams{}
	assert.Empty(t, p.GetNamespace())
}

func TestListParams_GetNamespace(t *testing.T) {
	p := ListParams{Namespace: "org-a"}
	assert.Equal(t, "org-a", p.GetNamespace())
}

func TestListParams_GetNamespace_Empty(t *testing.T) {
	p := ListParams{}
	assert.Empty(t, p.GetNamespace())
}

func TestGetParams_GetNamespace(t *testing.T) {
	p := GetParams{Namespace: "org-a", WorkloadName: "wl-1"}
	assert.Equal(t, "org-a", p.GetNamespace())
}

func TestGetParams_GetNamespace_Empty(t *testing.T) {
	p := GetParams{}
	assert.Empty(t, p.GetNamespace())
}

func TestDeleteParams_Getters(t *testing.T) {
	p := DeleteParams{Namespace: "org-a", WorkloadName: "wl-1"}
	assert.Equal(t, "org-a", p.GetNamespace())
	assert.Equal(t, "wl-1", p.GetWorkloadName())
}

func TestDeleteParams_Getters_Empty(t *testing.T) {
	p := DeleteParams{}
	assert.Empty(t, p.GetNamespace())
	assert.Empty(t, p.GetWorkloadName())
}
