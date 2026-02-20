// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestMergeWorkloadOverrides(t *testing.T) {
	tests := []struct {
		name         string
		baseWorkload *v1alpha1.Workload
		overrides    *v1alpha1.WorkloadOverrideTemplateSpec
		want         *v1alpha1.Workload
	}{
		{
			name: "nil overrides",
			baseWorkload: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "ENV1", Value: "value1"},
							},
						},
					},
				},
			},
			overrides: nil,
			want: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "ENV1", Value: "value1"},
							},
						},
					},
				},
			},
		},
		{
			name: "nil container override",
			baseWorkload: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "ENV1", Value: "value1"},
							},
						},
					},
				},
			},
			overrides: &v1alpha1.WorkloadOverrideTemplateSpec{
				Container: nil,
			},
			want: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "ENV1", Value: "value1"},
							},
						},
					},
				},
			},
		},
		{
			name: "override env var",
			baseWorkload: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "ENV1", Value: "value1"},
								{Key: "ENV2", Value: "value2"},
							},
						},
					},
				},
			},
			overrides: &v1alpha1.WorkloadOverrideTemplateSpec{
				Container: &v1alpha1.ContainerOverride{
					Env: []v1alpha1.EnvVar{
						{Key: "ENV1", Value: "overridden_value1"},
					},
				},
			},
			want: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "ENV1", Value: "overridden_value1"},
								{Key: "ENV2", Value: "value2"},
							},
						},
					},
				},
			},
		},
		{
			name: "add new env var",
			baseWorkload: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "ENV1", Value: "value1"},
							},
						},
					},
				},
			},
			overrides: &v1alpha1.WorkloadOverrideTemplateSpec{
				Container: &v1alpha1.ContainerOverride{
					Env: []v1alpha1.EnvVar{
						{Key: "ENV3", Value: "value3"},
					},
				},
			},
			want: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "ENV1", Value: "value1"},
								{Key: "ENV3", Value: "value3"},
							},
						},
					},
				},
			},
		},
		{
			name: "override file var",
			baseWorkload: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Files: []v1alpha1.FileVar{
								{Key: "config.yaml", MountPath: "/etc/config/config.yaml", Value: "base content"},
							},
						},
					},
				},
			},
			overrides: &v1alpha1.WorkloadOverrideTemplateSpec{
				Container: &v1alpha1.ContainerOverride{
					Files: []v1alpha1.FileVar{
						{Key: "config.yaml", MountPath: "/etc/config/config.yaml", Value: "overridden content"},
					},
				},
			},
			want: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Files: []v1alpha1.FileVar{
								{Key: "config.yaml", MountPath: "/etc/config/config.yaml", Value: "overridden content"},
							},
						},
					},
				},
			},
		},
		{
			name: "add new file var",
			baseWorkload: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Files: []v1alpha1.FileVar{
								{Key: "config.yaml", MountPath: "/etc/config/config.yaml", Value: "content"},
							},
						},
					},
				},
			},
			overrides: &v1alpha1.WorkloadOverrideTemplateSpec{
				Container: &v1alpha1.ContainerOverride{
					Files: []v1alpha1.FileVar{
						{Key: "secrets.yaml", MountPath: "/etc/secrets/secrets.yaml", Value: "secret content"},
					},
				},
			},
			want: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Files: []v1alpha1.FileVar{
								{Key: "config.yaml", MountPath: "/etc/config/config.yaml", Value: "content"},
								{Key: "secrets.yaml", MountPath: "/etc/secrets/secrets.yaml", Value: "secret content"},
							},
						},
					},
				},
			},
		},
		{
			name: "override env with secret reference",
			baseWorkload: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "DATABASE_URL", Value: "localhost:5432"},
								{Key: "API_KEY", ValueFrom: &v1alpha1.EnvVarValueFrom{
									SecretRef: &v1alpha1.SecretKeyRef{
										Name: "dev-secrets",
										Key:  "api-key",
									},
								}},
							},
						},
					},
				},
			},
			overrides: &v1alpha1.WorkloadOverrideTemplateSpec{
				Container: &v1alpha1.ContainerOverride{
					Env: []v1alpha1.EnvVar{
						{Key: "API_KEY", ValueFrom: &v1alpha1.EnvVarValueFrom{
							SecretRef: &v1alpha1.SecretKeyRef{
								Name: "prod-secrets",
								Key:  "api-key",
							},
						}},
					},
				},
			},
			want: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "DATABASE_URL", Value: "localhost:5432"},
								{Key: "API_KEY", ValueFrom: &v1alpha1.EnvVarValueFrom{
									SecretRef: &v1alpha1.SecretKeyRef{
										Name: "prod-secrets",
										Key:  "api-key",
									},
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "override file with secret reference",
			baseWorkload: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Files: []v1alpha1.FileVar{
								{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "dev config"},
								{Key: "credentials.json", MountPath: "/etc/credentials.json", ValueFrom: &v1alpha1.EnvVarValueFrom{
									SecretRef: &v1alpha1.SecretKeyRef{
										Name: "dev-creds",
										Key:  "credentials",
									},
								}},
							},
						},
					},
				},
			},
			overrides: &v1alpha1.WorkloadOverrideTemplateSpec{
				Container: &v1alpha1.ContainerOverride{
					Files: []v1alpha1.FileVar{
						{Key: "credentials.json", MountPath: "/etc/credentials.json", ValueFrom: &v1alpha1.EnvVarValueFrom{
							SecretRef: &v1alpha1.SecretKeyRef{
								Name: "prod-creds",
								Key:  "credentials",
							},
						}},
					},
				},
			},
			want: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Files: []v1alpha1.FileVar{
								{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "dev config"},
								{Key: "credentials.json", MountPath: "/etc/credentials.json", ValueFrom: &v1alpha1.EnvVarValueFrom{
									SecretRef: &v1alpha1.SecretKeyRef{
										Name: "prod-creds",
										Key:  "credentials",
									},
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "mix literal values and secret references",
			baseWorkload: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "ENV1", Value: "value1"},
								{Key: "SECRET1", ValueFrom: &v1alpha1.EnvVarValueFrom{
									SecretRef: &v1alpha1.SecretKeyRef{
										Name: "dev-secrets",
										Key:  "secret1",
									},
								}},
							},
							Files: []v1alpha1.FileVar{
								{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "dev config"},
								{Key: "secret.key", MountPath: "/etc/secret.key", ValueFrom: &v1alpha1.EnvVarValueFrom{
									SecretRef: &v1alpha1.SecretKeyRef{
										Name: "dev-keys",
										Key:  "private-key",
									},
								}},
							},
						},
					},
				},
			},
			overrides: &v1alpha1.WorkloadOverrideTemplateSpec{
				Container: &v1alpha1.ContainerOverride{
					Env: []v1alpha1.EnvVar{
						{Key: "ENV1", Value: "overridden_value1"},
						{Key: "SECRET1", ValueFrom: &v1alpha1.EnvVarValueFrom{
							SecretRef: &v1alpha1.SecretKeyRef{
								Name: "prod-secrets",
								Key:  "secret1",
							},
						}},
						{Key: "ENV2", Value: "value2"},
					},
					Files: []v1alpha1.FileVar{
						{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "prod config"},
						{Key: "secret.key", MountPath: "/etc/secret.key", ValueFrom: &v1alpha1.EnvVarValueFrom{
							SecretRef: &v1alpha1.SecretKeyRef{
								Name: "prod-keys",
								Key:  "private-key",
							},
						}},
					},
				},
			},
			want: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "ENV1", Value: "overridden_value1"},
								{Key: "SECRET1", ValueFrom: &v1alpha1.EnvVarValueFrom{
									SecretRef: &v1alpha1.SecretKeyRef{
										Name: "prod-secrets",
										Key:  "secret1",
									},
								}},
								{Key: "ENV2", Value: "value2"},
							},
							Files: []v1alpha1.FileVar{
								{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "prod config"},
								{Key: "secret.key", MountPath: "/etc/secret.key", ValueFrom: &v1alpha1.EnvVarValueFrom{
									SecretRef: &v1alpha1.SecretKeyRef{
										Name: "prod-keys",
										Key:  "private-key",
									},
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "override both env and file vars",
			baseWorkload: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "ENV1", Value: "value1"},
								{Key: "ENV2", Value: "value2"},
							},
							Files: []v1alpha1.FileVar{
								{Key: "config.yaml", MountPath: "/etc/config/config.yaml", Value: "base content"},
							},
						},
					},
				},
			},
			overrides: &v1alpha1.WorkloadOverrideTemplateSpec{
				Container: &v1alpha1.ContainerOverride{
					Env: []v1alpha1.EnvVar{
						{Key: "ENV1", Value: "overridden_value1"},
						{Key: "ENV3", Value: "value3"},
					},
					Files: []v1alpha1.FileVar{
						{Key: "config.yaml", MountPath: "/etc/config/config.yaml", Value: "overridden content"},
						{Key: "secrets.yaml", MountPath: "/etc/secrets/secrets.yaml", Value: "secret content"},
					},
				},
			},
			want: &v1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{
							Image: "test:v1",
							Env: []v1alpha1.EnvVar{
								{Key: "ENV1", Value: "overridden_value1"},
								{Key: "ENV3", Value: "value3"},
								{Key: "ENV2", Value: "value2"},
							},
							Files: []v1alpha1.FileVar{
								{Key: "config.yaml", MountPath: "/etc/config/config.yaml", Value: "overridden content"},
								{Key: "secrets.yaml", MountPath: "/etc/secrets/secrets.yaml", Value: "secret content"},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeWorkloadOverrides(tt.baseWorkload, tt.overrides)
			if diff := cmp.Diff(tt.want, got, sortEnvsByKey(), sortFilesByKey()); diff != "" {
				t.Errorf("MergeWorkloadOverrides() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergeEnvConfigs(t *testing.T) {
	tests := []struct {
		name         string
		baseEnvs     []v1alpha1.EnvVar
		overrideEnvs []v1alpha1.EnvVar
		want         []v1alpha1.EnvVar
	}{
		{
			name:         "empty override envs",
			baseEnvs:     []v1alpha1.EnvVar{{Key: "ENV1", Value: "value1"}},
			overrideEnvs: []v1alpha1.EnvVar{},
			want:         []v1alpha1.EnvVar{{Key: "ENV1", Value: "value1"}},
		},
		{
			name:         "nil override envs",
			baseEnvs:     []v1alpha1.EnvVar{{Key: "ENV1", Value: "value1"}},
			overrideEnvs: nil,
			want:         []v1alpha1.EnvVar{{Key: "ENV1", Value: "value1"}},
		},
		{
			name:     "override existing env",
			baseEnvs: []v1alpha1.EnvVar{{Key: "ENV1", Value: "value1"}},
			overrideEnvs: []v1alpha1.EnvVar{
				{Key: "ENV1", Value: "overridden_value1"},
			},
			want: []v1alpha1.EnvVar{{Key: "ENV1", Value: "overridden_value1"}},
		},
		{
			name:     "add new env",
			baseEnvs: []v1alpha1.EnvVar{{Key: "ENV1", Value: "value1"}},
			overrideEnvs: []v1alpha1.EnvVar{
				{Key: "ENV2", Value: "value2"},
			},
			want: []v1alpha1.EnvVar{
				{Key: "ENV1", Value: "value1"},
				{Key: "ENV2", Value: "value2"},
			},
		},
		{
			name: "override and add envs",
			baseEnvs: []v1alpha1.EnvVar{
				{Key: "ENV1", Value: "value1"},
				{Key: "ENV2", Value: "value2"},
			},
			overrideEnvs: []v1alpha1.EnvVar{
				{Key: "ENV1", Value: "overridden_value1"},
				{Key: "ENV3", Value: "value3"},
			},
			want: []v1alpha1.EnvVar{
				{Key: "ENV3", Value: "value3"},
				{Key: "ENV1", Value: "overridden_value1"},
				{Key: "ENV2", Value: "value2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeEnvConfigs(tt.baseEnvs, tt.overrideEnvs)
			if diff := cmp.Diff(tt.want, got, sortEnvsByKey()); diff != "" {
				t.Errorf("mergeEnvConfigs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergeFileConfigs(t *testing.T) {
	tests := []struct {
		name          string
		baseFiles     []v1alpha1.FileVar
		overrideFiles []v1alpha1.FileVar
		want          []v1alpha1.FileVar
	}{
		{
			name:          "empty override files",
			baseFiles:     []v1alpha1.FileVar{{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "content"}},
			overrideFiles: []v1alpha1.FileVar{},
			want:          []v1alpha1.FileVar{{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "content"}},
		},
		{
			name:          "nil override files",
			baseFiles:     []v1alpha1.FileVar{{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "content"}},
			overrideFiles: nil,
			want:          []v1alpha1.FileVar{{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "content"}},
		},
		{
			name:      "override existing file",
			baseFiles: []v1alpha1.FileVar{{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "base content"}},
			overrideFiles: []v1alpha1.FileVar{
				{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "overridden content"},
			},
			want: []v1alpha1.FileVar{{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "overridden content"}},
		},
		{
			name:      "add new file",
			baseFiles: []v1alpha1.FileVar{{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "content"}},
			overrideFiles: []v1alpha1.FileVar{
				{Key: "secrets.yaml", MountPath: "/etc/secrets.yaml", Value: "secret content"},
			},
			want: []v1alpha1.FileVar{
				{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "content"},
				{Key: "secrets.yaml", MountPath: "/etc/secrets.yaml", Value: "secret content"},
			},
		},
		{
			name: "override and add files",
			baseFiles: []v1alpha1.FileVar{
				{Key: "app.yaml", MountPath: "/etc/app.yaml", Value: "app content"},
				{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "base content"},
			},
			overrideFiles: []v1alpha1.FileVar{
				{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "overridden content"},
				{Key: "secrets.yaml", MountPath: "/etc/secrets.yaml", Value: "secret content"},
			},
			want: []v1alpha1.FileVar{
				{Key: "app.yaml", MountPath: "/etc/app.yaml", Value: "app content"},
				{Key: "secrets.yaml", MountPath: "/etc/secrets.yaml", Value: "secret content"},
				{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "overridden content"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeFileConfigs(tt.baseFiles, tt.overrideFiles)
			if diff := cmp.Diff(tt.want, got, sortFilesByKey()); diff != "" {
				t.Errorf("mergeFileConfigs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// sortEnvsByKey is used to sort EnvVar slices by Key for comparison, ignoring order
func sortEnvsByKey() cmp.Option {
	return cmpopts.SortSlices(func(a, b v1alpha1.EnvVar) bool {
		return a.Key < b.Key
	})
}

// sortFilesByKey is used to sort FileVar slices by Key for comparison, ignoring order
func sortFilesByKey() cmp.Option {
	return cmpopts.SortSlices(func(a, b v1alpha1.FileVar) bool {
		return a.Key < b.Key
	})
}
