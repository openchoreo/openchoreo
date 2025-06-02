// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package ptr

// This file includes helper functions for creating pointers.
// These functions can be used where primitive type pointers are required, such as when setting optional
// fields in Kubernetes API objects or working with custom types that follow the Kubernetes conventions
// for nullable values.

func Bool(b bool) *bool {
	return &b
}

func String(s string) *string {
	return &s
}

func Int(i int) *int {
	return &i
}

func Int32(i int32) *int32 {
	return &i
}

func Int64(i int64) *int64 {
	return &i
}
