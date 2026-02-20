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
		want             ContainerConfigurations
	}{
		{
			name:             "workload with no configurations",
			secretReferences: map[string]*v1alpha1.SecretReference{},
			workload: &v1alpha1.Workload{
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:latest",
						},
					},
				},
			},
			want: ContainerConfigurations{
				Configs: ConfigurationItems{
					Envs:  []EnvConfiguration{},
					Files: []FileConfiguration{},
				},
				Secrets: ConfigurationItems{
					Envs:  []EnvConfiguration{},
					Files: []FileConfiguration{},
				},
			},
		},
		{
			name:             "workload with direct env values",
			secretReferences: map[string]*v1alpha1.SecretReference{},
			workload: &v1alpha1.Workload{
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:latest",
							Env: []v1alpha1.EnvVar{
								{Key: "DATABASE_URL", Value: "postgres://localhost:5432"},
								{Key: "LOG_LEVEL", Value: "debug"},
							},
						},
					},
				},
			},
			want: ContainerConfigurations{
				Configs: ConfigurationItems{
					Envs: []EnvConfiguration{
						{Name: "DATABASE_URL", Value: "postgres://localhost:5432"},
						{Name: "LOG_LEVEL", Value: "debug"},
					},
					Files: []FileConfiguration{},
				},
				Secrets: ConfigurationItems{
					Envs:  []EnvConfiguration{},
					Files: []FileConfiguration{},
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
						Container: v1alpha1.Container{
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
			want: ContainerConfigurations{
				Configs: ConfigurationItems{
					Envs:  []EnvConfiguration{},
					Files: []FileConfiguration{},
				},
				Secrets: ConfigurationItems{
					Envs: []EnvConfiguration{
						{
							Name: "DB_PASSWORD",
							RemoteRef: &RemoteRefData{
								Key:      "secret/data/db",
								Property: "password",
							},
						},
					},
					Files: []FileConfiguration{},
				},
			},
		},
		{
			name:             "workload with files as configs",
			secretReferences: map[string]*v1alpha1.SecretReference{},
			workload: &v1alpha1.Workload{
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
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
			want: ContainerConfigurations{
				Configs: ConfigurationItems{
					Envs: []EnvConfiguration{},
					Files: []FileConfiguration{
						{
							Name:      "app-config",
							MountPath: "/etc/app/config.yaml",
							Value:     "key: value",
						},
					},
				},
				Secrets: ConfigurationItems{
					Envs:  []EnvConfiguration{},
					Files: []FileConfiguration{},
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
						Container: v1alpha1.Container{
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
			want: ContainerConfigurations{
				Configs: ConfigurationItems{
					Envs:  []EnvConfiguration{},
					Files: []FileConfiguration{},
				},
				Secrets: ConfigurationItems{
					Envs: []EnvConfiguration{},
					Files: []FileConfiguration{
						{
							Name:      "tls-certificate",
							MountPath: "/etc/tls/tls.crt",
							RemoteRef: &RemoteRefData{
								Key: "secret/data/tls/cert",
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
						Container: v1alpha1.Container{
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
			want: ContainerConfigurations{
				Configs: ConfigurationItems{
					Envs: []EnvConfiguration{
						{Name: "APP_NAME", Value: "my-app"},
						{Name: "LOG_LEVEL", Value: "info"},
						{Name: "PORT", Value: "8080"},
					},
					Files: []FileConfiguration{
						{Name: "app-config", MountPath: "/etc/app/config.yaml", Value: "server:\n  port: 8080"},
						{Name: "logging-config", MountPath: "/etc/app/logging.yaml", Value: "level: info"},
					},
				},
				Secrets: ConfigurationItems{
					Envs: []EnvConfiguration{
						{
							Name: "API_KEY",
							RemoteRef: &RemoteRefData{
								Key:      "secret/data/api",
								Property: "key",
							},
						},
						{
							Name: "API_TOKEN",
							RemoteRef: &RemoteRefData{
								Key:      "secret/data/api",
								Property: "token",
							},
						},
					},
					Files: []FileConfiguration{
						{
							Name:      "tls-certificate",
							MountPath: "/etc/tls/tls.crt",
							RemoteRef: &RemoteRefData{
								Key: "secret/data/tls/certificate",
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
						Container: v1alpha1.Container{
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
			want: ContainerConfigurations{
				Configs: ConfigurationItems{
					Envs:  []EnvConfiguration{},
					Files: []FileConfiguration{},
				},
				Secrets: ConfigurationItems{
					Envs:  []EnvConfiguration{},
					Files: []FileConfiguration{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractConfigurationsFromWorkload(tt.secretReferences, tt.workload)

			if diff := cmp.Diff(tt.want, got, sortSliceByName()); diff != "" {
				t.Errorf("ExtractConfigurationsFromWorkload() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// sortSliceByName is used to sort slices for comparison, ignoring order
func sortSliceByName() cmp.Option {
	return cmp.Options{
		cmpopts.SortSlices(func(a, b EnvConfiguration) bool {
			return a.Name < b.Name
		}),
		cmpopts.SortSlices(func(a, b FileConfiguration) bool {
			return a.Name < b.Name
		}),
	}
}
