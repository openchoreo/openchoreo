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

// List retrieves all actions from the database
func (r *ActionRepository) List() ([]Action, error) {
	var actions []Action
	result := r.db.Order("action").Find(&actions)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list actions: %w", result.Error)
	}

	return actions, nil
}
