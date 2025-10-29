// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package backend

import "context"

type Querier interface {
	DeleteScannedPod(ctx context.Context, id int64) error
	GetScannedPod(ctx context.Context, id int64) (ScannedPod, error)
	GetScannedPodByName(ctx context.Context, podName string) (ScannedPod, error)
	InsertScannedPod(ctx context.Context, podName string) error
	ListScannedPods(ctx context.Context) ([]ScannedPod, error)
}
