// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package crd embeds the generated CRD YAML files so other packages
// can access them without filesystem reads at runtime.
package crd

import "embed"

//go:embed bases/openchoreo.dev_componenttypes.yaml
//go:embed bases/openchoreo.dev_clustercomponenttypes.yaml
//go:embed bases/openchoreo.dev_traits.yaml
var FS embed.FS
