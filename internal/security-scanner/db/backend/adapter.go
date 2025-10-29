// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package backend

import "context"

type IDType interface {
	int32 | int64
}

type DBModel[ID IDType] interface {
	GetID() ID
	GetPodName() string
}

type DBQuerier[ID IDType, M DBModel[ID]] interface {
	InsertScannedPod(ctx context.Context, podName string) error
	GetScannedPod(ctx context.Context, id ID) (M, error)
	GetScannedPodByName(ctx context.Context, podName string) (M, error)
	ListScannedPods(ctx context.Context) ([]M, error)
	DeleteScannedPod(ctx context.Context, id ID) error
}

type genericAdapter[ID IDType, M DBModel[ID]] struct {
	q DBQuerier[ID, M]
}

func NewGenericAdapter[ID IDType, M DBModel[ID]](q DBQuerier[ID, M]) Querier {
	return &genericAdapter[ID, M]{q: q}
}

func (a *genericAdapter[ID, M]) InsertScannedPod(ctx context.Context, podName string) error {
	return a.q.InsertScannedPod(ctx, podName)
}

func (a *genericAdapter[ID, M]) GetScannedPod(ctx context.Context, id int64) (ScannedPod, error) {
	var typedID ID
	switch any(typedID).(type) {
	case int32:
		//nolint:gosec
		typedID = any(int32(id)).(ID)
	case int64:
		typedID = any(id).(ID)
	}

	pod, err := a.q.GetScannedPod(ctx, typedID)
	if err != nil {
		return ScannedPod{}, err
	}

	return ScannedPod{
		ID:      int64(pod.GetID()),
		PodName: pod.GetPodName(),
	}, nil
}

func (a *genericAdapter[ID, M]) GetScannedPodByName(ctx context.Context, podName string) (ScannedPod, error) {
	pod, err := a.q.GetScannedPodByName(ctx, podName)
	if err != nil {
		return ScannedPod{}, err
	}

	return ScannedPod{
		ID:      int64(pod.GetID()),
		PodName: pod.GetPodName(),
	}, nil
}

func (a *genericAdapter[ID, M]) ListScannedPods(ctx context.Context) ([]ScannedPod, error) {
	pods, err := a.q.ListScannedPods(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]ScannedPod, len(pods))
	for i, pod := range pods {
		result[i] = ScannedPod{
			ID:      int64(pod.GetID()),
			PodName: pod.GetPodName(),
		}
	}
	return result, nil
}

func (a *genericAdapter[ID, M]) DeleteScannedPod(ctx context.Context, id int64) error {
	var typedID ID
	switch any(typedID).(type) {
	case int32:
		//nolint:gosec
		typedID = any(int32(id)).(ID)
	case int64:
		typedID = any(id).(ID)
	}

	return a.q.DeleteScannedPod(ctx, typedID)
}
