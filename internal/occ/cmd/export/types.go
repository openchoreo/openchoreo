// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

// Resource associates an API object with its export identity. Namespace is empty
// for cluster-scoped objects, including Namespace objects. Fetch supplies a
// generated API value in Object; Filter replaces it with an independent JSON
// document containing the approved fields for export.
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
