// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// Domain errors re-exported from authz core for service layer consistency.
var (
	ErrRoleNotFound             = authzcore.ErrRoleNotFound
	ErrRoleAlreadyExists        = authzcore.ErrRoleAlreadyExists
	ErrRoleInUse                = authzcore.ErrRoleInUse
	ErrRoleBindingNotFound      = authzcore.ErrRoleMappingNotFound
	ErrRoleBindingAlreadyExists = authzcore.ErrRoleMappingAlreadyExists
)
