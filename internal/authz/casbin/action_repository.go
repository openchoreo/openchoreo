// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"fmt"

	"gorm.io/gorm"
)

// ActionRepository handles CRUD operations for actions
type ActionRepository struct {
	db *gorm.DB
}

// NewActionRepository creates a new action repository
func NewActionRepository(db *gorm.DB) *ActionRepository {
	return &ActionRepository{db: db}
}

// ListPublicActions retrieves all public actions from the database
func (r *ActionRepository) ListPublicActions() ([]Action, error) {
	var actions []Action
	result := r.db.
		Where("internal = ?", false).
		Order("action").
		Find(&actions)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list actions: %w", result.Error)
	}

	return actions, nil
}

// ListConcretePublicActions retrieves only concrete (non-wildcarded) public actions from the database.
func (r *ActionRepository) ListConcretePublicActions() ([]Action, error) {
	var actions []Action
	// exclude wildcarded actions (containing *) and internal actions
	result := r.db.
		Where("action NOT LIKE '%*%'").
		Where("internal = ?", false).
		Order("action").
		Find(&actions)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to list concrete actions: %w", result.Error)
	}

	return actions, nil
}
