// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"fmt"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// Trait wraps v1alpha1.Trait with domain-specific helper methods
type Trait struct {
	*v1alpha1.Trait
}

// NewTrait creates a Trait wrapper from a ResourceEntry
func NewTrait(entry *index.ResourceEntry) (*Trait, error) {
	trait, err := FromEntry[v1alpha1.Trait](entry)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to Trait: %w", err)
	}
	return &Trait{Trait: trait}, nil
}
