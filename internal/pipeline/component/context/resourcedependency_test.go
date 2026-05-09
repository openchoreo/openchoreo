// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestBuildResourceDependencyItem(t *testing.T) {
	t.Run("value_output_dispatches_to_literal_env_var", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{
			Ref:         "orders-db",
			EnvBindings: map[string]string{"host": "DB_HOST"},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "host", Value: "10.0.0.5"},
		}

		got, err := BuildResourceDependencyItem(dep, outputs)
		require.NoError(t, err)
		require.Len(t, got.EnvVars, 1)
		assert.Equal(t, "DB_HOST", got.EnvVars[0].Name)
		assert.Equal(t, "10.0.0.5", got.EnvVars[0].Value)
		assert.Nil(t, got.EnvVars[0].ValueFrom)
	})

	t.Run("secret_key_ref_output_dispatches_to_value_from_secret", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{
			Ref:         "orders-db",
			EnvBindings: map[string]string{"password": "DB_PASS"},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "password", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "orders-db-conn", Key: "password"}},
		}

		got, err := BuildResourceDependencyItem(dep, outputs)
		require.NoError(t, err)
		require.Len(t, got.EnvVars, 1)
		assert.Equal(t, "DB_PASS", got.EnvVars[0].Name)
		assert.Empty(t, got.EnvVars[0].Value)
		require.NotNil(t, got.EnvVars[0].ValueFrom)
		require.NotNil(t, got.EnvVars[0].ValueFrom.SecretKeyRef)
		assert.Equal(t, "orders-db-conn", got.EnvVars[0].ValueFrom.SecretKeyRef.Name)
		assert.Equal(t, "password", got.EnvVars[0].ValueFrom.SecretKeyRef.Key)
	})

	t.Run("config_map_key_ref_output_dispatches_to_value_from_config_map", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{
			Ref:         "orders-db",
			EnvBindings: map[string]string{"caCert": "DB_CA"},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "caCert", ConfigMapKeyRef: &v1alpha1.ConfigMapKeyRef{Name: "orders-db-tls", Key: "ca.crt"}},
		}

		got, err := BuildResourceDependencyItem(dep, outputs)
		require.NoError(t, err)
		require.Len(t, got.EnvVars, 1)
		assert.Equal(t, "DB_CA", got.EnvVars[0].Name)
		require.NotNil(t, got.EnvVars[0].ValueFrom)
		require.NotNil(t, got.EnvVars[0].ValueFrom.ConfigMapKeyRef)
		assert.Equal(t, "orders-db-tls", got.EnvVars[0].ValueFrom.ConfigMapKeyRef.Name)
		assert.Equal(t, "ca.crt", got.EnvVars[0].ValueFrom.ConfigMapKeyRef.Key)
	})

	t.Run("multiple_env_bindings_emit_in_deterministic_order", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{
			Ref: "orders-db",
			EnvBindings: map[string]string{
				"port": "DB_PORT",
				"host": "DB_HOST",
			},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "host", Value: "h"},
			{Name: "port", Value: "p"},
		}

		// Two calls must produce identical ordering despite map iteration randomness.
		first, err := BuildResourceDependencyItem(dep, outputs)
		require.NoError(t, err)
		for i := 0; i < 5; i++ {
			again, err := BuildResourceDependencyItem(dep, outputs)
			require.NoError(t, err)
			require.Equal(t, first, again)
		}
		// Sorted by output name → host, port.
		assert.Equal(t, "DB_HOST", first.EnvVars[0].Name)
		assert.Equal(t, "DB_PORT", first.EnvVars[1].Name)
	})

	t.Run("output_not_listed_in_env_bindings_is_skipped", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{
			Ref:         "orders-db",
			EnvBindings: map[string]string{"host": "DB_HOST"}, // port not bound
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "host", Value: "h"},
			{Name: "port", Value: "p"},
		}

		got, err := BuildResourceDependencyItem(dep, outputs)
		require.NoError(t, err)
		require.Len(t, got.EnvVars, 1)
		assert.Equal(t, "DB_HOST", got.EnvVars[0].Name)
	})

	t.Run("env_binding_keying_a_missing_output_returns_error", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{
			Ref:         "orders-db",
			EnvBindings: map[string]string{"password": "DB_PASS"},
		}
		// Provider hasn't resolved "password" yet.
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "host", Value: "h"},
		}

		_, err := BuildResourceDependencyItem(dep, outputs)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrOutputNotResolved))
		assert.Contains(t, err.Error(), "password")
	})

	t.Run("secret_file_binding_synthesizes_volume_and_mount", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{
			Ref:          "orders-db",
			FileBindings: map[string]string{"caCert": "/etc/tls"},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "caCert", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "orders-db-tls", Key: "tls.crt"}},
		}

		got, err := BuildResourceDependencyItem(dep, outputs)
		require.NoError(t, err)
		require.Len(t, got.VolumeMounts, 1)
		require.Len(t, got.Volumes, 1)

		volName := got.Volumes[0].Name
		assert.Equal(t, VolumeEntry{
			Name:   volName,
			Secret: &SecretVolume{SecretName: "orders-db-tls"},
		}, got.Volumes[0])
		assert.Equal(t, VolumeMountEntry{
			Name: volName, MountPath: "/etc/tls", SubPath: "tls.crt",
		}, got.VolumeMounts[0])
	})

	t.Run("config_map_file_binding_synthesizes_volume_and_mount", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{
			Ref:          "orders-db",
			FileBindings: map[string]string{"caCert": "/etc/tls"},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "caCert", ConfigMapKeyRef: &v1alpha1.ConfigMapKeyRef{Name: "orders-db-tls", Key: "ca.crt"}},
		}

		got, err := BuildResourceDependencyItem(dep, outputs)
		require.NoError(t, err)
		require.Len(t, got.VolumeMounts, 1)
		require.Len(t, got.Volumes, 1)

		volName := got.Volumes[0].Name
		require.NotNil(t, got.Volumes[0].ConfigMap)
		assert.Equal(t, "orders-db-tls", got.Volumes[0].ConfigMap.Name)
		assert.Equal(t, VolumeMountEntry{
			Name: volName, MountPath: "/etc/tls", SubPath: "ca.crt",
		}, got.VolumeMounts[0])
	})

	t.Run("value_kind_output_with_file_binding_returns_error", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{
			Ref:          "orders-db",
			FileBindings: map[string]string{"host": "/etc/host"},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "host", Value: "10.0.0.5"},
		}

		_, err := BuildResourceDependencyItem(dep, outputs)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidFileBinding))
		assert.Contains(t, err.Error(), "host")
	})

	t.Run("multiple_files_from_same_secret_dedupe_volume", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{
			Ref: "orders-db",
			FileBindings: map[string]string{
				"crt": "/etc/tls/crt",
				"key": "/etc/tls/key",
			},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "crt", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "orders-db-tls", Key: "tls.crt"}},
			{Name: "key", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "orders-db-tls", Key: "tls.key"}},
		}

		got, err := BuildResourceDependencyItem(dep, outputs)
		require.NoError(t, err)
		assert.Len(t, got.Volumes, 1, "two outputs from same Secret should share one volume")
		require.Len(t, got.VolumeMounts, 2)
		assert.Equal(t, got.VolumeMounts[0].Name, got.VolumeMounts[1].Name, "both mounts point at the shared volume")
		assert.Equal(t, got.VolumeMounts[0].Name, got.Volumes[0].Name)
	})

	t.Run("multiple_files_from_same_config_map_dedupe_volume", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{
			Ref: "orders-db",
			FileBindings: map[string]string{
				"ca":  "/etc/ca.crt",
				"crt": "/etc/cert.crt",
			},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "ca", ConfigMapKeyRef: &v1alpha1.ConfigMapKeyRef{Name: "shared-cm", Key: "ca.crt"}},
			{Name: "crt", ConfigMapKeyRef: &v1alpha1.ConfigMapKeyRef{Name: "shared-cm", Key: "cert.crt"}},
		}

		got, err := BuildResourceDependencyItem(dep, outputs)
		require.NoError(t, err)
		assert.Len(t, got.Volumes, 1)
		assert.Len(t, got.VolumeMounts, 2)
	})

	t.Run("same_secret_referenced_by_two_resources_does_not_dedupe", func(t *testing.T) {
		// dep "db" referencing secret "shared"
		depA := v1alpha1.WorkloadResourceDependency{
			Ref:          "db",
			FileBindings: map[string]string{"key": "/etc/db"},
		}
		outA := []v1alpha1.ResolvedResourceOutput{
			{Name: "key", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "shared", Key: "k"}},
		}
		// dep "cache" referencing the same secret name "shared" — independent resource
		depB := v1alpha1.WorkloadResourceDependency{
			Ref:          "cache",
			FileBindings: map[string]string{"key": "/etc/cache"},
		}
		outB := []v1alpha1.ResolvedResourceOutput{
			{Name: "key", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "shared", Key: "k"}},
		}

		gotA, err := BuildResourceDependencyItem(depA, outA)
		require.NoError(t, err)
		gotB, err := BuildResourceDependencyItem(depB, outB)
		require.NoError(t, err)

		assert.NotEqual(t, gotA.Volumes[0].Name, gotB.Volumes[0].Name,
			"different resource refs must produce different volume names even if secret name matches")
	})

	t.Run("volume_name_is_deterministic_across_calls", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{
			Ref:          "orders-db",
			FileBindings: map[string]string{"caCert": "/etc/tls"},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "caCert", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "orders-db-tls", Key: "tls.crt"}},
		}

		first, _ := BuildResourceDependencyItem(dep, outputs)
		for i := 0; i < 10; i++ {
			again, _ := BuildResourceDependencyItem(dep, outputs)
			require.Equal(t, first.Volumes[0].Name, again.Volumes[0].Name)
		}
	})

	t.Run("volume_name_is_length_bounded", func(t *testing.T) {
		// Long-everything inputs should still produce a name within the K8s 63-char limit.
		longRef := "an-incredibly-long-resource-dependency-reference-name-that-pushes-the-limit"
		longSecret := "an-extremely-long-secret-name-from-a-very-deeply-nested-resource"
		dep := v1alpha1.WorkloadResourceDependency{
			Ref:          longRef,
			FileBindings: map[string]string{"k": "/x"},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "k", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: longSecret, Key: "k"}},
		}

		got, err := BuildResourceDependencyItem(dep, outputs)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(got.Volumes[0].Name), 63, "volume name must fit K8s 63-char limit")
	})

	t.Run("empty_env_and_file_bindings_returns_empty_item", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{Ref: "orders-db"}

		got, err := BuildResourceDependencyItem(dep, []v1alpha1.ResolvedResourceOutput{})
		require.NoError(t, err)
		assert.Equal(t, "orders-db", got.Ref)
		assert.NotNil(t, got.EnvVars)
		assert.NotNil(t, got.VolumeMounts)
		assert.NotNil(t, got.Volumes)
		assert.Empty(t, got.EnvVars)
		assert.Empty(t, got.VolumeMounts)
		assert.Empty(t, got.Volumes)
	})

	t.Run("nil_outputs_with_bindings_returns_error_per_first_missing", func(t *testing.T) {
		// Caller passes nil outputs (provider RRB hasn't resolved any outputs yet) but the
		// dep references one. Builder errors on the first missing reference.
		dep := v1alpha1.WorkloadResourceDependency{
			Ref:         "orders-db",
			EnvBindings: map[string]string{"host": "DB_HOST"},
		}

		_, err := BuildResourceDependencyItem(dep, nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrOutputNotResolved))
	})

	t.Run("env_and_file_bindings_combined_in_one_dep", func(t *testing.T) {
		// Smoke combination: literal value, secret env, configmap file in a single dep.
		dep := v1alpha1.WorkloadResourceDependency{
			Ref: "orders-db",
			EnvBindings: map[string]string{
				"host":     "DB_HOST",
				"password": "DB_PASS",
			},
			FileBindings: map[string]string{"caCert": "/etc/tls"},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "host", Value: "10.0.0.5"},
			{Name: "password", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "orders-db-conn", Key: "password"}},
			{Name: "caCert", ConfigMapKeyRef: &v1alpha1.ConfigMapKeyRef{Name: "orders-db-tls", Key: "ca.crt"}},
		}

		got, err := BuildResourceDependencyItem(dep, outputs)
		require.NoError(t, err)

		assert.Len(t, got.EnvVars, 2)
		assert.Len(t, got.VolumeMounts, 1)
		assert.Len(t, got.Volumes, 1)

		// EnvVars sorted: host, password
		assert.Equal(t, "DB_HOST", got.EnvVars[0].Name)
		assert.Equal(t, "10.0.0.5", got.EnvVars[0].Value)
		assert.Equal(t, "DB_PASS", got.EnvVars[1].Name)
		require.NotNil(t, got.EnvVars[1].ValueFrom)
		assert.Equal(t, "orders-db-conn", got.EnvVars[1].ValueFrom.SecretKeyRef.Name)

		// VolumeMount points at configmap-backed volume
		assert.Equal(t, "/etc/tls", got.VolumeMounts[0].MountPath)
		assert.Equal(t, "ca.crt", got.VolumeMounts[0].SubPath)
		require.NotNil(t, got.Volumes[0].ConfigMap)
		assert.Equal(t, "orders-db-tls", got.Volumes[0].ConfigMap.Name)
	})

	t.Run("ref_carries_through_to_item", func(t *testing.T) {
		dep := v1alpha1.WorkloadResourceDependency{Ref: "my-db"}
		got, err := BuildResourceDependencyItem(dep, nil)
		require.NoError(t, err)
		assert.Equal(t, "my-db", got.Ref)
	})

	t.Run("same_output_in_env_and_file_bindings_emits_both", func(t *testing.T) {
		// Locks the contract: a developer can name the same output in both envBindings
		// and fileBindings to get both an env var (e.g. POSTGRES_PASSWORD) AND a mounted
		// file (e.g. /etc/db/password) backed by the same secret key. The two paths are
		// independent — env binding produces an env var, file binding produces a
		// volume + mount. Output kind must be ref-kind for the file path to succeed.
		dep := v1alpha1.WorkloadResourceDependency{
			Ref:          "orders-db",
			EnvBindings:  map[string]string{"password": "DB_PASS"},
			FileBindings: map[string]string{"password": "/etc/db/password"},
		}
		outputs := []v1alpha1.ResolvedResourceOutput{
			{Name: "password", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "orders-db-conn", Key: "password"}},
		}

		got, err := BuildResourceDependencyItem(dep, outputs)
		require.NoError(t, err)

		// Env var emitted
		require.Len(t, got.EnvVars, 1)
		assert.Equal(t, "DB_PASS", got.EnvVars[0].Name)
		require.NotNil(t, got.EnvVars[0].ValueFrom)
		assert.Equal(t, "orders-db-conn", got.EnvVars[0].ValueFrom.SecretKeyRef.Name)
		assert.Equal(t, "password", got.EnvVars[0].ValueFrom.SecretKeyRef.Key)

		// Volume + mount emitted
		require.Len(t, got.VolumeMounts, 1)
		require.Len(t, got.Volumes, 1)
		assert.Equal(t, "/etc/db/password", got.VolumeMounts[0].MountPath)
		assert.Equal(t, "password", got.VolumeMounts[0].SubPath)
		require.NotNil(t, got.Volumes[0].Secret)
		assert.Equal(t, "orders-db-conn", got.Volumes[0].Secret.SecretName)
	})
}

