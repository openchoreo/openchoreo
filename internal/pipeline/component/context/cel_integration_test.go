// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// TestCELMacroIntegration exercises the full path from ComponentContextInput through
// BuildComponentContext, ToMap, and CEL macro evaluation. Every assertion uses deep
// equality so that any change in output shape is caught, serving as a behavioral
// equivalence check across refactors.
func TestCELMacroIntegration(t *testing.T) {
	workload := &v1alpha1.Workload{
		Spec: v1alpha1.WorkloadSpec{
			WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
				Container: v1alpha1.Container{
					Image: "myapp:v1",
					Env: []v1alpha1.EnvVar{
						{Key: "LOG_LEVEL", Value: "info"},
						{Key: "DEBUG", Value: "true"},
					},
					Files: []v1alpha1.FileVar{
						{Key: "app.yaml", MountPath: "/etc/config", Value: "server:\n  port: 8080"},
						{Key: "logging.properties", MountPath: "/etc/config", Value: "log.level=INFO"},
					},
				},
				Endpoints: map[string]v1alpha1.WorkloadEndpoint{
					"http": {Port: 8080, Type: v1alpha1.EndpointTypeHTTP},
					"grpc": {Port: 9090, Type: v1alpha1.EndpointTypeGRPC},
				},
			},
		},
	}

	secretRefs := map[string]*v1alpha1.SecretReference{
		"db-secret": {
			Spec: v1alpha1.SecretReferenceSpec{
				Data: []v1alpha1.SecretDataSource{
					{SecretKey: "password", RemoteRef: v1alpha1.RemoteReference{Key: "db/password", Property: "value"}},
				},
			},
		},
	}

	workloadWithSecrets := &v1alpha1.Workload{
		Spec: v1alpha1.WorkloadSpec{
			WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
				Container: v1alpha1.Container{
					Image: "myapp:v1",
					Env: []v1alpha1.EnvVar{
						{Key: "LOG_LEVEL", Value: "info"},
						{Key: "DB_PASSWORD", ValueFrom: &v1alpha1.EnvVarValueFrom{
							SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "db-secret", Key: "password"},
						}},
					},
					Files: []v1alpha1.FileVar{
						{Key: "app.yaml", MountPath: "/etc/config", Value: "server:\n  port: 8080"},
					},
				},
				Endpoints: map[string]v1alpha1.WorkloadEndpoint{
					"http": {Port: 8080, Type: v1alpha1.EndpointTypeHTTP},
				},
			},
		},
	}

	depData := ConnectionsData{
		Items: []ConnectionItem{
			{
				Namespace: "ns1", Project: "proj1", Component: "svc-a",
				Endpoint: "http", Visibility: "project",
				EnvVars: []EnvVarEntry{
					{Name: "SVC_A_URL", Value: "http://svc-a.ns1:8080"},
					{Name: "SVC_A_HOST", Value: "svc-a.ns1"},
				},
			},
		},
		Resources: []ResourceDependencyItem{
			{
				Ref: "my-db",
				EnvVars: []EnvVarEntry{
					{Name: "DB_HOST", Value: "db.example.com"},
				},
				VolumeMounts: []VolumeMountEntry{
					{Name: "db-creds", MountPath: "/etc/db", SubPath: "password"},
				},
				Volumes: []VolumeEntry{
					{Name: "db-creds", Secret: &SecretVolume{SecretName: "db-conn-secret"}},
				},
			},
		},
	}

	metadata := MetadataContext{
		Name:               "myapp-dev-12345678",
		Namespace:          "dp-test-ns",
		ComponentName:      "myapp",
		ComponentUID:       "uid-1234",
		ComponentNamespace: "cp-test-ns",
		ProjectName:        "test-project",
		ProjectUID:         "proj-uid",
		DataPlaneName:      "test-dp",
		DataPlaneUID:       "dp-uid",
		EnvironmentName:    "dev",
		EnvironmentUID:     "env-uid",
		Labels:             map[string]string{},
		Annotations:        map[string]string{},
		PodSelectors:       map[string]string{"openchoreo.dev/component-uid": "uid-1234"},
	}

	prefix := "myapp-dev"

	// Pre-compute resource names and volume hashes used in expected values.
	appYamlVolName := "file-mount-" + generateVolumeHash("/etc/config", "app.yaml")
	loggingVolName := "file-mount-" + generateVolumeHash("/etc/config", "logging.properties")
	appYamlConfigRes := generateConfigResourceName(prefix, "app.yaml")
	loggingConfigRes := generateConfigResourceName(prefix, "logging.properties")
	envConfigsRes := generateEnvResourceName(prefix)
	envSecretsRes := generateSecretEnvResourceName(prefix, []EnvConfiguration{{Name: "DB_PASSWORD", RemoteRef: &RemoteRefData{Key: "db/password", Property: "value"}}})

	// buildVolumes sorts output by volume name; compute the sorted order.
	type volInfo struct {
		name     string
		fileName string
	}
	sortedVols := []volInfo{
		{appYamlVolName, "app.yaml"},
		{loggingVolName, "logging.properties"},
	}
	sort.Slice(sortedVols, func(i, j int) bool { return sortedVols[i].name < sortedVols[j].name })

	expectedConfigVolumes := make([]any, len(sortedVols))
	for i, v := range sortedVols {
		expectedConfigVolumes[i] = map[string]any{
			"name":      v.name,
			"configMap": map[string]any{"name": generateConfigResourceName(prefix, v.fileName)},
		}
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	diffOpts := cmp.Options{cmpopts.EquateEmpty()}

	buildContext := func(t *testing.T, w *v1alpha1.Workload, secrets map[string]*v1alpha1.SecretReference, deps ConnectionsData) map[string]any {
		t.Helper()
		input := &ComponentContextInput{
			Component:     &v1alpha1.Component{},
			ComponentType: &v1alpha1.ComponentType{},
			DataPlane:     &v1alpha1.DataPlane{},
			Environment: &v1alpha1.Environment{Spec: v1alpha1.EnvironmentSpec{
				DataPlaneRef: &v1alpha1.DataPlaneRef{Kind: v1alpha1.DataPlaneRefKindDataPlane, Name: "test-dp"},
			}},
			WorkloadData:   ExtractWorkloadData(w),
			Configurations: ExtractConfigurationsFromWorkload(secrets, w),
			Dependencies:   deps,
			Metadata:       metadata,
		}
		ctx, err := BuildComponentContext(input)
		if err != nil {
			t.Fatalf("BuildComponentContext failed: %v", err)
		}
		return ctx.ToMap()
	}

	eval := func(t *testing.T, expr string, ctxMap map[string]any) any {
		t.Helper()
		result, err := engine.Render("${"+expr+"}", ctxMap)
		if err != nil {
			t.Fatalf("CEL error for %q: %v", expr, err)
		}
		return result
	}

	t.Run("configurations.toConfigFileList", func(t *testing.T) {
		got := eval(t, "configurations.toConfigFileList()", buildContext(t, workload, nil, ConnectionsData{}))
		want := []any{
			map[string]any{
				"name":         "app.yaml",
				"mountPath":    "/etc/config",
				"value":        "server:\n  port: 8080",
				"resourceName": appYamlConfigRes,
			},
			map[string]any{
				"name":         "logging.properties",
				"mountPath":    "/etc/config",
				"value":        "log.level=INFO",
				"resourceName": loggingConfigRes,
			},
		}
		if diff := cmp.Diff(want, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("configurations.toSecretFileList_empty", func(t *testing.T) {
		got := eval(t, "configurations.toSecretFileList()", buildContext(t, workload, nil, ConnectionsData{}))
		if diff := cmp.Diff([]any{}, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("configurations.toContainerEnvFrom_config_only", func(t *testing.T) {
		got := eval(t, "configurations.toContainerEnvFrom()", buildContext(t, workload, nil, ConnectionsData{}))
		want := []any{
			map[string]any{
				"configMapRef": map[string]any{"name": envConfigsRes},
			},
		}
		if diff := cmp.Diff(want, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("configurations.toContainerEnvFrom_with_secrets", func(t *testing.T) {
		got := eval(t, "configurations.toContainerEnvFrom()", buildContext(t, workloadWithSecrets, secretRefs, ConnectionsData{}))
		want := []any{
			map[string]any{
				"configMapRef": map[string]any{"name": envConfigsRes},
			},
			map[string]any{
				"secretRef": map[string]any{"name": envSecretsRes},
			},
		}
		if diff := cmp.Diff(want, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("configurations.toContainerVolumeMounts", func(t *testing.T) {
		got := eval(t, "configurations.toContainerVolumeMounts()", buildContext(t, workload, nil, ConnectionsData{}))
		want := []any{
			map[string]any{
				"name":      appYamlVolName,
				"mountPath": "/etc/config/app.yaml",
				"subPath":   "app.yaml",
			},
			map[string]any{
				"name":      loggingVolName,
				"mountPath": "/etc/config/logging.properties",
				"subPath":   "logging.properties",
			},
		}
		if diff := cmp.Diff(want, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("configurations.toVolumes", func(t *testing.T) {
		got := eval(t, "configurations.toVolumes()", buildContext(t, workload, nil, ConnectionsData{}))
		if diff := cmp.Diff(expectedConfigVolumes, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("configurations.toConfigEnvsByContainer", func(t *testing.T) {
		got := eval(t, "configurations.toConfigEnvsByContainer()", buildContext(t, workload, nil, ConnectionsData{}))
		want := []any{
			map[string]any{
				"resourceName": envConfigsRes,
				"envs": []any{
					map[string]any{"name": "LOG_LEVEL", "value": "info"},
					map[string]any{"name": "DEBUG", "value": "true"},
				},
			},
		}
		if diff := cmp.Diff(want, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("configurations.toSecretEnvsByContainer", func(t *testing.T) {
		got := eval(t, "configurations.toSecretEnvsByContainer()", buildContext(t, workloadWithSecrets, secretRefs, ConnectionsData{}))
		want := []any{
			map[string]any{
				"resourceName": envSecretsRes,
				"envs": []any{
					map[string]any{
						"name":  "DB_PASSWORD",
						"value": "",
						"remoteRef": map[string]any{
							"key":      "db/password",
							"property": "value",
						},
					},
				},
			},
		}
		if diff := cmp.Diff(want, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("workload.toServicePorts", func(t *testing.T) {
		got := eval(t, "workload.toServicePorts()", buildContext(t, workload, nil, ConnectionsData{}))
		want := []any{
			map[string]any{"name": "grpc", "port": float64(9090), "targetPort": float64(9090), "protocol": "TCP"},
			map[string]any{"name": "http", "port": float64(8080), "targetPort": float64(8080), "protocol": "TCP"},
		}
		if diff := cmp.Diff(want, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("dependencies.toContainerEnvs", func(t *testing.T) {
		got := eval(t, "dependencies.toContainerEnvs()", buildContext(t, workload, nil, depData))
		want := []any{
			map[string]any{"name": "SVC_A_URL", "value": "http://svc-a.ns1:8080"},
			map[string]any{"name": "SVC_A_HOST", "value": "svc-a.ns1"},
			map[string]any{"name": "DB_HOST", "value": "db.example.com"},
		}
		if diff := cmp.Diff(want, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("dependencies.toContainerVolumeMounts", func(t *testing.T) {
		got := eval(t, "dependencies.toContainerVolumeMounts()", buildContext(t, workload, nil, depData))
		want := []any{
			map[string]any{"name": "db-creds", "mountPath": "/etc/db", "subPath": "password"},
		}
		if diff := cmp.Diff(want, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("dependencies.toVolumes", func(t *testing.T) {
		got := eval(t, "dependencies.toVolumes()", buildContext(t, workload, nil, depData))
		want := []any{
			map[string]any{
				"name":   "db-creds",
				"secret": map[string]any{"secretName": "db-conn-secret"},
			},
		}
		if diff := cmp.Diff(want, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("volume_concat_configurations_plus_dependencies", func(t *testing.T) {
		got := eval(t, "configurations.toVolumes() + dependencies.toVolumes()", buildContext(t, workload, nil, depData))
		want := append(
			append([]any{}, expectedConfigVolumes...),
			map[string]any{
				"name":   "db-creds",
				"secret": map[string]any{"secretName": "db-conn-secret"},
			},
		)
		if diff := cmp.Diff(want, got, diffOpts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("all_macros_empty_inputs", func(t *testing.T) {
		ctxMap := buildContext(t, nil, nil, ConnectionsData{})
		macros := []string{
			"configurations.toConfigFileList()",
			"configurations.toSecretFileList()",
			"configurations.toContainerEnvFrom()",
			"configurations.toContainerVolumeMounts()",
			"configurations.toVolumes()",
			"configurations.toConfigEnvsByContainer()",
			"configurations.toSecretEnvsByContainer()",
			"workload.toServicePorts()",
			"dependencies.toContainerEnvs()",
			"dependencies.toContainerVolumeMounts()",
			"dependencies.toVolumes()",
		}
		for _, macro := range macros {
			t.Run(macro, func(t *testing.T) {
				got := eval(t, macro, ctxMap)
				if diff := cmp.Diff([]any{}, got, diffOpts...); diff != "" {
					t.Errorf("expected empty list, mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})
}
