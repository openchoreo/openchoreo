// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

// Resource associates an API object with its export identity. Namespace is empty
// for cluster-scoped objects, including Namespace objects. Object retains the
// generated API value, including its exposed metadata, spec, and status.
type Resource struct {
	Kind      string
	Namespace string
	Name      string
	Object    any
}

// SerializedFile is one manifest and its path relative to the export directory.
type SerializedFile struct {
	Path string
	Data []byte
}
