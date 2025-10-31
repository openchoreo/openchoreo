// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package backend

import "time"

type Resource struct {
	ID                int64     `json:"id"`
	ResourceType      string    `json:"resource_type"`
	ResourceNamespace string    `json:"resource_namespace"`
	ResourceName      string    `json:"resource_name"`
	ResourceUID       string    `json:"resource_uid"`
	ResourceVersion   string    `json:"resource_version"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type ResourceLabel struct {
	ID         int64  `json:"id"`
	ResourceID int64  `json:"resource_id"`
	LabelKey   string `json:"label_key"`
	LabelValue string `json:"label_value"`
}

type PostureScannedResource struct {
	ID              int64     `json:"id"`
	ResourceID      int64     `json:"resource_id"`
	ResourceVersion string    `json:"resource_version"`
	ScanDurationMs  *int64    `json:"scan_duration_ms"`
	ScannedAt       time.Time `json:"scanned_at"`
}

type PostureFinding struct {
	ID              int64     `json:"id"`
	ResourceID      int64     `json:"resource_id"`
	CheckID         string    `json:"check_id"`
	CheckName       string    `json:"check_name"`
	Severity        string    `json:"severity"`
	Category        *string   `json:"category"`
	Description     *string   `json:"description"`
	Remediation     *string   `json:"remediation"`
	ResourceVersion string    `json:"resource_version"`
	CreatedAt       time.Time `json:"created_at"`
}

type PostureFindingWithResource struct {
	PostureFinding
	ResourceType      string `json:"resource_type"`
	ResourceNamespace string `json:"resource_namespace"`
	ResourceName      string `json:"resource_name"`
}

type FindingsSummary struct {
	Severity string `json:"severity"`
	Count    int64  `json:"count"`
}

type FindingsSummaryByCategory struct {
	Category *string `json:"category"`
	Count    int64   `json:"count"`
}
