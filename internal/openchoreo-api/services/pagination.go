// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// ListOptions controls pagination for list operations.
type ListOptions struct {
	// Limit is the maximum number of items to return per page.
	Limit int
	// Cursor is an opaque pagination cursor from a previous response.
	Cursor string
}

// ListResult holds a page of items along with pagination metadata.
type ListResult[T any] struct {
	// Items is the list of resources in this page.
	Items []T
	// NextCursor is an opaque cursor for fetching the next page.
	// Empty when there are no more items.
	NextCursor string
	// RemainingCount is an approximate count of items remaining after this page.
	// Nil when the count is unknown (e.g. authz-filtered queries).
	RemainingCount *int64
}

// paginationCursor is the internal structure encoded as the opaque cursor.
type paginationCursor struct {
	// Continue is the K8s continuation token for the current page.
	Continue string `json:"c,omitempty"`
	// Skip is the number of items to skip within the K8s page identified by Continue.
	Skip int `json:"s,omitempty"`
}

// EncodeCursor encodes a pagination cursor to a base64 string.
func encodeCursor(c paginationCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor decodes a base64 string into a pagination cursor.
func decodeCursor(s string) (paginationCursor, error) {
	var c paginationCursor
	if s == "" {
		return c, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return c, fmt.Errorf("invalid cursor: %w", err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("invalid cursor: %w", err)
	}
	return c, nil
}

// ListResource fetches a single page of items from the underlying data source.
type ListResource[T any] func(ctx context.Context, opts ListOptions) (*ListResult[T], error)

// ItemFilter is a predicate that returns true if the item should be included.
type ItemFilter[T any] func(item T) bool

// PreFilteredList wraps a ListResource with in-memory predicate filters.
// It returns a new ListResource that over-fetches and filters items while maintaining
// correct pagination using an internal cursor. This can be composed with FilteredList
// for layered filtering (e.g., project filter + authz filter).
// When no filters are provided, the original listResource is returned unchanged.
func PreFilteredList[T any](
	listResource ListResource[T],
	filters ...ItemFilter[T],
) ListResource[T] {
	if len(filters) == 0 {
		return listResource
	}

	return func(ctx context.Context, opts ListOptions) (*ListResult[T], error) {
		cur, err := decodeInternalCursor(opts.Cursor)
		if err != nil {
			return nil, err
		}

		filtered := make([]T, 0, opts.Limit)
		k8sContinue := cur.Continue
		skip := cur.InternalSkip

		for len(filtered) < opts.Limit {
			page, err := listResource(ctx, ListOptions{
				Limit:  opts.Limit,
				Cursor: k8sContinue,
			})
			if err != nil {
				return nil, err
			}

			items := page.Items

			// Skip items already processed in the previous call.
			if skip > 0 {
				if skip >= len(items) {
					skip -= len(items)
					if page.NextCursor == "" {
						break
					}
					k8sContinue = page.NextCursor
					continue
				}
				items = items[skip:]
				skip = 0
			}

			for i, item := range items {
				if !matchesAll(item, filters) {
					continue
				}

				filtered = append(filtered, item)

				if len(filtered) == opts.Limit {
					// Page is full. Build cursor pointing to the next unprocessed item.
					consumed := (len(page.Items) - len(items)) + i + 1
					nextCur := internalCursor{
						Continue:     k8sContinue,
						InternalSkip: consumed,
					}
					if consumed >= len(page.Items) {
						nextCur.Continue = page.NextCursor
						nextCur.InternalSkip = 0
						if page.NextCursor == "" {
							return &ListResult[T]{Items: filtered}, nil
						}
					}
					return &ListResult[T]{
						Items:      filtered,
						NextCursor: encodeInternalCursor(nextCur),
					}, nil
				}
			}

			if page.NextCursor == "" {
				break
			}
			k8sContinue = page.NextCursor
		}

		return &ListResult[T]{Items: filtered}, nil
	}
}

func matchesAll[T any](item T, filters []ItemFilter[T]) bool {
	for _, f := range filters {
		if !f(item) {
			return false
		}
	}
	return true
}

// internalCursor tracks position within pages when in-memory filtering is applied.
type internalCursor struct {
	// Continue is the upstream continuation token for the current page.
	Continue string `json:"c,omitempty"`
	// InternalSkip is the number of items to skip within the page (already processed).
	InternalSkip int `json:"is,omitempty"`
}

func encodeInternalCursor(c internalCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeInternalCursor(s string) (internalCursor, error) {
	var c internalCursor
	if s == "" {
		return c, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return c, fmt.Errorf("invalid cursor: %w", err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("invalid cursor: %w", err)
	}
	return c, nil
}

// GenerateAuthzCheckRequest builds a CheckRequest for an individual item.
type GenerateAuthzCheckRequest[T any] func(item T) CheckRequest

// FilteredList implements the generic authz-filtered over-fetch loop.
// It fetches pages from the data source, filters items through the authz checker,
// and accumulates results until the requested limit is reached or all items are exhausted.
func FilteredList[T any](
	ctx context.Context,
	opts ListOptions,
	authzChecker *AuthzChecker,
	listResource ListResource[T],
	generateAuthzCheckRequest GenerateAuthzCheckRequest[T],
) (*ListResult[T], error) {
	cur, err := decodeCursor(opts.Cursor)
	if err != nil {
		return nil, err
	}

	// Fetch exactly the requested limit per K8s page since most items are
	// expected to pass authz filtering.
	batchSize := opts.Limit

	authorized := make([]T, 0, opts.Limit)
	k8sContinue := cur.Continue
	skip := cur.Skip

	for len(authorized) < opts.Limit {
		page, err := listResource(ctx, ListOptions{
			Limit:  batchSize,
			Cursor: k8sContinue,
		})
		if err != nil {
			return nil, err
		}

		items := page.Items

		// Skip items that were already returned in the previous client page.
		if skip > 0 {
			if skip >= len(items) {
				// All items on this K8s page were already consumed.
				skip -= len(items)
				if page.NextCursor == "" {
					break
				}
				k8sContinue = page.NextCursor
				continue
			}
			items = items[skip:]
			skip = 0
		}

		for i, item := range items {
			if err := authzChecker.Check(ctx, generateAuthzCheckRequest(item)); err != nil {
				if errors.Is(err, ErrForbidden) {
					continue
				}
				return nil, err
			}

			authorized = append(authorized, item)

			if len(authorized) == opts.Limit {
				// Page is full. Build cursor pointing to the next unprocessed item.
				// i is relative to items (after skip adjustment), so compute the
				// absolute index in the original page.Items slice.
				consumed := (len(page.Items) - len(items)) + i + 1
				nextCur := paginationCursor{
					Continue: k8sContinue,
					Skip:     consumed,
				}
				// If we consumed exactly all items on this K8s page, advance to next.
				if consumed >= len(page.Items) {
					nextCur.Continue = page.NextCursor
					nextCur.Skip = 0
					if page.NextCursor == "" {
						// No more K8s pages — we just happened to fill exactly.
						return &ListResult[T]{Items: authorized}, nil
					}
				}
				return &ListResult[T]{
					Items:      authorized,
					NextCursor: encodeCursor(nextCur),
				}, nil
			}
		}

		// Exhausted this K8s page without filling the client page.
		if page.NextCursor == "" {
			break
		}
		k8sContinue = page.NextCursor
	}

	// K8s exhausted or no more items — return what we have without a next cursor.
	return &ListResult[T]{Items: authorized}, nil
}
