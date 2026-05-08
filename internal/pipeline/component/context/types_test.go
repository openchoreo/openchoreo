// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestNewDependenciesContextData_ResourceMerge(t *testing.T) {
	t.Run("exposes_resources_per_item_at_dependencies_resources", func(t *testing.T) {
		data := ConnectionsData{
			Resources: []ResourceDependencyItem{
				{Ref: "orders-db"},
				{Ref: "cache"},
			},
		}

		ctx := newDependenciesContextData(data)
		require.Len(t, ctx.Resources, 2)
		assert.Equal(t, "orders-db", ctx.Resources[0].Ref)
		assert.Equal(t, "cache", ctx.Resources[1].Ref)
	})

	t.Run("merges_endpoint_and_resource_env_vars", func(t *testing.T) {
		data := ConnectionsData{
			Items: []ConnectionItem{
				{
					Namespace: "ns", Project: "p", Component: "svc-a", Endpoint: "http", Visibility: "project",
					EnvVars: []corev1.EnvVar{
						{Name: "SVC_A_URL", Value: "http://svc-a:8080"},
					},
				},
			},
			Resources: []ResourceDependencyItem{
				{
					Ref: "orders-db",
					EnvVars: []corev1.EnvVar{
						{Name: "DB_HOST", Value: "10.0.0.5"},
						{Name: "DB_PASS", ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "db-conn"},
								Key:                  "password",
							},
						}},
					},
				},
			},
		}

		ctx := newDependenciesContextData(data)
		// Endpoints first, then resources, in declaration order within each.
		require.Len(t, ctx.EnvVars, 3)
		assert.Equal(t, "SVC_A_URL", ctx.EnvVars[0].Name)
		assert.Equal(t, "DB_HOST", ctx.EnvVars[1].Name)
		assert.Equal(t, "DB_PASS", ctx.EnvVars[2].Name)
		require.NotNil(t, ctx.EnvVars[2].ValueFrom)
		assert.Equal(t, "db-conn", ctx.EnvVars[2].ValueFrom.SecretKeyRef.Name)
	})

	t.Run("merges_resource_volume_mounts_and_volumes", func(t *testing.T) {
		data := ConnectionsData{
			Resources: []ResourceDependencyItem{
				{
					Ref:          "db",
					VolumeMounts: []corev1.VolumeMount{{Name: "v1", MountPath: "/etc/db"}},
					Volumes: []corev1.Volume{{
						Name: "v1",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: "db-conn"},
						},
					}},
				},
				{
					Ref:          "cache",
					VolumeMounts: []corev1.VolumeMount{{Name: "v2", MountPath: "/etc/cache"}},
					Volumes: []corev1.Volume{{
						Name: "v2",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "cache-cm"},
							},
						},
					}},
				},
			},
		}

		ctx := newDependenciesContextData(data)
		require.Len(t, ctx.VolumeMounts, 2)
		require.Len(t, ctx.Volumes, 2)
		assert.Equal(t, "v1", ctx.VolumeMounts[0].Name)
		assert.Equal(t, "v2", ctx.VolumeMounts[1].Name)
		assert.Equal(t, "v1", ctx.Volumes[0].Name)
		assert.Equal(t, "v2", ctx.Volumes[1].Name)
	})

	t.Run("empty_resource_items_yields_empty_volumes_and_mounts", func(t *testing.T) {
		ctx := newDependenciesContextData(ConnectionsData{})
		assert.NotNil(t, ctx.Resources, "empty slice expected, not nil")
		assert.NotNil(t, ctx.VolumeMounts)
		assert.NotNil(t, ctx.Volumes)
		assert.Empty(t, ctx.Resources)
		assert.Empty(t, ctx.VolumeMounts)
		assert.Empty(t, ctx.Volumes)
	})

	t.Run("endpoints_only_input_keeps_existing_envVars_shape", func(t *testing.T) {
		// Backward compat: a workload with only endpoint connections renders an envVars list
		// shaped exactly like before. Locks the "merged shape stays compatible when no resources
		// are present" contract so existing CCT samples keep working.
		data := ConnectionsData{
			Items: []ConnectionItem{
				{
					Namespace: "ns", Project: "p", Component: "svc-a", Endpoint: "http", Visibility: "project",
					EnvVars: []corev1.EnvVar{
						{Name: "SVC_A_URL", Value: "http://svc-a:8080"},
						{Name: "SVC_A_HOST", Value: "svc-a"},
					},
				},
			},
		}
		ctx := newDependenciesContextData(data)

		want := []corev1.EnvVar{
			{Name: "SVC_A_URL", Value: "http://svc-a:8080"},
			{Name: "SVC_A_HOST", Value: "svc-a"},
		}
		if diff := cmp.Diff(want, ctx.EnvVars); diff != "" {
			t.Errorf("merged envVars mismatch (-want +got):\n%s", diff)
		}
		// Resources side stays empty.
		assert.Empty(t, ctx.Resources)
		assert.Empty(t, ctx.VolumeMounts)
		assert.Empty(t, ctx.Volumes)
	})

	t.Run("nil_resource_envvars_volumes_normalized_to_empty_slices", func(t *testing.T) {
		data := ConnectionsData{
			Resources: []ResourceDependencyItem{
				{Ref: "db"}, // all slices nil
			},
		}
		ctx := newDependenciesContextData(data)
		require.Len(t, ctx.Resources, 1)
		assert.NotNil(t, ctx.Resources[0].EnvVars)
		assert.NotNil(t, ctx.Resources[0].VolumeMounts)
		assert.NotNil(t, ctx.Resources[0].Volumes)
	})

	t.Run("per_resource_view_exposes_envvars_independently_of_merged", func(t *testing.T) {
		// Templates that want per-resource filtering can iterate dependencies.resources[]
		// and pick env vars off each item, instead of using the merged dependencies.envVars.
		data := ConnectionsData{
			Resources: []ResourceDependencyItem{
				{
					Ref:     "db",
					EnvVars: []corev1.EnvVar{{Name: "DB_HOST", Value: "h1"}},
				},
				{
					Ref:     "cache",
					EnvVars: []corev1.EnvVar{{Name: "CACHE_HOST", Value: "h2"}},
				},
			},
		}
		ctx := newDependenciesContextData(data)

		// Per-resource view stays separated.
		assert.Equal(t, "DB_HOST", ctx.Resources[0].EnvVars[0].Name)
		assert.Equal(t, "CACHE_HOST", ctx.Resources[1].EnvVars[0].Name)
		// Merged view contains both.
		assert.Len(t, ctx.EnvVars, 2)
	})
}
