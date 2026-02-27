// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package pagination

// defaultChunkSize is the number of items to request per page.
// The API server caps at 100 per page, so the CLI follows nextCursor
// to fetch all pages transparently.
const defaultChunkSize = 500

// FetchPage fetches one page given limit and cursor.
// Returns items, next cursor (empty string when done), and error.
type FetchPage[T any] func(limit int, cursor string) (items []T, nextCursor string, err error)

// FetchAll repeatedly calls fetchPage, accumulating all items until
// there are no more pages. This provides kubectl-style chunk-based
// pagination that is transparent to the user.
func FetchAll[T any](fetchPage FetchPage[T]) ([]T, error) {
	var all []T
	cursor := ""
	for {
		items, next, err := fetchPage(defaultChunkSize, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if next == "" {
			break
		}
		cursor = next
	}
	return all, nil
}
