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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractConfigurationsFromWorkload(tt.secretReferences, tt.workload)

			if diff := cmp.Diff(tt.want, got, sortSliceByName()); diff != "" {
				t.Errorf("extractConfigurationsFromWorkload() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestApplyConfigurationOverrides(t *testing.T) {
	tests := []struct {
		name               string
		secretReferences   map[string]*v1alpha1.SecretReference
		baseConfigurations map[string]any
		overrides          *v1alpha1.EnvConfigurationOverrides
		want               map[string]any
	}{
		{
			name:             "override existing config env value",
			secretReferences: map[string]*v1alpha1.SecretReference{},
			baseConfigurations: map[string]any{
				"configs": map[string]any{
					"envs":  []any{map[string]any{"name": "DATABASE_URL", "value": "postgres://localhost:5432"}},
					"files": []any{},
				},
				"secrets": map[string]any{
					"envs":  []any{},
					"files": []any{},
				},
			},
			overrides: &v1alpha1.EnvConfigurationOverrides{
				Env: []v1alpha1.EnvVar{{Key: "DATABASE_URL", Value: "postgres://prod-server:5432"}},
			},
			want: map[string]any{
				"configs": map[string]any{
					"envs": []any{
						map[string]any{"name": "DATABASE_URL", "value": "postgres://prod-server:5432"},
					},
					"files": []any{},
				},
				"secrets": map[string]any{
					"envs":  []any{},
					"files": []any{},
				},
			},
		},
		{
			name:             "add new config env value",
			secretReferences: map[string]*v1alpha1.SecretReference{},
			baseConfigurations: map[string]any{
				"configs": map[string]any{
					"envs":  []any{map[string]any{"name": "EXISTING_VAR", "value": "existing"}},
					"files": []any{},
				},
				"secrets": map[string]any{
					"envs":  []any{},
					"files": []any{},
				},
			},
			overrides: &v1alpha1.EnvConfigurationOverrides{
				Env: []v1alpha1.EnvVar{{Key: "NEW_VAR", Value: "new-value"}},
			},
			want: map[string]any{
				"configs": map[string]any{
					"envs": []any{
						map[string]any{"name": "EXISTING_VAR", "value": "existing"},
						map[string]any{"name": "NEW_VAR", "value": "new-value"},
					},
					"files": []any{},
				},
				"secrets": map[string]any{
					"envs":  []any{},
					"files": []any{},
				},
			},
		},
		{
			name:             "override file config value",
			secretReferences: map[string]*v1alpha1.SecretReference{},
			baseConfigurations: map[string]any{
				"configs": map[string]any{
					"envs": []any{},
					"files": []any{
						map[string]any{"name": "app-config", "mountPath": "/etc/app/config.yaml", "value": "old: value"},
					},
				},
				"secrets": map[string]any{
					"envs":  []any{},
					"files": []any{},
				},
			},
			overrides: &v1alpha1.EnvConfigurationOverrides{
				Files: []v1alpha1.FileVar{
					{Key: "app-config", MountPath: "/etc/app/config.yaml", Value: "new: value"},
				},
			},
			want: map[string]any{
				"configs": map[string]any{
					"envs": []any{},
					"files": []any{
						map[string]any{"name": "app-config", "mountPath": "/etc/app/config.yaml", "value": "new: value"},
					},
				},
				"secrets": map[string]any{
					"envs":  []any{},
					"files": []any{},
				},
			},
		},
		{
			name: "add new file with secret reference",
			secretReferences: map[string]*v1alpha1.SecretReference{
				"tls-secret": {
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
			baseConfigurations: map[string]any{
				"configs": map[string]any{
					"envs":  []any{},
					"files": []any{},
				},
				"secrets": map[string]any{
					"envs":  []any{},
					"files": []any{},
				},
			},
			overrides: &v1alpha1.EnvConfigurationOverrides{
				Files: []v1alpha1.FileVar{
					{
						Key:       "tls-cert",
						MountPath: "/etc/tls/tls.crt",
						ValueFrom: &v1alpha1.EnvVarValueFrom{
							SecretRef: &v1alpha1.SecretKeyRef{
								Name: "tls-secret",
								Key:  "tls.crt",
							},
						},
					},
				},
			},
			want: map[string]any{
				"configs": map[string]any{
					"envs":  []any{},
					"files": []any{},
				},
				"secrets": map[string]any{
					"envs": []any{},
					"files": []any{
						map[string]any{
							"name":      "tls-cert",
							"mountPath": "/etc/tls/tls.crt",
							"remoteRef": map[string]any{
								"key": "secret/data/tls/cert",
							},
						},
					},
				},
			},
		},
		{
			name: "mixed overrides with multiple envs, secrets, and files",
			secretReferences: map[string]*v1alpha1.SecretReference{
				"api-credentials": {
					Spec: v1alpha1.SecretReferenceSpec{
						Data: []v1alpha1.SecretDataSource{
							{
								SecretKey: "api-token",
								RemoteRef: v1alpha1.RemoteReference{
									Key:      "secret/data/api",
									Property: "token",
								},
							},
							{
								SecretKey: "api-key",
								RemoteRef: v1alpha1.RemoteReference{
									Key:      "secret/data/api",
									Property: "key",
								},
							},
						},
					},
				},
				"db-credentials": {
					Spec: v1alpha1.SecretReferenceSpec{
						Data: []v1alpha1.SecretDataSource{
							{
								SecretKey: "password",
								RemoteRef: v1alpha1.RemoteReference{
									Key: "secret/data/db/password",
								},
							},
						},
					},
				},
				"tls-certificates": {
					Spec: v1alpha1.SecretReferenceSpec{
						Data: []v1alpha1.SecretDataSource{
							{
								SecretKey: "server-cert",
								RemoteRef: v1alpha1.RemoteReference{
									Key: "secret/data/tls/server",
								},
							},
							{
								SecretKey: "client-cert",
								RemoteRef: v1alpha1.RemoteReference{
									Key: "secret/data/tls/client",
								},
							},
						},
					},
				},
			},
			baseConfigurations: map[string]any{
				"configs": map[string]any{
					"envs": []any{
						map[string]any{"name": "LOG_LEVEL", "value": "info"},
						map[string]any{"name": "APP_NAME", "value": "old-app"},
						map[string]any{"name": "PORT", "value": "3000"},
					},
					"files": []any{
						map[string]any{"name": "app-config", "mountPath": "/etc/app/config.yaml", "value": "version: 1.0"},
						map[string]any{"name": "readme", "mountPath": "/etc/readme.txt", "value": "old readme"},
					},
				},
				"secrets": map[string]any{
					"envs": []any{
						map[string]any{
							"name": "OLD_SECRET",
							"remoteRef": map[string]any{
								"key": "secret/data/old",
							},
						},
					},
					"files": []any{
						map[string]any{
							"name":      "old-cert",
							"mountPath": "/etc/certs/old.crt",
							"remoteRef": map[string]any{
								"key": "secret/data/old-cert",
							},
						},
					},
				},
			},
			overrides: &v1alpha1.EnvConfigurationOverrides{
				Env: []v1alpha1.EnvVar{
					{Key: "LOG_LEVEL", Value: "debug"},
					{Key: "NEW_ENV", Value: "new-value"},
					{
						Key: "API_TOKEN",
						ValueFrom: &v1alpha1.EnvVarValueFrom{
							SecretRef: &v1alpha1.SecretKeyRef{
								Name: "api-credentials",
								Key:  "api-token",
							},
						},
					},
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
					{Key: "app-config", MountPath: "/etc/app/config.yaml", Value: "version: 2.0"},
					{Key: "new-config", MountPath: "/etc/app/new.yaml", Value: "feature: enabled"},
					{
						Key:       "api-key-file",
						MountPath: "/etc/secrets/api-key",
						ValueFrom: &v1alpha1.EnvVarValueFrom{
							SecretRef: &v1alpha1.SecretKeyRef{
								Name: "api-credentials",
								Key:  "api-key",
							},
						},
					},
					{
						Key:       "old-cert",
						MountPath: "/etc/certs/old.crt",
						ValueFrom: &v1alpha1.EnvVarValueFrom{
							SecretRef: &v1alpha1.SecretKeyRef{
								Name: "tls-certificates",
								Key:  "server-cert",
							},
						},
					},
					{
						Key:       "client-cert",
						MountPath: "/etc/certs/client.crt",
						ValueFrom: &v1alpha1.EnvVarValueFrom{
							SecretRef: &v1alpha1.SecretKeyRef{
								Name: "tls-certificates",
								Key:  "client-cert",
							},
						},
					},
				},
			},
			want: map[string]any{
				"configs": map[string]any{
					"envs": []any{
						map[string]any{"name": "APP_NAME", "value": "old-app"},
						map[string]any{"name": "LOG_LEVEL", "value": "debug"},
						map[string]any{"name": "NEW_ENV", "value": "new-value"},
						map[string]any{"name": "PORT", "value": "3000"},
					},
					"files": []any{
						map[string]any{"name": "app-config", "mountPath": "/etc/app/config.yaml", "value": "version: 2.0"},
						map[string]any{"name": "new-config", "mountPath": "/etc/app/new.yaml", "value": "feature: enabled"},
						map[string]any{"name": "readme", "mountPath": "/etc/readme.txt", "value": "old readme"},
					},
				},
				"secrets": map[string]any{
					"envs": []any{
						map[string]any{
							"name": "API_TOKEN",
							"remoteRef": map[string]any{
								"key":      "secret/data/api",
								"property": "token",
							},
						},
						map[string]any{
							"name": "DB_PASSWORD",
							"remoteRef": map[string]any{
								"key": "secret/data/db/password",
							},
						},
						map[string]any{
							"name": "OLD_SECRET",
							"remoteRef": map[string]any{
								"key": "secret/data/old",
							},
						},
					},
					"files": []any{
						map[string]any{
							"name":      "api-key-file",
							"mountPath": "/etc/secrets/api-key",
							"remoteRef": map[string]any{
								"key":      "secret/data/api",
								"property": "key",
							},
						},
						map[string]any{
							"name":      "client-cert",
							"mountPath": "/etc/certs/client.crt",
							"remoteRef": map[string]any{
								"key": "secret/data/tls/client",
							},
						},
						map[string]any{
							"name":      "old-cert",
							"mountPath": "/etc/certs/old.crt",
							"remoteRef": map[string]any{
								"key": "secret/data/tls/server",
							},
						},
					},
				},
			},
		},
		{
			name:             "empty overrides returns base unchanged",
			secretReferences: map[string]*v1alpha1.SecretReference{},
			baseConfigurations: map[string]any{
				"configs": map[string]any{
					"envs":  []any{map[string]any{"name": "EXISTING", "value": "value"}},
					"files": []any{},
				},
				"secrets": map[string]any{
					"envs":  []any{},
					"files": []any{},
				},
			},
			overrides: &v1alpha1.EnvConfigurationOverrides{
				Env:   []v1alpha1.EnvVar{},
				Files: []v1alpha1.FileVar{},
			},
			want: map[string]any{
				"configs": map[string]any{
					"envs": []any{
						map[string]any{"name": "EXISTING", "value": "value"},
					},
					"files": []any{},
				},
				"secrets": map[string]any{
					"envs":  []any{},
					"files": []any{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyConfigurationOverrides(tt.secretReferences, tt.baseConfigurations, tt.overrides)

			if diff := cmp.Diff(tt.want, got, sortSliceByName()); diff != "" {
				t.Errorf("applyConfigurationOverrides() mismatch (-want +got):\n%s", diff)
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
