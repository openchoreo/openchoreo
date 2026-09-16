// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	metadatasvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/metadata"
)

type fakeMetadataService struct {
	md  *metadatasvc.Metadata
	err error
}

func (f *fakeMetadataService) GetMetadata(context.Context) (*metadatasvc.Metadata, error) {
	return f.md, f.err
}

func newMetadataHandler(svc metadatasvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{MetadataService: svc},
		logger:   slog.Default(),
	}
}

func TestGetMetadata(t *testing.T) {
	tests := []struct {
		name     string
		md       *metadatasvc.Metadata
		wantJSON string
	}{
		{
			name:     "audit logs disabled omits the observer URL",
			md:       &metadatasvc.Metadata{},
			wantJSON: `{"features":{"auditLogs":{"enabled":false}}}`,
		},
		{
			name:     "audit logs enabled without an observer URL omits it",
			md:       &metadatasvc.Metadata{AuditLogs: metadatasvc.AuditLogs{Enabled: true}},
			wantJSON: `{"features":{"auditLogs":{"enabled":true}}}`,
		},
		{
			name: "audit logs available advertises the observer URL",
			md: &metadatasvc.Metadata{AuditLogs: metadatasvc.AuditLogs{
				Enabled:     true,
				ObserverURL: "https://observer.example.com",
			}},
			wantJSON: `{"features":{"auditLogs":{"enabled":true,"observerURL":"https://observer.example.com"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newMetadataHandler(&fakeMetadataService{md: tt.md})

			resp, err := h.GetMetadata(context.Background(), gen.GetMetadataRequestObject{})
			require.NoError(t, err)
			typed, ok := resp.(gen.GetMetadata200JSONResponse)
			require.True(t, ok, "expected 200 JSON response, got %T", resp)

			body, err := json.Marshal(typed)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(body))
		})
	}

	t.Run("service error returns 500", func(t *testing.T) {
		h := newMetadataHandler(&fakeMetadataService{err: errors.New("boom")})

		resp, err := h.GetMetadata(context.Background(), gen.GetMetadataRequestObject{})
		require.NoError(t, err)
		_, ok := resp.(gen.GetMetadata500JSONResponse)
		assert.True(t, ok, "expected 500 JSON response, got %T", resp)
	})
}
