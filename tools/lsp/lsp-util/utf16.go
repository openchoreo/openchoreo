// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsputil

import "unicode/utf16"

// UTF16Length reports the number of UTF-16 code units in a UTF-8 string.
func UTF16Length(text string) int {
	units := 0
	for _, r := range text {
		units += utf16.RuneLen(r)
	}
	return units
}
