// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/security-scanner/db/backend"
)

type mockQuerier struct{}

func (m *mockQuerier) UpsertResource(ctx context.Context, resourceType, resourceNamespace, resourceName, resourceUID, resourceVersion string) (int64, error) {
	return 1, nil
}

func (m *mockQuerier) GetResource(ctx context.Context, resourceID int64) (backend.Resource, error) {
	return backend.Resource{
		ID:                resourceID,
		ResourceType:      "Deployment",
		ResourceNamespace: "default",
		ResourceName:      "test",
		ResourceUID:       "uid-123",
		ResourceVersion:   "100",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}, nil
}

func (m *mockQuerier) InsertResourceLabel(ctx context.Context, resourceID int64, labelKey, labelValue string) error {
	return nil
}

func (m *mockQuerier) DeleteResourceLabels(ctx context.Context, resourceID int64) error {
	return nil
}

func (m *mockQuerier) GetResourceLabels(ctx context.Context, resourceID int64) (map[string]string, error) {
	return map[string]string{"app": "test"}, nil
}

func (m *mockQuerier) GetPostureScannedResource(ctx context.Context, resourceType, resourceNamespace, resourceName string) (backend.PostureScannedResource, error) {
	return backend.PostureScannedResource{}, nil
}

func (m *mockQuerier) UpsertPostureScannedResource(ctx context.Context, resourceID int64, resourceVersion string, scanDurationMs *int64) error {
	return nil
}

func (m *mockQuerier) InsertPostureFinding(ctx context.Context, resourceID int64, checkID, checkName, severity string, category, description, remediation *string, resourceVersion string) error {
	return nil
}

func (m *mockQuerier) DeletePostureFindingsByResourceID(ctx context.Context, resourceID int64) error {
	return nil
}

func (m *mockQuerier) GetPostureFindingsByResourceID(ctx context.Context, resourceID int64) ([]backend.PostureFinding, error) {
	cat := "General"
	desc := "Test finding description"
	rem := "Test remediation"
	return []backend.PostureFinding{
		{
			ID:              1,
			ResourceID:      resourceID,
			CheckID:         "CKV_K8S_1",
			CheckName:       "Test Check",
			Severity:        "HIGH",
			Category:        &cat,
			Description:     &desc,
			Remediation:     &rem,
			ResourceVersion: "100",
			CreatedAt:       time.Now(),
		},
	}, nil
}

func (m *mockQuerier) ListPostureFindings(ctx context.Context, limit, offset int64) ([]backend.PostureFindingWithResource, error) {
	return []backend.PostureFindingWithResource{}, nil
}

func (m *mockQuerier) ListResourcesWithPostureFindings(ctx context.Context, limit, offset int64) ([]backend.Resource, error) {
	return []backend.Resource{
		{
			ID:                1,
			ResourceType:      "Deployment",
			ResourceNamespace: "default",
			ResourceName:      "test",
			ResourceUID:       "uid-123",
			ResourceVersion:   "100",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		},
	}, nil
}

func (m *mockQuerier) CountResourcesWithPostureFindings(ctx context.Context) (int64, error) {
	return 1, nil
}

func TestHealthHandler(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	handler := NewHandler(&mockQuerier{}, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	handler.healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %v", response.Status)
	}

	if response.Timestamp.IsZero() {
		t.Error("expected timestamp to be set")
	}

	if response.Version != "v1.0.0" {
		t.Errorf("expected version 'v1.0.0', got %v", response.Version)
	}
}

func TestRegisterRoutes(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	handler := NewHandler(&mockQuerier{}, logger)

	mux := http.NewServeMux()
	RegisterRoutes(mux, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 after registering routes, got %d", w.Code)
	}
}

func TestListPostureFindingsHandler(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	handler := NewHandler(&mockQuerier{}, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posture/findings?page=1&page_size=10", nil)
	w := httptest.NewRecorder()

	handler.listPostureFindingsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var response PostureFindingsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Pagination.Page != 1 {
		t.Errorf("expected page 1, got %d", response.Pagination.Page)
	}

	if response.Pagination.PageSize != 10 {
		t.Errorf("expected page_size 10, got %d", response.Pagination.PageSize)
	}

	if response.Pagination.TotalItems != 1 {
		t.Errorf("expected total_items 1, got %d", response.Pagination.TotalItems)
	}

	if len(response.Resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(response.Resources))
	}

	if len(response.Resources) > 0 {
		resource := response.Resources[0]
		if resource.Type != "Deployment" {
			t.Errorf("expected type Deployment, got %s", resource.Type)
		}
		if resource.Name != "test" {
			t.Errorf("expected name test, got %s", resource.Name)
		}
		if len(resource.Findings) != 1 {
			t.Errorf("expected 1 finding, got %d", len(resource.Findings))
		}
		if len(resource.Labels) != 1 {
			t.Errorf("expected 1 label, got %d", len(resource.Labels))
		}
	}
}
