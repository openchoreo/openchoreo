// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"
)

// ResourceHandler is an interface that defines the operations that can be performed on the external resources
// that are managed by the deployment controller during the reconciliation process.
//
// Following are the operations that are performed by the deployment controller during the reconciliation process:
//  1. If the resource is not required (based on `IsRequired`), delete it and skip further reconciliation.
//  2. If the resource is required, fetch its current state using `GetCurrentState`.
//     2.1.If the resource does not exist (GetCurrentState returns nil,nil), create it using `Create`.
//     2.2 If the resource exists, update it using `Update` to match the desired state.
//
// Example Usage:
// Consider an external resource like an S3 bucket managed by the deployment controller.
//   - `Name` could return "S3Bucket".
//   - `IsRequired` might evaluate the deployment context to determine if the bucket should exist or not.
//   - `GetCurrentState` would fetch the current state of the bucket (e.g., bucket name, region, configurations).
//   - `Create` would create the bucket based on the deployment context.
//   - `Update` would adjust bucket configurations if there are differences between the provided current state and the
//     desired state derived from the deployment context.
//   - `Delete` would remove the bucket.
//
// Another example could be managing a database:
// - `Name` could return "DatabaseInstance".
// - `IsRequired` might evaluate the deployment context to decide if the database instance is necessary.
// - `GetCurrentState` would fetch details about the database instance, such as its capacity or engine version.
// - `Create` would provision the database instance if needed.
// - `Update` would modify configurations like storage capacity or backup settings.
// - `Delete` would delete the database instance when it is no longer required.
type ResourceHandler[T any] interface {
	// Name returns the name of the external resource.
	// The name should be in PascalCase in order to keep the consistency.
	Name() string

	// IsRequired indicates whether the external resource needs to be configured or not based on the endpoint context.
	// If this returns false, the controller will attempt to delete the resource.
	IsRequired(ctx *T) bool

	// GetCurrentState returns the current state of the external resource.
	// If the resource does not exist, the implementation should return nil.
	GetCurrentState(ctx context.Context, resourceCtx *T) (interface{}, error)

	// Create creates the external resource.
	Create(ctx context.Context, resourceCtx *T) error

	// Update updates the external resource.
	// The currentState parameter will provide the current state of the resource that is returned by GetCurrentState
	// Implementation should compare the current state with the new derived state and update the resource accordingly.
	Update(ctx context.Context, resourceCtx *T, currentState interface{}) error

	// Delete deletes the external resource.
	// The implementation should handle the case where the resource does not exist and return nil.
	Delete(ctx context.Context, resourceCtx *T) error
}
