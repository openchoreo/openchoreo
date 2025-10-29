// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"context"
	"database/sql"
	"time"

	"github.com/openchoreo/openchoreo/internal/security-scanner/db/backend/postgres"
	"github.com/openchoreo/openchoreo/internal/security-scanner/db/backend/sqlite"
)

type Querier interface {
	UpsertResource(ctx context.Context, resourceType, resourceNamespace, resourceName, resourceUID, resourceVersion string) (int64, error)

	InsertResourceLabel(ctx context.Context, resourceID int64, labelKey, labelValue string) error
	DeleteResourceLabels(ctx context.Context, resourceID int64) error

	GetPostureScannedResource(ctx context.Context, resourceType, resourceNamespace, resourceName string) (PostureScannedResource, error)
	UpsertPostureScannedResource(ctx context.Context, resourceID int64, resourceVersion string, scanDurationMs *int64) error

	InsertPostureFinding(ctx context.Context, resourceID int64, checkID, checkName, severity string, category, description, remediation *string, resourceVersion string) error
	DeletePostureFindingsByResourceID(ctx context.Context, resourceID int64) error
	ListPostureFindings(ctx context.Context, limit, offset int64) ([]PostureFindingWithResource, error)
}

type sqliteAdapter struct {
	q *sqlite.Queries
}

func NewSQLiteAdapter(q *sqlite.Queries) Querier {
	return &sqliteAdapter{q: q}
}

func (a *sqliteAdapter) UpsertResource(ctx context.Context, resourceType, resourceNamespace, resourceName, resourceUID, resourceVersion string) (int64, error) {
	return a.q.UpsertResource(ctx, sqlite.UpsertResourceParams{
		ResourceType:      resourceType,
		ResourceNamespace: resourceNamespace,
		ResourceName:      resourceName,
		ResourceUid:       resourceUID,
		ResourceVersion:   resourceVersion,
	})
}

func (a *sqliteAdapter) InsertResourceLabel(ctx context.Context, resourceID int64, labelKey, labelValue string) error {
	return a.q.InsertResourceLabel(ctx, sqlite.InsertResourceLabelParams{
		ResourceID: resourceID,
		LabelKey:   labelKey,
		LabelValue: labelValue,
	})
}

func (a *sqliteAdapter) DeleteResourceLabels(ctx context.Context, resourceID int64) error {
	return a.q.DeleteResourceLabels(ctx, resourceID)
}

func (a *sqliteAdapter) GetPostureScannedResource(ctx context.Context, resourceType, resourceNamespace, resourceName string) (PostureScannedResource, error) {
	r, err := a.q.GetPostureScannedResource(ctx, sqlite.GetPostureScannedResourceParams{
		ResourceType:      resourceType,
		ResourceNamespace: resourceNamespace,
		ResourceName:      resourceName,
	})
	if err != nil {
		return PostureScannedResource{}, err
	}
	return convertSQLitePostureScannedResource(r), nil
}

func (a *sqliteAdapter) UpsertPostureScannedResource(ctx context.Context, resourceID int64, resourceVersion string, scanDurationMs *int64) error {
	var scanDuration sql.NullInt64
	if scanDurationMs != nil {
		scanDuration = sql.NullInt64{Int64: *scanDurationMs, Valid: true}
	}
	return a.q.UpsertPostureScannedResource(ctx, sqlite.UpsertPostureScannedResourceParams{
		ResourceID:      resourceID,
		ResourceVersion: resourceVersion,
		ScanDurationMs:  scanDuration,
	})
}

func (a *sqliteAdapter) InsertPostureFinding(ctx context.Context, resourceID int64, checkID, checkName, severity string, category, description, remediation *string, resourceVersion string) error {
	return a.q.InsertPostureFinding(ctx, sqlite.InsertPostureFindingParams{
		ResourceID:      resourceID,
		CheckID:         checkID,
		CheckName:       checkName,
		Severity:        severity,
		Category:        toNullString(category),
		Description:     toNullString(description),
		Remediation:     toNullString(remediation),
		ResourceVersion: resourceVersion,
	})
}

func (a *sqliteAdapter) DeletePostureFindingsByResourceID(ctx context.Context, resourceID int64) error {
	return a.q.DeletePostureFindingsByResourceID(ctx, resourceID)
}

