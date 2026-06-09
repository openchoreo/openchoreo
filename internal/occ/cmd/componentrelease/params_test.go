// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListParams_Getters(t *testing.T) {
	p := ListParams{
		Namespace: "org-a",
		Project:   "proj-1",
		Component: "comp-1",
	}
	assert.Equal(t, "org-a", p.GetNamespace())
	assert.Equal(t, "proj-1", p.GetProject())
	assert.Equal(t, "comp-1", p.GetComponent())
}

func TestListParams_Getters_Empty(t *testing.T) {
	p := ListParams{}
	assert.Empty(t, p.GetNamespace())
	assert.Empty(t, p.GetProject())
	assert.Empty(t, p.GetComponent())
}

func TestGetParams_GetNamespace(t *testing.T) {
	p := GetParams{Namespace: "org-a", ComponentReleaseName: "rel-1"}
	assert.Equal(t, "org-a", p.GetNamespace())
}

func TestGetParams_GetNamespace_Empty(t *testing.T) {
	p := GetParams{}
	assert.Empty(t, p.GetNamespace())
}

func TestDeleteParams_Getters(t *testing.T) {
	p := DeleteParams{Namespace: "org-a", ComponentReleaseName: "rel-1"}
	assert.Equal(t, "org-a", p.GetNamespace())
	assert.Equal(t, "rel-1", p.GetComponentReleaseName())
}

func TestDeleteParams_Getters_Empty(t *testing.T) {
	p := DeleteParams{}
	assert.Empty(t, p.GetNamespace())
	assert.Empty(t, p.GetComponentReleaseName())
}
