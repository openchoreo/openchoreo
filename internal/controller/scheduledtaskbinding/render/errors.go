// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidSchedule       = errors.New("invalid schedule configuration")
	ErrInvalidResource       = errors.New("invalid resource configuration")
	ErrMissingContainer      = errors.New("missing container configuration")
	ErrMissingImage          = errors.New("missing image configuration")
	ErrInvalidCronExpression = errors.New("invalid cron expression")
)

// MergeError wraps a merge operation error with additional context
func MergeError(err error) error {
	return fmt.Errorf("merge operation failed: %w", err)
}
