// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
)

// ResourceOption applies configuration to a BaseResource.
type ResourceOption[T client.Object, L client.ObjectList] func(*BaseResource[T, L])

// WithClient sets the client on BaseResource.
func WithClient[T client.Object, L client.ObjectList](c client.Client) ResourceOption[T, L] {
	return func(b *BaseResource[T, L]) {
		b.client = c
	}
}

// WithScheme sets the scheme on BaseResource.
func WithScheme[T client.Object, L client.ObjectList](s *runtime.Scheme) ResourceOption[T, L] {
	return func(b *BaseResource[T, L]) {
		b.scheme = s
	}
}

// WithNamespace sets the namespace on BaseResource.
func WithNamespace[T client.Object, L client.ObjectList](ns string) ResourceOption[T, L] {
	return func(b *BaseResource[T, L]) {
		b.namespace = ns
	}
}

// WithLabels sets the labels on BaseResource.
func WithLabels[T client.Object, L client.ObjectList](lbls map[string]string) ResourceOption[T, L] {
	return func(b *BaseResource[T, L]) {
		b.labels = lbls
	}
}

// WithConfig sets the CRDConfig on BaseResource.
func WithConfig[T client.Object, L client.ObjectList](cfg constants.CRDConfig) ResourceOption[T, L] {
	return func(b *BaseResource[T, L]) {
		b.config = cfg
	}
}