// Spot-check: the volume-name helper is stable and changes when any input changes.
func TestResourceDepVolumeName(t *testing.T) {
	base := resourceDepVolumeName("ref", "secret", "name")
	cases := []struct {
		ref, kind, name string
	}{
		{"OTHER", "secret", "name"},
		{"ref", "configmap", "name"},
		{"ref", "secret", "OTHER"},
	}
	for _, c := range cases {
		got := resourceDepVolumeName(c.ref, c.kind, c.name)
		if diff := cmp.Diff(base, got); diff == "" {
			t.Errorf("expected different name for %+v, got same: %s", c, got)
		}
	}
}

// Resource-dependency volumes (r-*) and configurations volumes (file-mount-*) land on the
// same Pod. Lock the prefix isolation that keeps their name spaces disjoint — a regression
// here would corrupt the rendered Pod spec.
func TestResourceDepVolumeNameDoesNotCollideWithConfigurationsVolumes(t *testing.T) {
	// Adversarial inputs: identical source names across both paths.
	rdName := resourceDepVolumeName("orders-db", "secret", "orders-conn")
	cfgName := "file-mount-" + generateVolumeHash("/etc/orders", "orders-conn")

	assert.True(t, strings.HasPrefix(rdName, "r-"),
		"resource-dep volume name must use the r- prefix; got %q", rdName)
	assert.True(t, strings.HasPrefix(cfgName, "file-mount-"),
		"configurations volume name must use the file-mount- prefix; got %q", cfgName)
	assert.NotEqual(t, rdName, cfgName)
}
