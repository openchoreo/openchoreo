// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsputil

import "testing"

func TestUTF16Length(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       int
	}{
		{"empty", "", 0}, {"ascii", "kind", 4}, {"bmp", "añb", 3},
		{"non_bmp", "a😀b", 4}, {"multiple_non_bmp", "😀🌟", 4}, {"invalid_utf8", string([]byte{0xff}), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := UTF16Length(tc.text); got != tc.want {
				t.Fatalf("length=%d, want %d", got, tc.want)
			}
		})
	}
}
