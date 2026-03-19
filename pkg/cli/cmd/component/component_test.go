// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestParseCSV(t *testing.T) {
	tests := []struct {
		name     string
		flagVal  string
		wantNil  bool
		wantVals []string
	}{
		{
			name:     "comma separated values",
			flagVal:  "a,b,c",
			wantVals: []string{"a", "b", "c"},
		},
		{
			name:    "empty string",
			flagVal: "",
			wantNil: true,
		},
		{
			name:     "single value",
			flagVal:  "only",
			wantVals: []string{"only"},
		},
		{
			name:     "values with whitespace",
			flagVal:  " a , b , c ",
			wantVals: []string{"a", "b", "c"},
		},
		{
			name:     "trailing comma produces no empty entry",
			flagVal:  "a,b,",
			wantVals: []string{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("test-flag", "", "")
			if tt.flagVal != "" {
				_ = cmd.Flags().Set("test-flag", tt.flagVal)
			}
			result := parseCSV(cmd, "test-flag")
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			assert.Equal(t, tt.wantVals, result)
		})
	}
}
