// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package occ

import (
	"github.com/openchoreo/openchoreo/internal/occ/cmd/apply"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/componentrelease"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/delete"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/logout"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/releasebinding"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workload"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CommandImplementation struct{}

var _ api.CommandImplementationInterface = &CommandImplementation{}

func NewCommandImplementation() *CommandImplementation {
	return &CommandImplementation{}
}

func (c *CommandImplementation) CreateWorkload(params api.CreateWorkloadParams) error {
	workloadImpl := workload.NewWorkloadImpl(constants.WorkloadV1Config)
	return workloadImpl.CreateWorkload(params)
}

// Delete Operations

func (c *CommandImplementation) Delete(params api.DeleteParams) error {
	deleteImpl := delete.NewDeleteImpl()
	return deleteImpl.Delete(params)
}

// Authentication Operations

func (c *CommandImplementation) Login(params api.LoginParams) error {
	loginImpl := login.NewAuthImpl()
	return loginImpl.Login(params)
}

func (c *CommandImplementation) IsLoggedIn() bool {
	loginImpl := login.NewAuthImpl()
	return loginImpl.IsLoggedIn()
}

func (c *CommandImplementation) GetLoginPrompt() string {
	loginImpl := login.NewAuthImpl()
	return loginImpl.GetLoginPrompt()
}

func (c *CommandImplementation) Logout() error {
	logoutImpl := logout.NewLogoutImpl()
	return logoutImpl.Logout()
}

func (c *CommandImplementation) Apply(params api.ApplyParams) error {
	applyImpl := apply.NewApplyImpl()
	return applyImpl.Apply(params)
}

// Config Context Operations

func (c *CommandImplementation) AddContext(params api.AddContextParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.AddContext(params)
}

func (c *CommandImplementation) ListContexts() error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.ListContexts()
}

func (c *CommandImplementation) DeleteContext(params api.DeleteContextParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.DeleteContext(params)
}

func (c *CommandImplementation) UpdateContext(params api.UpdateContextParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.UpdateContext(params)
}

func (c *CommandImplementation) UseContext(params api.UseContextParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.UseContext(params)
}

func (c *CommandImplementation) DescribeContext(params api.DescribeContextParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.DescribeContext(params)
}

func (c *CommandImplementation) AddControlPlane(params api.AddControlPlaneParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.AddControlPlane(params)
}

func (c *CommandImplementation) ListControlPlanes() error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.ListControlPlanes()
}

func (c *CommandImplementation) UpdateControlPlane(params api.UpdateControlPlaneParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.UpdateControlPlane(params)
}

func (c *CommandImplementation) DeleteControlPlane(params api.DeleteControlPlaneParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.DeleteControlPlane(params)
}

func (c *CommandImplementation) AddCredentials(params api.AddCredentialsParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.AddCredentials(params)
}

func (c *CommandImplementation) ListCredentials() error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.ListCredentials()
}

func (c *CommandImplementation) DeleteCredentials(params api.DeleteCredentialsParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.DeleteCredentials(params)
}

// Component Release Operations (File-System Mode)

func (c *CommandImplementation) GenerateComponentRelease(params api.GenerateComponentReleaseParams) error {
	releaseImpl := componentrelease.NewComponentReleaseImpl()
	return releaseImpl.GenerateComponentRelease(params)
}

// Component Release Operations (Api-Server Mode)

func (c *CommandImplementation) ListComponentReleases(params api.ListComponentReleasesParams) error {
	componentReleaseImpl := componentrelease.NewComponentReleaseImpl()
	return componentReleaseImpl.ListComponentReleases(params)
}

// Release Binding Operations (File-System Mode)

func (c *CommandImplementation) GenerateReleaseBinding(params api.GenerateReleaseBindingParams) error {
	bindingImpl := releasebinding.NewReleaseBindingImpl()
	return bindingImpl.GenerateReleaseBinding(params)
}

func (c *CommandImplementation) ListReleaseBindings(params api.ListReleaseBindingsParams) error {
	bindingImpl := releasebinding.NewReleaseBindingImpl()
	return bindingImpl.ListReleaseBindings(params)
}
