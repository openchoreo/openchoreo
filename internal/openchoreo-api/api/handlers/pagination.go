// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

// NormalizeListOptions clamps the limit to [1, maxPageLimit] and extracts the cursor and label selector.
func NormalizeListOptions(limit *gen.LimitParam, cursor *gen.CursorParam, labelSelector *gen.LabelSelectorParam) services.ListOptions {
	l := defaultPageLimit
	if limit != nil {
		l = *limit
	}
	if l < 1 {
		l = 1
	} else if l > maxPageLimit {
		l = maxPageLimit
	}

	var c string
	if cursor != nil {
		c = *cursor
	}

	var ls string
	if labelSelector != nil {
		ls = *labelSelector
	}

	return services.ListOptions{
		Limit:         l,
		Cursor:        c,
		LabelSelector: ls,
	}
}

// ToPagination builds a gen.Pagination from a ListResult.
func ToPagination[T any](result *services.ListResult[T]) gen.Pagination {
	p := gen.Pagination{}
	if result.NextCursor != "" {
		p.NextCursor = &result.NextCursor
	}
	return p
}
