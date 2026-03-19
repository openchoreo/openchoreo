// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package pagination

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchAll(t *testing.T) {
	t.Run("single page", func(t *testing.T) {
		fetcher := func(limit int, cursor string) ([]string, string, error) {
			return []string{"a", "b", "c"}, "", nil
		}
		result, err := FetchAll(fetcher)
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})

	t.Run("multi-page", func(t *testing.T) {
		call := 0
		fetcher := func(limit int, cursor string) ([]string, string, error) {
			call++
			switch call {
			case 1:
				assert.Empty(t, cursor)
				return []string{"a", "b"}, "page2", nil
			case 2:
				assert.Equal(t, "page2", cursor)
				return []string{"c", "d"}, "page3", nil
			case 3:
				assert.Equal(t, "page3", cursor)
				return []string{"e"}, "", nil
			default:
				t.Fatal("unexpected call")
				return nil, "", nil
			}
		}
		result, err := FetchAll(fetcher)
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c", "d", "e"}, result)
	})

	t.Run("empty first page", func(t *testing.T) {
		fetcher := func(limit int, cursor string) ([]string, string, error) {
			return nil, "", nil
		}
		result, err := FetchAll(fetcher)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("error on first page", func(t *testing.T) {
		fetcher := func(limit int, cursor string) ([]string, string, error) {
			return nil, "", fmt.Errorf("connection refused")
		}
		_, err := FetchAll(fetcher)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection refused")
	})

	t.Run("error on second page", func(t *testing.T) {
		call := 0
		fetcher := func(limit int, cursor string) ([]string, string, error) {
			call++
			if call == 1 {
				return []string{"a"}, "page2", nil
			}
			return nil, "", fmt.Errorf("timeout")
		}
		_, err := FetchAll(fetcher)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timeout")
	})

	t.Run("passes defaultChunkSize as limit", func(t *testing.T) {
		fetcher := func(limit int, cursor string) ([]string, string, error) {
			assert.Equal(t, defaultChunkSize, limit)
			return nil, "", nil
		}
		_, err := FetchAll(fetcher)
		require.NoError(t, err)
	})
}
