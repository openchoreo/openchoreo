// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	authzmocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
)

// --- BuildListOptions ---

func mustParseSelector(t *testing.T, s string) labels.Selector {
	t.Helper()
	sel, err := labels.Parse(s)
	require.NoError(t, err)
	return sel
}

func TestBuildListOptions(t *testing.T) {
	tests := []struct {
		name    string
		input   ListOptions
		want    []client.ListOption
		wantErr bool
	}{
		{
			name:  "empty options",
			input: ListOptions{},
			want:  nil,
		},
		{
			name:  "limit only",
			input: ListOptions{Limit: 25},
			want:  []client.ListOption{client.Limit(25)},
		},
		{
			name:  "zero limit excluded",
			input: ListOptions{Limit: 0},
			want:  nil,
		},
		{
			name:  "cursor only",
			input: ListOptions{Cursor: "abc"},
			want:  []client.ListOption{client.Continue("abc")},
		},
		{
			name:  "valid label selector",
			input: ListOptions{LabelSelector: "app=web"},
			want:  []client.ListOption{client.MatchingLabelsSelector{Selector: mustParseSelector(t, "app=web")}},
		},
		{
			name:    "invalid label selector",
			input:   ListOptions{LabelSelector: "===invalid"},
			wantErr: true,
		},
		{
			name:  "all options combined",
			input: ListOptions{Limit: 10, Cursor: "tok", LabelSelector: "tier=prod"},
			want: []client.ListOption{
				client.Limit(10),
				client.Continue("tok"),
				client.MatchingLabelsSelector{Selector: mustParseSelector(t, "tier=prod")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildListOptions(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				var validationErr *ValidationError
				assert.ErrorAs(t, err, &validationErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

type listPage[T any] struct {
	items      []T
	nextCursor string
	err        error
}

func pagedListResource[T any](t *testing.T, pages map[string]listPage[T], calls *[]ListOptions) ListResource[T] {
	t.Helper()
	return func(_ context.Context, opts ListOptions) (*ListResult[T], error) {
		*calls = append(*calls, opts)

		page, ok := pages[opts.Cursor]
		if !ok {
			t.Fatalf("unexpected cursor %q", opts.Cursor)
		}
		if page.err != nil {
			return nil, page.err
		}

		return &ListResult[T]{
			Items:      append([]T(nil), page.items...),
			NextCursor: page.nextCursor,
		}, nil
	}
}

func intCheckRequest(item int) CheckRequest {
	return CheckRequest{
		Action:       "number:view",
		ResourceType: "number",
		ResourceID:   strconv.Itoa(item),
	}
}

func alwaysAllowDecision() *authzcore.Decision {
	return &authzcore.Decision{
		Decision: true,
		Context: &authzcore.DecisionContext{
			Reason: "allowed",
		},
	}
}

func denyDecision() *authzcore.Decision {
	return &authzcore.Decision{
		Decision: false,
		Context: &authzcore.DecisionContext{
			Reason: "denied",
		},
	}
}

func newPaginationChecker(
	t *testing.T,
	evaluate func(ctx context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error),
) *AuthzChecker {
	t.Helper()
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).RunAndReturn(evaluate)
	return NewAuthzChecker(pdp, slog.Default())
}

func TestEncodeCursor_DecodeCursor_RoundTrip(t *testing.T) {
	in := paginationCursor{
		Continue: "next-token",
		Skip:     7,
	}

	encoded := encodeCursor(in)
	out, err := decodeCursor(encoded)

	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func TestDecodeCursor_EmptyString(t *testing.T) {
	out, err := decodeCursor("")

	require.NoError(t, err)
	assert.Equal(t, paginationCursor{}, out)
}

func TestDecodeCursor_InvalidBase64(t *testing.T) {
	_, err := decodeCursor("%%%")

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid cursor")
}

func TestDecodeCursor_InvalidJSON(t *testing.T) {
	invalidJSONCursor := base64.RawURLEncoding.EncodeToString([]byte("{invalid-json"))

	_, err := decodeCursor(invalidJSONCursor)

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid cursor")
}

func TestPreFilteredList_NoFilters(t *testing.T) {
	listResource := func(_ context.Context, _ ListOptions) (*ListResult[int], error) {
		return &ListResult[int]{Items: []int{1, 2, 3}, NextCursor: "next"}, nil
	}

	filtered := PreFilteredList(listResource)
	opts := ListOptions{Limit: 10, Cursor: "cursor"}

	got, err := filtered(context.Background(), opts)
	require.NoError(t, err)

	want, err := listResource(context.Background(), opts)
	require.NoError(t, err)

	assert.Equal(t, want.Items, got.Items, "items should match original list resource")
	assert.Equal(t, want.NextCursor, got.NextCursor, "next cursor should match original list resource")
}

func TestPreFilteredList_SingleFilter(t *testing.T) {
	var calls []ListOptions
	listResource := pagedListResource(t, map[string]listPage[int]{
		"": {items: []int{1, 2, 3, 4}},
	}, &calls)

	filtered := PreFilteredList(listResource, func(item int) bool {
		return item%2 == 0
	})

	result, err := filtered(context.Background(), ListOptions{Limit: 10})

	require.NoError(t, err)
	assert.Equal(t, []int{2, 4}, result.Items)
	assert.Empty(t, result.NextCursor)
	assert.Equal(t, []ListOptions{{Limit: 10, Cursor: ""}}, calls)
}

func TestPreFilteredList_MultipleFilters(t *testing.T) {
	var calls []ListOptions
	listResource := pagedListResource(t, map[string]listPage[int]{
		"": {items: []int{1, 2, 3, 4, 5, 6}},
	}, &calls)

	filtered := PreFilteredList(
		listResource,
		func(item int) bool { return item%2 == 0 },
		func(item int) bool { return item > 3 },
	)

	result, err := filtered(context.Background(), ListOptions{Limit: 10})

	require.NoError(t, err)
	assert.Equal(t, []int{4, 6}, result.Items)
	assert.Empty(t, result.NextCursor)
	assert.Equal(t, []ListOptions{{Limit: 10, Cursor: ""}}, calls)
}

func TestPreFilteredList_PaginationAcrossFilteredPages(t *testing.T) {
	var calls []ListOptions
	listResource := pagedListResource(t, map[string]listPage[int]{
		"":   {items: []int{1, 2, 3}, nextCursor: "p2"},
		"p2": {items: []int{4, 5, 6}},
	}, &calls)

	filtered := PreFilteredList(listResource, func(item int) bool {
		return item%2 == 0
	})

	firstPage, err := filtered(context.Background(), ListOptions{Limit: 2})
	require.NoError(t, err)
	require.Len(t, firstPage.Items, 2)
	assert.Equal(t, []int{2, 4}, firstPage.Items)
	require.NotEmpty(t, firstPage.NextCursor)

	nextInternalCursor, err := decodeInternalCursor(firstPage.NextCursor)
	require.NoError(t, err)
	assert.Equal(t, internalCursor{Continue: "p2", InternalSkip: 1}, nextInternalCursor)

	secondPage, err := filtered(context.Background(), ListOptions{
		Limit:  2,
		Cursor: firstPage.NextCursor,
	})
	require.NoError(t, err)
	assert.Equal(t, []int{6}, secondPage.Items)
	assert.Empty(t, secondPage.NextCursor)

	assert.Equal(t, []ListOptions{
		{Limit: 2, Cursor: ""},
		{Limit: 2, Cursor: "p2"},
		{Limit: 2, Cursor: "p2"},
	}, calls)
}

func TestPreFilteredList_EmptyResults(t *testing.T) {
	listResource := func(_ context.Context, _ ListOptions) (*ListResult[int], error) {
		return &ListResult[int]{Items: []int{1, 3, 5}}, nil
	}

	filtered := PreFilteredList(listResource, func(item int) bool {
		return item%2 == 0
	})

	result, err := filtered(context.Background(), ListOptions{Limit: 5})

	require.NoError(t, err)
	assert.Empty(t, result.Items)
	assert.Empty(t, result.NextCursor)
}

func TestPreFilteredList_ErrorPropagation(t *testing.T) {
	upstreamErr := errors.New("list failed")
	listResource := func(_ context.Context, _ ListOptions) (*ListResult[int], error) {
		return nil, upstreamErr
	}

	filtered := PreFilteredList(listResource, func(_ int) bool { return true })

	_, err := filtered(context.Background(), ListOptions{Limit: 1})

	require.Error(t, err)
	assert.ErrorIs(t, err, upstreamErr)
}

func TestFilteredList_AllAllowed(t *testing.T) {
	var calls []ListOptions
	listResource := pagedListResource(t, map[string]listPage[int]{
		"": {items: []int{1, 2, 3}},
	}, &calls)

	checker := newPaginationChecker(t, func(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
		return alwaysAllowDecision(), nil
	})

	result, err := FilteredList(
		context.Background(),
		ListOptions{Limit: 10},
		checker,
		listResource,
		intCheckRequest,
	)

	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, result.Items)
	assert.Empty(t, result.NextCursor)
	assert.Equal(t, []ListOptions{{Limit: 10, Cursor: ""}}, calls)
}

func TestFilteredList_SomeDenied(t *testing.T) {
	listResource := func(_ context.Context, _ ListOptions) (*ListResult[int], error) {
		return &ListResult[int]{Items: []int{1, 2, 3, 4}}, nil
	}

	checker := newPaginationChecker(t, func(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
		num, err := strconv.Atoi(req.Resource.ID)
		require.NoError(t, err)
		if num%2 == 0 {
			return alwaysAllowDecision(), nil
		}
		return denyDecision(), nil
	})

	result, err := FilteredList(
		context.Background(),
		ListOptions{Limit: 10},
		checker,
		listResource,
		intCheckRequest,
	)

	require.NoError(t, err)
	assert.Equal(t, []int{2, 4}, result.Items)
	assert.Empty(t, result.NextCursor)
}

func TestFilteredList_PaginationWithPartialAuthz(t *testing.T) {
	var calls []ListOptions
	listResource := pagedListResource(t, map[string]listPage[int]{
		"":   {items: []int{1, 2, 3}, nextCursor: "p2"},
		"p2": {items: []int{4, 5, 6}},
	}, &calls)

	checker := newPaginationChecker(t, func(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
		num, err := strconv.Atoi(req.Resource.ID)
		require.NoError(t, err)
		if num%2 == 0 {
			return alwaysAllowDecision(), nil
		}
		return denyDecision(), nil
	})

	firstPage, err := FilteredList(
		context.Background(),
		ListOptions{Limit: 2},
		checker,
		listResource,
		intCheckRequest,
	)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 4}, firstPage.Items)
	require.NotEmpty(t, firstPage.NextCursor)

	nextCursor, err := decodeCursor(firstPage.NextCursor)
	require.NoError(t, err)
	assert.Equal(t, paginationCursor{Continue: "p2", Skip: 1}, nextCursor)

	secondPage, err := FilteredList(
		context.Background(),
		ListOptions{
			Limit:  2,
			Cursor: firstPage.NextCursor,
		},
		checker,
		listResource,
		intCheckRequest,
	)
	require.NoError(t, err)
	assert.Equal(t, []int{6}, secondPage.Items)
	assert.Empty(t, secondPage.NextCursor)

	assert.Equal(t, []ListOptions{
		{Limit: 2, Cursor: ""},
		{Limit: 2, Cursor: "p2"},
		{Limit: 2, Cursor: "p2"},
	}, calls)
}

func TestFilteredList_RemainingCountCleared(t *testing.T) {
	remaining := int64(3)

	listResource := func(_ context.Context, _ ListOptions) (*ListResult[int], error) {
		return &ListResult[int]{
			Items:          []int{1, 2, 3, 4},
			RemainingCount: &remaining,
		}, nil
	}

	checker := newPaginationChecker(t, func(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
		num, err := strconv.Atoi(req.Resource.ID)
		require.NoError(t, err)
		if num%2 == 0 {
			return alwaysAllowDecision(), nil
		}
		return denyDecision(), nil
	})

	result, err := FilteredList(
		context.Background(),
		ListOptions{Limit: 2},
		checker,
		listResource,
		intCheckRequest,
	)

	require.NoError(t, err)
	assert.Equal(t, []int{2, 4}, result.Items)
	assert.Nil(t, result.RemainingCount, "authz-filtered pages must not expose upstream RemainingCount")
}

func TestFilteredList_AuthzErrorPropagation(t *testing.T) {
	authzErr := errors.New("pdp unavailable")
	listResource := func(_ context.Context, _ ListOptions) (*ListResult[int], error) {
		return &ListResult[int]{Items: []int{1, 2}}, nil
	}

	checker := newPaginationChecker(t, func(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
		if req.Resource.ID == "2" {
			return nil, authzErr
		}
		return alwaysAllowDecision(), nil
	})

	_, err := FilteredList(
		context.Background(),
		ListOptions{Limit: 2},
		checker,
		listResource,
		intCheckRequest,
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, authzErr)
}

func TestFilteredList_EmptyCursor(t *testing.T) {
	var gotCursor string
	listResource := func(_ context.Context, opts ListOptions) (*ListResult[int], error) {
		gotCursor = opts.Cursor
		return &ListResult[int]{Items: []int{42}}, nil
	}

	checker := newPaginationChecker(t, func(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
		return alwaysAllowDecision(), nil
	})

	result, err := FilteredList(
		context.Background(),
		ListOptions{Limit: 1, Cursor: ""},
		checker,
		listResource,
		intCheckRequest,
	)

	require.NoError(t, err)
	assert.Equal(t, "", gotCursor)
	assert.Equal(t, []int{42}, result.Items)
}
