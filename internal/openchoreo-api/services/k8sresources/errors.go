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
	// ErrResourceTreeIncomplete means the live tree walk that would prove or
	// disprove membership did not complete (an agent/gateway error, a forbidden
	// edge, or truncation), so absence of the target is not authoritative. It must
	// surface as a retryable server error, never as a 404 that reads as "the
	// resource does not exist".
	ErrResourceTreeIncomplete = errors.New("resource tree discovery was incomplete")
	// ErrResourceMatchAmbiguous means the requested identity matched more than one
	// live tree member, so choosing one would be an arbitrary guess.
	ErrResourceMatchAmbiguous = errors.New("resource identity matched more than one live tree member")
	ErrInvalidContainer       = errors.New("container not found in pod")
	ErrNotCronJobWorkload     = errors.New("release binding component is not a cronjob workload")
	ErrCronJobNotFound        = errors.New("cronjob not found in rendered release")
	ErrTriggerConflict        = errors.New("a job with the same name already exists, retry the trigger")
)
