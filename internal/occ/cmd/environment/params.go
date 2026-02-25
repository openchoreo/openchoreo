// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

// ListParams defines parameters for listing environments
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single environment
type GetParams struct {
	Namespace       string
	EnvironmentName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single environment
type DeleteParams struct {
	Namespace       string
	EnvironmentName string
}

func (p DeleteParams) GetNamespace() string       { return p.Namespace }
func (p DeleteParams) GetEnvironmentName() string { return p.EnvironmentName }
