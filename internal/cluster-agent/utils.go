// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}
