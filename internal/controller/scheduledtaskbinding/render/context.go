// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

type Context struct {
	ScheduledTaskBinding      *openchoreov1alpha1.ScheduledTaskBinding
	ScheduledTaskClass        *openchoreov1alpha1.ScheduledTaskClass
	DataPlane                 *openchoreov1alpha1.DataPlane
	ImagePullSecretReferences map[string]*openchoreov1alpha1.SecretReference

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