func (a *sqliteAdapter) ListPostureFindings(ctx context.Context, limit, offset int64) ([]PostureFindingWithResource, error) {
	rows, err := a.q.ListPostureFindings(ctx, sqlite.ListPostureFindingsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	result := make([]PostureFindingWithResource, len(rows))
	for i, r := range rows {
		result[i] = convertSQLitePostureFindingRow(r)
	}
	return result, nil
}

type postgresAdapter struct {
	q *postgres.Queries
}

func NewPostgresAdapter(q *postgres.Queries) Querier {
	return &postgresAdapter{q: q}
}

func (a *postgresAdapter) UpsertResource(ctx context.Context, resourceType, resourceNamespace, resourceName, resourceUID, resourceVersion string) (int64, error) {
	id, err := a.q.UpsertResource(ctx, postgres.UpsertResourceParams{
		ResourceType:      resourceType,
		ResourceNamespace: resourceNamespace,
		ResourceName:      resourceName,
		ResourceUid:       resourceUID,
		ResourceVersion:   resourceVersion,
	})
	return int64(id), err
}

func (a *postgresAdapter) InsertResourceLabel(ctx context.Context, resourceID int64, labelKey, labelValue string) error {
	return a.q.InsertResourceLabel(ctx, postgres.InsertResourceLabelParams{
		ResourceID: int32(resourceID),
		LabelKey:   labelKey,
		LabelValue: labelValue,
	})
}

func (a *postgresAdapter) DeleteResourceLabels(ctx context.Context, resourceID int64) error {
	return a.q.DeleteResourceLabels(ctx, int32(resourceID))
}

func (a *postgresAdapter) GetPostureScannedResource(ctx context.Context, resourceType, resourceNamespace, resourceName string) (PostureScannedResource, error) {
	r, err := a.q.GetPostureScannedResource(ctx, postgres.GetPostureScannedResourceParams{
		ResourceType:      resourceType,
		ResourceNamespace: resourceNamespace,
		ResourceName:      resourceName,
	})
	if err != nil {
		return PostureScannedResource{}, err
	}
	return convertPostgresPostureScannedResource(r), nil
}

func (a *postgresAdapter) UpsertPostureScannedResource(ctx context.Context, resourceID int64, resourceVersion string, scanDurationMs *int64) error {
	var scanDuration sql.NullInt32
	if scanDurationMs != nil {
		scanDuration = sql.NullInt32{Int32: int32(*scanDurationMs), Valid: true}
	}
	return a.q.UpsertPostureScannedResource(ctx, postgres.UpsertPostureScannedResourceParams{
		ResourceID:      int32(resourceID),
		ResourceVersion: resourceVersion,
		ScanDurationMs:  scanDuration,
	})
}

func (a *postgresAdapter) InsertPostureFinding(ctx context.Context, resourceID int64, checkID, checkName, severity string, category, description, remediation *string, resourceVersion string) error {
	return a.q.InsertPostureFinding(ctx, postgres.InsertPostureFindingParams{
		ResourceID:      int32(resourceID),
		CheckID:         checkID,
		CheckName:       checkName,
		Severity:        severity,
		Category:        toNullString(category),
		Description:     toNullString(description),
		Remediation:     toNullString(remediation),
		ResourceVersion: resourceVersion,
	})
}

func (a *postgresAdapter) DeletePostureFindingsByResourceID(ctx context.Context, resourceID int64) error {
	return a.q.DeletePostureFindingsByResourceID(ctx, int32(resourceID))
}

func (a *postgresAdapter) ListPostureFindings(ctx context.Context, limit, offset int64) ([]PostureFindingWithResource, error) {
	rows, err := a.q.ListPostureFindings(ctx, postgres.ListPostureFindingsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	result := make([]PostureFindingWithResource, len(rows))
	for i, r := range rows {
		result[i] = convertPostgresPostureFindingRow(r)
	}
	return result, nil
}

func convertSQLitePostureScannedResource(r sqlite.PostureScannedResource) PostureScannedResource {
	scannedAt := r.ScannedAt.Time
	if !r.ScannedAt.Valid {
		scannedAt = time.Time{}
	}
	return PostureScannedResource{
		ID:              r.ID,
		ResourceID:      r.ResourceID,
		ResourceVersion: r.ResourceVersion,
		ScanDurationMs:  fromNullInt64(r.ScanDurationMs),
		ScannedAt:       scannedAt,
	}
}

func convertPostgresPostureScannedResource(r postgres.PostureScannedResource) PostureScannedResource {
	scannedAt := r.ScannedAt.Time
	if !r.ScannedAt.Valid {
		scannedAt = time.Time{}
	}
	return PostureScannedResource{
		ID:              int64(r.ID),
		ResourceID:      int64(r.ResourceID),
		ResourceVersion: r.ResourceVersion,
		ScanDurationMs:  fromNullInt32(r.ScanDurationMs),
		ScannedAt:       scannedAt,
	}
}

func convertSQLitePostureFindingRow(r sqlite.ListPostureFindingsRow) PostureFindingWithResource {
	createdAt := r.CreatedAt.Time
	if !r.CreatedAt.Valid {
		createdAt = time.Time{}
	}
	return PostureFindingWithResource{
		PostureFinding: PostureFinding{
			ID:              r.ID,
			ResourceID:      r.ResourceID,
			CheckID:         r.CheckID,
			CheckName:       r.CheckName,
			Severity:        r.Severity,
			Category:        fromNullString(r.Category),
			Description:     fromNullString(r.Description),
			Remediation:     fromNullString(r.Remediation),
			ResourceVersion: r.ResourceVersion,
			CreatedAt:       createdAt,
		},
		ResourceType:      r.ResourceType,
		ResourceNamespace: r.ResourceNamespace,
		ResourceName:      r.ResourceName,
	}
}

func convertPostgresPostureFindingRow(r postgres.ListPostureFindingsRow) PostureFindingWithResource {
	createdAt := r.CreatedAt.Time
	if !r.CreatedAt.Valid {
		createdAt = time.Time{}
	}
	return PostureFindingWithResource{
		PostureFinding: PostureFinding{
			ID:              int64(r.ID),
			ResourceID:      int64(r.ResourceID),
			CheckID:         r.CheckID,
			CheckName:       r.CheckName,
			Severity:        r.Severity,
			Category:        fromNullString(r.Category),
			Description:     fromNullString(r.Description),
			Remediation:     fromNullString(r.Remediation),
			ResourceVersion: r.ResourceVersion,
			CreatedAt:       createdAt,
		},
		ResourceType:      r.ResourceType,
		ResourceNamespace: r.ResourceNamespace,
		ResourceName:      r.ResourceName,
	}
}

func toNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: *s, Valid: true}
}

func fromNullString(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}

func fromNullInt64(i sql.NullInt64) *int64 {
	if !i.Valid {
		return nil
	}
	return &i.Int64
}

func fromNullInt32(i sql.NullInt32) *int64 {
	if !i.Valid {
		return nil
	}
	result := int64(i.Int32)
	return &result
}
