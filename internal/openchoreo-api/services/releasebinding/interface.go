// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the release binding service interface.
type Service interface {
	CreateReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.ReleaseBinding, error)
	UpdateReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.ReleaseBinding, error)
	ListReleaseBindings(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseBinding], error)
	GetReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) (*openchoreov1alpha1.ReleaseBinding, error)
	DeleteReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) error

	// ListHooks returns the deployment gate of a release binding. When no hooks are bound
	// (or the hooks feature is disabled) the returned gate has empty PreDeploy/PostDeploy lists.
	ListHooks(ctx context.Context, namespaceName, releaseBindingName string) (*openchoreov1alpha1.DeploymentGateStatus, error)
	// RetryHook asks the controller to re-run one hook binding of the given gate phase by
	// setting the openchoreo.dev/hook-retry annotation to "<phase>/<hookName>".
	RetryHook(ctx context.Context, namespaceName, releaseBindingName string, phase, hookName string) (*openchoreov1alpha1.ReleaseBinding, error)
	// AcknowledgeGate acknowledges an Alert post-deploy failure for the given gate key by
	// setting the openchoreo.dev/gate-acknowledged annotation.
	AcknowledgeGate(ctx context.Context, namespaceName, releaseBindingName, key string) (*openchoreov1alpha1.ReleaseBinding, error)
}

// Deployment gate phase names accepted by RetryHook. Plain strings rather than a
// named type so the generated mocks do not import this package (import cycle in tests).
const (
	HookPhasePreDeploy  = "preDeploy"
	HookPhasePostDeploy = "postDeploy"
)
