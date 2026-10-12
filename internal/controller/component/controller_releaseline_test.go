// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"github.com/stretchr/testify/assert"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestFilterWorkloadsByLine(t *testing.T) {
	wl := func(line string) openchoreov1alpha1.Workload {
		return openchoreov1alpha1.Workload{Spec: openchoreov1alpha1.WorkloadSpec{Owner: openchoreov1alpha1.WorkloadOwner{ReleaseLine: line}}}
	}
	items := []openchoreov1alpha1.Workload{wl(""), wl("main"), wl("v2")}
	assert.Len(t, filterWorkloadsByLine(items, ""), 1)
	assert.Len(t, filterWorkloadsByLine(items, "main"), 1, `"main" is an ordinary line name`)
	assert.Len(t, items, 3, "input is not modified")
}
