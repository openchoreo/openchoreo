// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

type Context struct {
	WorkloadBinding *choreov1.WorkloadBinding
	WorkloadClass   *choreov1.WorkloadClass
	Endpoints       []choreov1.EndpointV2

	// Stores the errors encountered during rendering.
	errs []error
}

func (c *Context) AddError(err error) {
	if err != nil {
		c.errs = append(c.errs, err)
	}
}

func (c *Context) Errors() []error {
	if len(c.errs) == 0 {
		return nil
	}
	return c.errs
}

func (c *Context) Error() error {
	if len(c.errs) > 0 {
		return utilerrors.NewAggregate(c.errs)
	}
	return nil
}
