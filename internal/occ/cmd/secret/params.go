// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

// ListParams defines parameters for listing secrets.
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single secret.
type GetParams struct {
	Namespace  string
	SecretName string
}

func (p GetParams) GetNamespace() string  { return p.Namespace }
func (p GetParams) GetSecretName() string { return p.SecretName }

// DeleteParams defines parameters for deleting a secret.
type DeleteParams struct {
	Namespace  string
	SecretName string
}

func (p DeleteParams) GetNamespace() string  { return p.Namespace }
func (p DeleteParams) GetSecretName() string { return p.SecretName }

// CreateInput is the shared parsed input for all create subcommands.
type CreateInput struct {
	Namespace   string
	SecretName  string
	TargetPlane string // raw "Kind/Name"
	FromLiteral []string
	FromFile    []string
	FromEnvFile []string
}
