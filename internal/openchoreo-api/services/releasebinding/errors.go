// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import "errors"

var (
	ErrReleaseBindingNotFound      = errors.New("release binding not found")
	ErrReleaseBindingAlreadyExists = errors.New("release binding already exists")
	ErrComponentNotFound           = errors.New("component not found")
	// ErrHookNotFound is returned when a hook binding name is not part of the release binding's gate.
	ErrHookNotFound = errors.New("hook not found in the release binding's deployment gate")
	// ErrGateKeyMismatch is returned when an acknowledgement names a key other than the current gate key.
	ErrGateKeyMismatch = errors.New("gate key does not match the release binding's current gate")
)
