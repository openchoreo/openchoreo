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
	Category    string // "" (default: generic), "generic", or "git-credentials"
	FromLiteral []string
	FromFile    []string
	FromEnvFile []string
}

// UpdateInput is the parsed input for the update command.
type UpdateInput struct {
	Namespace   string
	SecretName  string
	FromLiteral []string
	FromFile    []string
	FromEnvFile []string
	Replace     bool
}

func (in UpdateInput) GetNamespace() string  { return in.Namespace }
func (in UpdateInput) GetSecretName() string { return in.SecretName }
