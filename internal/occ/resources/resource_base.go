// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
)

// ResourceBase provides common fields and functionality for all resources
type ResourceBase struct {
	namespace string
	config    constants.CRDConfig
}

// ResourceBaseOption configures a ResourceBase
type ResourceBaseOption func(*ResourceBase)

// NewResourceBase creates a new ResourceBase with options
func NewResourceBase(opts ...ResourceBaseOption) *ResourceBase {
	base := &ResourceBase{}
	for _, opt := range opts {
		opt(base)
	}
	return base
}

// WithResourceNamespace sets the namespace for the resource
func WithResourceNamespace(namespace string) ResourceBaseOption {
	return func(base *ResourceBase) {
		base.namespace = namespace
	}
}

// WithResourceConfig sets the CRD config for the resource
func WithResourceConfig(config constants.CRDConfig) ResourceBaseOption {
	return func(base *ResourceBase) {
		base.config = config
	}
}

func (base *ResourceBase) GetNamespace() string {
	return base.namespace
}

func (base *ResourceBase) GetConfig() constants.CRDConfig {
	return base.config
}

func (base *ResourceBase) SetNamespace(namespace string) {
	base.namespace = namespace
}
