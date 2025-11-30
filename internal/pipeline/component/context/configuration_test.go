// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestExtractConfigurationsFromWorkload(t *testing.T) {
	tests := []struct {
		name             string
		secretReferences map[string]*v1alpha1.SecretReference
		workload         *v1alpha1.Workload
		want             map[string]any
	}{
		{
			name:             "workload with no configurations",
			secretReferences: map[string]*v1alpha1.SecretReference{},
			workload: &v1alpha1.Workload{
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Containers: map[string]v1alpha1.Container{
							"main": {
								Image: "test:latest",
							},
						},
					},
				},
			},
			want: map[string]any{
				"main": map[string]any{
					"configs": map[string]any{
						"envs":  []any{},
						"files": []any{},
					},
					"secrets": map[string]any{
						"envs":  []any{},
						"files": []any{},
					},
				},
			},
		},
		{
			name:             "workload with direct env values",
			secretReferences: map[string]*v1alpha1.SecretReference{},
			workload: &v1alpha1.Workload{
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Containers: map[string]v1alpha1.Container{
							"main": {
								Image: "test:latest",
								Env: []v1alpha1.EnvVar{
									{Key: "DATABASE_URL", Value: "postgres://localhost:5432"},
									{Key: "LOG_LEVEL", Value: "debug"},
								},
							},
						},
					},
				},
			},
			want: map[string]any{
				"main": map[string]any{
					"configs": map[string]any{
						"envs": []any{
							map[string]any{"name": "DATABASE_URL", "value": "postgres://localhost:5432"},
							map[string]any{"name": "LOG_LEVEL", "value": "debug"},
						},
						"files": []any{},
					},
					"secrets": map[string]any{
						"envs":  []any{},
						"files": []any{},
					},
				},
			},
		},
		{
			name: "workload with secret references",
			secretReferences: map[string]*v1alpha1.SecretReference{
				"db-credentials": {
					Spec: v1alpha1.SecretReferenceSpec{
						Data: []v1alpha1.SecretDataSource{
							{
								SecretKey: "password",
								RemoteRef: v1alpha1.RemoteReference{
									Key:      "secret/data/db",
									Property: "password",
								},
							},
						},
					},
				},
			},
			workload: &v1alpha1.Workload{
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Containers: map[string]v1alpha1.Container{
							"main": {
								Image: "test:latest",
								Env: []v1alpha1.EnvVar{
									{
										Key: "DB_PASSWORD",
										ValueFrom: &v1alpha1.EnvVarValueFrom{
											SecretRef: &v1alpha1.SecretKeyRef{
												Name: "db-credentials",
												Key:  "password",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: map[string]any{
				"main": map[string]any{
					"configs": map[string]any{
						"envs":  []any{},
						"files": []any{},
					},
					"secrets": map[string]any{
						"envs": []any{
							map[string]any{
								"name": "DB_PASSWORD",
								"remoteRef": map[string]any{
									"key":      "secret/data/db",
									"property": "password",
								},
							},
						},
						"files": []any{},
					},
				},
			},
		},
		{
			name:             "workload with files as configs",
			secretReferences: map[string]*v1alpha1.SecretReference{},
			workload: &v1alpha1.Workload{
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Containers: map[string]v1alpha1.Container{
							"main": {
								Image: "test:latest",
								Files: []v1alpha1.FileVar{
									{
										Key:       "app-config",
										MountPath: "/etc/app/config.yaml",
										Value:     "key: value",
									},
								},
							},
						},
					},
				},
			},
			want: map[string]any{
				"main": map[string]any{
					"configs": map[string]any{
						"envs": []any{},
						"files": []any{
							map[string]any{
								"name":      "app-config",
								"mountPath": "/etc/app/config.yaml",
								"value":     "key: value",
							},
						},
					},
					"secrets": map[string]any{
						"envs":  []any{},
						"files": []any{},
					},
				},
			},
		},
		{
			name: "workload with file secret references",
			secretReferences: map[string]*v1alpha1.SecretReference{
				"tls-cert": {
					Spec: v1alpha1.SecretReferenceSpec{
						Data: []v1alpha1.SecretDataSource{
							{
								SecretKey: "tls.crt",
								RemoteRef: v1alpha1.RemoteReference{
									Key: "secret/data/tls/cert",
								},
							},
						},
					},
				},
			},
			workload: &v1alpha1.Workload{
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Containers: map[string]v1alpha1.Container{
							"main": {
								Image: "test:latest",
								Files: []v1alpha1.FileVar{
									{
										Key:       "tls-certificate",
										MountPath: "/etc/tls/tls.crt",
										ValueFrom: &v1alpha1.EnvVarValueFrom{
											SecretRef: &v1alpha1.SecretKeyRef{
												Name: "tls-cert",
												Key:  "tls.crt",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: map[string]any{
				"main": map[string]any{
					"configs": map[string]any{
						"envs":  []any{},
						"files": []any{},
					},
					"secrets": map[string]any{
						"envs": []any{},
						"files": []any{
							map[string]any{
								"name":      "tls-certificate",
								"mountPath": "/etc/tls/tls.crt",
								"remoteRef": map[string]any{
									"key": "secret/data/tls/cert",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "workload with multiple envs and files and secrets",
			secretReferences: map[string]*v1alpha1.SecretReference{
				"api-credentials": {
					Spec: v1alpha1.SecretReferenceSpec{
						Data: []v1alpha1.SecretDataSource{
							{
								SecretKey: "api-key",
								RemoteRef: v1alpha1.RemoteReference{
									Key:      "secret/data/api",
									Property: "key",
								},
							},
							{
								SecretKey: "api-token",
								RemoteRef: v1alpha1.RemoteReference{
									Key:      "secret/data/api",
									Property: "token",
								},
							},
						},
					},
				},
				"tls-cert": {
					Spec: v1alpha1.SecretReferenceSpec{
						Data: []v1alpha1.SecretDataSource{
							{
								SecretKey: "cert",
								RemoteRef: v1alpha1.RemoteReference{
									Key: "secret/data/tls/certificate",
								},
							},
						},
					},
				},
			},
			workload: &v1alpha1.Workload{
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Containers: map[string]v1alpha1.Container{
							"main": {
								Image: "test:latest",
								Env: []v1alpha1.EnvVar{
									{Key: "LOG_LEVEL", Value: "info"},
									{Key: "APP_NAME", Value: "my-app"},
									{Key: "PORT", Value: "8080"},
									{
										Key: "API_KEY",
										ValueFrom: &v1alpha1.EnvVarValueFrom{
											SecretRef: &v1alpha1.SecretKeyRef{
												Name: "api-credentials",
												Key:  "api-key",
											},
										},
									},
									{
										Key: "API_TOKEN",
										ValueFrom: &v1alpha1.EnvVarValueFrom{
											SecretRef: &v1alpha1.SecretKeyRef{
												Name: "api-credentials",
												Key:  "api-token",
											},
										},
									},
								},
								Files: []v1alpha1.FileVar{
									{Key: "app-config", MountPath: "/etc/app/config.yaml", Value: "server:\n  port: 8080"},
									{Key: "logging-config", MountPath: "/etc/app/logging.yaml", Value: "level: info"},
									{
										Key:       "tls-certificate",
										MountPath: "/etc/tls/tls.crt",
										ValueFrom: &v1alpha1.EnvVarValueFrom{
											SecretRef: &v1alpha1.SecretKeyRef{
												Name: "tls-cert",
												Key:  "cert",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: map[string]any{
				"main": map[string]any{
					"configs": map[string]any{
						"envs": []any{
							map[string]any{"name": "APP_NAME", "value": "my-app"},
							map[string]any{"name": "LOG_LEVEL", "value": "info"},
							map[string]any{"name": "PORT", "value": "8080"},
						},
						"files": []any{
							map[string]any{"name": "app-config", "mountPath": "/etc/app/config.yaml", "value": "server:\n  port: 8080"},
							map[string]any{"name": "logging-config", "mountPath": "/etc/app/logging.yaml", "value": "level: info"},
						},
					},
					"secrets": map[string]any{
						"envs": []any{
							map[string]any{
								"name": "API_KEY",
								"remoteRef": map[string]any{
									"key":      "secret/data/api",
									"property": "key",
								},
							},
							map[string]any{
								"name": "API_TOKEN",
								"remoteRef": map[string]any{
									"key":      "secret/data/api",
									"property": "token",
								},
							},
						},
						"files": []any{
							map[string]any{
								"name":      "tls-certificate",
								"mountPath": "/etc/tls/tls.crt",
								"remoteRef": map[string]any{
									"key": "secret/data/tls/certificate",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "unresolved secret reference is skipped",
			secretReferences: map[string]*v1alpha1.SecretReference{
				"existing-secret": {
					Spec: v1alpha1.SecretReferenceSpec{
						Data: []v1alpha1.SecretDataSource{
							{
								SecretKey: "key1",
								RemoteRef: v1alpha1.RemoteReference{
									Key: "secret/data/key1",
								},
							},
						},
					},
				},
			},
			workload: &v1alpha1.Workload{
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Containers: map[string]v1alpha1.Container{
							"main": {
								Image: "test:latest",
								Env: []v1alpha1.EnvVar{
									{
										Key: "MISSING_SECRET",
										ValueFrom: &v1alpha1.EnvVarValueFrom{
											SecretRef: &v1alpha1.SecretKeyRef{
												Name: "non-existent-secret",
												Key:  "key",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: map[string]any{
				"main": map[string]any{
					"configs": map[string]any{
						"envs":  []any{},
						"files": []any{},
					},
					"secrets": map[string]any{
						"envs":  []any{},
						"files": []any{},
					},
				},
			},
		},
		{
			name: "workload with multiple containers",
			secretReferences: map[string]*v1alpha1.SecretReference{
				"db-credentials": {
					Spec: v1alpha1.SecretReferenceSpec{
						Data: []v1alpha1.SecretDataSource{
							{
								SecretKey: "password",
								RemoteRef: v1alpha1.RemoteReference{
									Key:      "secret/data/db",
									Property: "password",
								},
							},
						},
					},
				},
				"cache-credentials": {
					Spec: v1alpha1.SecretReferenceSpec{
						Data: []v1alpha1.SecretDataSource{
							{
								SecretKey: "password",
								RemoteRef: v1alpha1.RemoteReference{
									Key:      "secret/data/cache",
									Property: "password",
								},
							},
						},
					},
				},
			},
			workload: &v1alpha1.Workload{
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Containers: map[string]v1alpha1.Container{
							"app": {
								Image: "app:latest",
								Env: []v1alpha1.EnvVar{
									{Key: "APP_NAME", Value: "my-app"},
									{Key: "PORT", Value: "8080"},
									{
										Key: "DB_PASSWORD",
										ValueFrom: &v1alpha1.EnvVarValueFrom{
											SecretRef: &v1alpha1.SecretKeyRef{
												Name: "db-credentials",
												Key:  "password",
											},
										},
									},
								},
								Files: []v1alpha1.FileVar{
									{Key: "app-config", MountPath: "/etc/app/config.yaml", Value: "server:\n  port: 8080"},
								},
							},
							"sidecar": {
								Image: "sidecar:latest",
								Env: []v1alpha1.EnvVar{
									{Key: "CACHE_HOST", Value: "localhost"},
									{
										Key: "CACHE_PASSWORD",
										ValueFrom: &v1alpha1.EnvVarValueFrom{
											SecretRef: &v1alpha1.SecretKeyRef{
												Name: "cache-credentials",
												Key:  "password",
											},
										},
									},
								},
								Files: []v1alpha1.FileVar{
									{Key: "sidecar-config", MountPath: "/etc/sidecar/config.yaml", Value: "cache:\n  enabled: true"},
								},
							},
						},
					},
				},
			},
			want: map[string]any{
				"app": map[string]any{
					"configs": map[string]any{
						"envs": []any{
							map[string]any{"name": "APP_NAME", "value": "my-app"},
							map[string]any{"name": "PORT", "value": "8080"},
						},
						"files": []any{
							map[string]any{"name": "app-config", "mountPath": "/etc/app/config.yaml", "value": "server:\n  port: 8080"},
						},
					},
					"secrets": map[string]any{
						"envs": []any{
							map[string]any{
								"name": "DB_PASSWORD",
								"remoteRef": map[string]any{
									"key":      "secret/data/db",
									"property": "password",
								},
							},
						},
						"files": []any{},
					},
				},
				"sidecar": map[string]any{
					"configs": map[string]any{
						"envs": []any{
							map[string]any{"name": "CACHE_HOST", "value": "localhost"},
						},
						"files": []any{
							map[string]any{"name": "sidecar-config", "mountPath": "/etc/sidecar/config.yaml", "value": "cache:\n  enabled: true"},
						},
					},
					"secrets": map[string]any{
						"envs": []any{
							map[string]any{
								"name": "CACHE_PASSWORD",
								"remoteRef": map[string]any{
									"key":      "secret/data/cache",
									"property": "password",
								},
							},
						},
						"files": []any{},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractConfigurationsFromWorkload(tt.secretReferences, tt.workload)

			// Convert to map[string]any for comparison with expected values
			gotAny, err := structToMap(got)
			if err != nil {
				t.Fatalf("failed to convert result to map: %v", err)
			}

			if diff := cmp.Diff(tt.want, gotAny, sortSliceByName()); diff != "" {
				t.Errorf("extractConfigurationsFromWorkload() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// sortSliceByName is used to sort slices for comparison, ignoring order
func sortSliceByName() cmp.Option {
	return cmpopts.SortSlices(func(a, b any) bool {
		aMap, aOk := a.(map[string]any)
		bMap, bOk := b.(map[string]any)
		if !aOk || !bOk {
			return false
		}
		aName, aOk := aMap["name"].(string)
		bName, bOk := bMap["name"].(string)
		if !aOk || !bOk {
			return false
		}
		return aName < bName
	})
}
