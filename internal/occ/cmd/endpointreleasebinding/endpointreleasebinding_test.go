// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func bindingFor(name, env string) gen.EndpointReleaseBinding {
	b := gen.EndpointReleaseBinding{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.EndpointReleaseBindingSpec{
			Environment: env,
		},
	}
	b.Spec.Owner.EndpointName = "orders-api"
	return b
}

func ptrString(s string) *string { return &s }

func bindingWithReadyCondition(name, env, releaseName, readyReason string) gen.EndpointReleaseBinding {
	b := bindingFor(name, env)
	b.Spec.EndpointRelease = ptrString(releaseName)
	b.Status = &gen.EndpointReleaseBindingStatus{
		Conditions: &[]gen.Condition{
			{Type: "Ready", Reason: readyReason},
		},
	}
	return b
}

// --- printList tests ---

func TestPrint_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No endpoint release bindings found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.EndpointReleaseBinding{}))
	})
	assert.Contains(t, out, "No endpoint release bindings found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.EndpointReleaseBinding{
		bindingFor("orders-api-dev", "dev"),
		bindingFor("orders-api-prod", "prod"),
	}
	items[0].Metadata.CreationTimestamp = &now

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "ENDPOINT")
	assert.Contains(t, out, "ENVIRONMENT")
	assert.Contains(t, out, "RELEASE")
	assert.Contains(t, out, "STATUS")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "orders-api-dev")
	assert.Contains(t, out, "dev")
	assert.Contains(t, out, "prod")
	assert.Contains(t, out, "orders-api")
}

func TestPrint_ShowsReleaseAndReadyReason(t *testing.T) {
	items := []gen.EndpointReleaseBinding{
		bindingWithReadyCondition("orders-api-dev", "dev", "orders-api-abc123", "Ready"),
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "orders-api-abc123")
	assert.Contains(t, out, "Ready")
}

func TestPrint_HandlesMissingReadyCondition(t *testing.T) {
	// Binding with conditions but no Ready type — STATUS column stays empty
	b := bindingFor("orders-api-dev", "dev")
	b.Spec.EndpointRelease = ptrString("orders-api-abc123")
	b.Status = &gen.EndpointReleaseBindingStatus{
		Conditions: &[]gen.Condition{
			{Type: "Synced", Reason: "ReleaseSynced"},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.EndpointReleaseBinding{b}))
	})

	assert.Contains(t, out, "orders-api-abc123")
	// "Synced" is not surfaced — only the Ready condition's Reason makes it to STATUS.
	assert.NotContains(t, out, "ReleaseSynced")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.EndpointReleaseBinding{
		{
			Metadata: gen.ObjectMeta{Name: "no-spec", CreationTimestamp: &now},
			Spec:     nil,
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-spec")
}

// --- Validation tests ---

func TestList_ValidationError(t *testing.T) {
	erb := New(mocks.NewMockInterface(t))
	assert.Error(t, erb.List(ListParams{Namespace: ""}))
}

func TestGet_ValidationError(t *testing.T) {
	erb := New(mocks.NewMockInterface(t))
	assert.Error(t, erb.Get(GetParams{Namespace: ""}))
}

func TestDelete_ValidationError(t *testing.T) {
	erb := New(mocks.NewMockInterface(t))
	assert.Error(t, erb.Delete(DeleteParams{Namespace: "my-org", EndpointReleaseBindingName: ""}))
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpointReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	erb := New(mc)
	assert.EqualError(t, erb.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success_NoEndpointFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpointReleaseBindings(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListEndpointReleaseBindingsParams) bool {
		return p.Endpoint == nil
	})).Return(&gen.EndpointReleaseBindingList{
		Items:      []gen.EndpointReleaseBinding{bindingFor("orders-api-dev", "dev")},
		Pagination: gen.Pagination{},
	}, nil)

	erb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, erb.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "orders-api-dev")
}

func TestList_Success_WithEndpointFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpointReleaseBindings(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListEndpointReleaseBindingsParams) bool {
		return p.Endpoint != nil && *p.Endpoint == "orders-api"
	})).Return(&gen.EndpointReleaseBindingList{
		Items:      []gen.EndpointReleaseBinding{bindingFor("orders-api-dev", "dev")},
		Pagination: gen.Pagination{},
	}, nil)

	erb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, erb.List(ListParams{Namespace: "my-org", Endpoint: "orders-api"}))
	})

	assert.Contains(t, out, "orders-api-dev")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpointReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(&gen.EndpointReleaseBindingList{
		Items:      []gen.EndpointReleaseBinding{},
		Pagination: gen.Pagination{},
	}, nil)

	erb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, erb.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No endpoint release bindings found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointReleaseBinding(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found"))

	erb := New(mc)
	assert.EqualError(t, erb.Get(GetParams{Namespace: "my-org", EndpointReleaseBindingName: "missing"}), "not found")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointReleaseBinding(mock.Anything, "my-org", "orders-api-dev").Return(&gen.EndpointReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "orders-api-dev"},
	}, nil)

	erb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, erb.Get(GetParams{Namespace: "my-org", EndpointReleaseBindingName: "orders-api-dev"}))
	})

	assert.Contains(t, out, "name: orders-api-dev")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteEndpointReleaseBinding(mock.Anything, "my-org", "orders-api-dev").Return(fmt.Errorf("forbidden"))

	erb := New(mc)
	assert.EqualError(t, erb.Delete(DeleteParams{Namespace: "my-org", EndpointReleaseBindingName: "orders-api-dev"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteEndpointReleaseBinding(mock.Anything, "my-org", "orders-api-dev").Return(nil)

	erb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, erb.Delete(DeleteParams{Namespace: "my-org", EndpointReleaseBindingName: "orders-api-dev"}))
	})

	assert.Contains(t, out, "EndpointReleaseBinding 'orders-api-dev' deleted")
}
