// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import "maps"

// Option is a function that configures a Pipeline.
type Option func(*Pipeline)

// WithValidation enables or disables resource validation.
// When enabled, rendered resources are validated to ensure they have
// required fields (kind, apiVersion, metadata.name).
// Validation failures always cause rendering to fail.
func WithValidation(enabled bool) Option {
	return func(p *Pipeline) {
		p.options.EnableValidation = enabled
	}
}

// WithResourceLabels adds labels to all rendered resources.
// These labels are merged with any existing labels on the resources.
func WithResourceLabels(labels map[string]string) Option {
	return func(p *Pipeline) {
		if p.options.ResourceLabels == nil {
			p.options.ResourceLabels = make(map[string]string)
		}
		maps.Copy(p.options.ResourceLabels, labels)
	}
}

// WithResourceAnnotations adds annotations to all rendered resources.
// These annotations are merged with any existing annotations on the resources.
func WithResourceAnnotations(annotations map[string]string) Option {
	return func(p *Pipeline) {
		if p.options.ResourceAnnotations == nil {
			p.options.ResourceAnnotations = make(map[string]string)
		}
		maps.Copy(p.options.ResourceAnnotations, annotations)
	}
}
