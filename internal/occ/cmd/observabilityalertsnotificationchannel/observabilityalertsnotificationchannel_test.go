// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/observabilityalertsnotificationchannel/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
		w.Close()
		r.Close()
	}()

	fn()

	os.Stdout = origStdout
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	return buf.String()
}

// --- printList tests ---

func TestPrintList_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No observability alerts notification channels found")
}

func TestPrintList_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.ObservabilityAlertsNotificationChannel{}))
	})
	assert.Contains(t, out, "No observability alerts notification channels found")
}

func TestPrintList_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ObservabilityAlertsNotificationChannel{
		{Metadata: gen.ObjectMeta{Name: "channel-1", CreationTimestamp: &now}},
		{Metadata: gen.ObjectMeta{Name: "channel-2"}},
	}
	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "channel-1")
	assert.Contains(t, out, "channel-2")
}

// --- List tests ---

func TestList_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	o := New(mc)
	err := o.List(ListParams{Namespace: ""})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListObservabilityAlertsNotificationChannels(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("server error"))

	o := New(mc)
	assert.EqualError(t, o.List(ListParams{Namespace: "org-a"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListObservabilityAlertsNotificationChannels(mock.Anything, "org-a", mock.Anything).Return(&gen.ObservabilityAlertsNotificationChannelList{
		Items:      []gen.ObservabilityAlertsNotificationChannel{{Metadata: gen.ObjectMeta{Name: "channel-1"}}},
		Pagination: gen.Pagination{},
	}, nil)

	o := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, o.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "channel-1")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListObservabilityAlertsNotificationChannels(mock.Anything, "org-a", mock.Anything).Return(&gen.ObservabilityAlertsNotificationChannelList{
		Items: []gen.ObservabilityAlertsNotificationChannel{
			{Metadata: gen.ObjectMeta{Name: "channel-1", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "channel-2", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	o := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, o.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "channel-1")
	assert.Contains(t, out, "channel-2")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListObservabilityAlertsNotificationChannels(mock.Anything, "org-a", mock.Anything).Return(&gen.ObservabilityAlertsNotificationChannelList{
		Items:      []gen.ObservabilityAlertsNotificationChannel{},
		Pagination: gen.Pagination{},
	}, nil)

	o := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, o.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "No observability alerts notification channels found")
}

// --- Get tests ---

func TestGet_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	o := New(mc)
	err := o.Get(GetParams{Namespace: "", ChannelName: "channel-1"})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetObservabilityAlertsNotificationChannel(mock.Anything, "org-a", "missing").Return(nil, fmt.Errorf("not found: missing"))

	o := New(mc)
	assert.EqualError(t, o.Get(GetParams{Namespace: "org-a", ChannelName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetObservabilityAlertsNotificationChannel(mock.Anything, "org-a", "channel-1").Return(&gen.ObservabilityAlertsNotificationChannel{
		Metadata: gen.ObjectMeta{Name: "channel-1"},
	}, nil)

	o := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, o.Get(GetParams{Namespace: "org-a", ChannelName: "channel-1"}))
	})
	assert.Contains(t, out, "name: channel-1")
}

// --- Delete tests ---

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	o := New(mc)
	err := o.Delete(DeleteParams{Namespace: "", ChannelName: "channel-1"})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteObservabilityAlertsNotificationChannel(mock.Anything, "org-a", "channel-1").Return(fmt.Errorf("forbidden"))

	o := New(mc)
	assert.EqualError(t, o.Delete(DeleteParams{Namespace: "org-a", ChannelName: "channel-1"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteObservabilityAlertsNotificationChannel(mock.Anything, "org-a", "channel-1").Return(nil)

	o := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, o.Delete(DeleteParams{Namespace: "org-a", ChannelName: "channel-1"}))
	})
	assert.Contains(t, out, "ObservabilityAlertsNotificationChannel 'channel-1' deleted")
}
