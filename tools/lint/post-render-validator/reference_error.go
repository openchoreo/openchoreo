// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import "fmt"

// referenceError retains resource kind names in user-facing lookup diagnostics.
type referenceError struct {
	ownerKind, ownerName, targetKind, targetName string
	activeDirectory, unsupported                 bool
}

func (e referenceError) Error() string {
	if e.unsupported {
		return fmt.Sprintf("%s %s references unsupported data plane kind %q", e.ownerKind, e.ownerName, e.targetKind)
	}
	suffix := ""
	if e.activeDirectory {
		suffix = " in the active directory"
	}
	return fmt.Sprintf("%s %s references %s %q, but it was not found%s", e.ownerKind, e.ownerName, e.targetKind, e.targetName, suffix)
}
