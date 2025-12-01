// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"errors"
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

// Create adds a new action to the database
func (r *ActionRepository) Create(action string) error {
	if action == "" {
		return fmt.Errorf("action cannot be empty")
	}

	result := r.db.Create(&Action{Action: action})
	if result.Error != nil {
		return fmt.Errorf("failed to create action: %w", result.Error)
	}

	return nil
}

// GetByAction retrieves an action by its action string
func (r *ActionRepository) GetByAction(actionStr string) (*Action, error) {
	var action Action
	result := r.db.Where("action = ?", actionStr).First(&action)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("action not found: %s", actionStr)
		}
		return nil, fmt.Errorf("failed to get action: %w", result.Error)
	}

	return &action, nil
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

// DeleteByAction removes an action by its action string
func (r *ActionRepository) DeleteByAction(actionStr string) error {
	result := r.db.Where("action = ?", actionStr).Delete(&Action{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete action: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("action not found: %s", actionStr)
	}

	return nil
}
