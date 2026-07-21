// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import "errors"

var (
	ErrReleaseBindingNotFound   = errors.New("release binding not found")
	ErrRenderedReleaseNotFound  = errors.New("rendered release not found")
	ErrComponentReleaseNotFound = errors.New("component release not found")
	ErrEnvironmentNotFound      = errors.New("environment not found")
	ErrResourceNotFound         = errors.New("resource not found in rendered release")
	ErrInvalidContainer         = errors.New("container not found in pod")
	ErrNotCronJobWorkload       = errors.New("release binding component is not a cronjob workload")
	ErrCronJobNotFound          = errors.New("cronjob not found in rendered release")
	ErrTriggerConflict          = errors.New("a job with the same name already exists, retry the trigger")
)
