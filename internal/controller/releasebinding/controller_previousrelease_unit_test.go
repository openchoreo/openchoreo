// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestPreviousDataPlaneResources(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	const releaseName = "my-component-staging"
	releaseBinding := &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "my-component-staging", Namespace: testNamespace, UID: "rb-uid"},
	}

	// release returns a dataplane Release holding one resource with the given raw JSON
	release := func(controlled bool, raw string) *openchoreov1alpha1.RenderedRelease {
		rel := &openchoreov1alpha1.RenderedRelease{
			ObjectMeta: metav1.ObjectMeta{Name: releaseName, Namespace: testNamespace},
			Spec: openchoreov1alpha1.RenderedReleaseSpec{
				Resources: []openchoreov1alpha1.RenderedManifest{
					{ID: "configmap-app-env", Object: &runtime.RawExtension{Raw: []byte(raw)}},
				},
			},
		}
		if controlled {
			require.NoError(t, controllerutil.SetControllerReference(releaseBinding, rel, scheme))
		}
		return rel
	}
	configMap := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"app-env"},"data":{"LOG_LEVEL":"info"}}`

	withEmptyResource := release(true, configMap)
	withEmptyResource.Spec.Resources = append(withEmptyResource.Spec.Resources,
		openchoreov1alpha1.RenderedManifest{ID: "empty", Object: &runtime.RawExtension{}})
	withNonObjectResource := release(true, configMap)
	withNonObjectResource.Spec.Resources = append(withNonObjectResource.Spec.Resources,
		openchoreov1alpha1.RenderedManifest{ID: "list", Object: &runtime.RawExtension{Raw: []byte(`["not", "an", "object"]`)}})

	tests := []struct {
		name    string
		objects []client.Object
		getErr  error
		want    []map[string]any
		wantErr string
	}{
		{
			name: "no Release yet",
			want: nil,
		},
		{
			name:    "Release controlled by the ReleaseBinding",
			objects: []client.Object{release(true, configMap)},
			want: []map[string]any{{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": "app-env"},
				"data":       map[string]any{"LOG_LEVEL": "info"},
			}},
		},
		{
			name:    "Release not controlled by the ReleaseBinding",
			objects: []client.Object{release(false, configMap)},
			want:    nil,
		},
		{
			name:    "Release with a resource that is not an object skips it",
			objects: []client.Object{withNonObjectResource},
			want: []map[string]any{{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": "app-env"},
				"data":       map[string]any{"LOG_LEVEL": "info"},
			}},
		},
		{
			name:    "Release with an empty resource skips it",
			objects: []client.Object{withEmptyResource},
			want: []map[string]any{{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": "app-env"},
				"data":       map[string]any{"LOG_LEVEL": "info"},
			}},
		},
		{
			name:    "failing Get is returned",
			getErr:  errors.New("connection refused"),
			wantErr: `failed to get dataplane Release "my-component-staging": connection refused`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...)
			if tt.getErr != nil {
				builder = builder.WithInterceptorFuncs(interceptor.Funcs{
					Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
						return tt.getErr
					},
				})
			}
			r := &Reconciler{Client: builder.Build(), Scheme: scheme}

			got, err := r.previousDataPlaneResources(context.Background(), releaseBinding, releaseName)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
