// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

type Context struct {
	ServiceBinding *openchoreov1alpha1.ServiceBinding
	ServiceClass   *openchoreov1alpha1.ServiceClass
	APIClasses     map[string]*openchoreov1alpha1.APIClass
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
