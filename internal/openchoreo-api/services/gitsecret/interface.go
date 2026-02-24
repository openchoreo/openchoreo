// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gitsecret

import "context"

// GitSecretInfo represents a git secret resource.
type GitSecretInfo struct {
	Name      string
	Namespace string
}

// CreateGitSecretParams holds the parameters for creating a git secret.
type CreateGitSecretParams struct {
	SecretName string
	SecretType string
	Username   string
	Token      string
	SSHKey     string
	SSHKeyID   string
}

// Service defines the git secret operations.
type Service interface {
	// ListGitSecrets returns all git secrets in a namespace.
	ListGitSecrets(ctx context.Context, namespaceName string) ([]GitSecretInfo, error)
	// CreateGitSecret creates a new git secret.
	CreateGitSecret(ctx context.Context, namespaceName string, req *CreateGitSecretParams) (*GitSecretInfo, error)
	// DeleteGitSecret deletes a git secret by name.
	DeleteGitSecret(ctx context.Context, namespaceName, secretName string) error
}
