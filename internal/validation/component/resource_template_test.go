// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestValidateWorkloadResources_Proxy(t *testing.T) {
	basePath := field.NewPath("spec", "resources")

	t.Run("valid proxy without workload kinds", func(t *testing.T) {
		resources := []v1alpha1.ResourceTemplate{
			{
				ID:       "service",
				Template: rawJSON(`{"apiVersion":"v1","kind":"Service","metadata":{"name":"test"}}`),
			},
			{
				ID:       "configmap",
				Template: rawJSON(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test-config"}}`),
			},
		}
		errs := ValidateWorkloadResources("proxy", resources, basePath)
		assert.Empty(t, errs)
	})

	t.Run("proxy with workload kind rejected", func(t *testing.T) {
		resources := []v1alpha1.ResourceTemplate{
			{
				ID:       "deployment",
				Template: rawJSON(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test"}}`),
			},
		}
		errs := ValidateWorkloadResources("proxy", resources, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "proxy ComponentType must not contain workload resources")
	})

	t.Run("proxy with nil template", func(t *testing.T) {
		resources := []v1alpha1.ResourceTemplate{
			{ID: "service", Template: nil},
		}
		errs := ValidateWorkloadResources("proxy", resources, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "template is required")
	})
}

func TestValidateWorkloadResources_Normal(t *testing.T) {
	basePath := field.NewPath("spec", "resources")

	t.Run("exactly one matching kind", func(t *testing.T) {
		resources := []v1alpha1.ResourceTemplate{
			{
				ID:       "deployment",
				Template: rawJSON(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test"}}`),
			},
			{
				ID:       "service",
				Template: rawJSON(`{"apiVersion":"v1","kind":"Service","metadata":{"name":"test-svc"}}`),
			},
		}
		errs := ValidateWorkloadResources("deployment", resources, basePath)
		assert.Empty(t, errs)
	})

	t.Run("zero matching kinds", func(t *testing.T) {
		resources := []v1alpha1.ResourceTemplate{
			{
				ID:       "service",
				Template: rawJSON(`{"apiVersion":"v1","kind":"Service","metadata":{"name":"test"}}`),
			},
		}
		errs := ValidateWorkloadResources("deployment", resources, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "must have exactly one resource with kind matching workloadType")
	})

	t.Run("duplicate matching kinds", func(t *testing.T) {
		resources := []v1alpha1.ResourceTemplate{
			{
				ID:       "dep1",
				Template: rawJSON(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test1"}}`),
			},
			{
				ID:       "dep2",
				Template: rawJSON(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test2"}}`),
			},
		}
		errs := ValidateWorkloadResources("deployment", resources, basePath)
		require.NotEmpty(t, errs)
		errStr := errs.ToAggregate().Error()
		assert.Contains(t, errStr, "must have exactly one resource with kind matching workloadType")
	})

	t.Run("mismatched workload kind rejected", func(t *testing.T) {
		resources := []v1alpha1.ResourceTemplate{
			{
				ID:       "deployment",
				Template: rawJSON(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test"}}`),
			},
			{
				ID:       "statefulset",
				Template: rawJSON(`{"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{"name":"test2"}}`),
			},
		}
		errs := ValidateWorkloadResources("deployment", resources, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "does not match the declared workloadType")
	})

	t.Run("case-insensitive match", func(t *testing.T) {
		resources := []v1alpha1.ResourceTemplate{
			{
				ID:       "deployment",
				Template: rawJSON(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test"}}`),
			},
		}
		errs := ValidateWorkloadResources("deployment", resources, basePath)
		assert.Empty(t, errs)
	})

	t.Run("structurally invalid template still counts for cardinality", func(t *testing.T) {
		// kind is present (matches workloadType) but metadata.name is missing
		// Both structure error and cardinality check should work
		resources := []v1alpha1.ResourceTemplate{
			{
				ID:       "deployment",
				Template: rawJSON(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{}}`),
			},
		}
		errs := ValidateWorkloadResources("deployment", resources, basePath)
		require.NotEmpty(t, errs)
		// Structure error for missing name, but no cardinality error
		errStr := errs.ToAggregate().Error()
		assert.Contains(t, errStr, "metadata.name is required")
		assert.NotContains(t, errStr, "must have exactly one resource")
	})

	t.Run("nil template in resource", func(t *testing.T) {
		resources := []v1alpha1.ResourceTemplate{
			{ID: "deployment", Template: nil},
		}
		errs := ValidateWorkloadResources("deployment", resources, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "template is required")
	})

	t.Run("empty resources", func(t *testing.T) {
		errs := ValidateWorkloadResources("deployment", nil, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "must have exactly one resource with kind matching workloadType")
	})
}

func TestValidateResourceTemplateStructure(t *testing.T) {
	basePath := field.NewPath("spec", "resources").Index(0).Child("template")

	t.Run("valid complete template", func(t *testing.T) {
		tmpl := runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test"}}`),
		}
		obj, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		assert.Empty(t, errs)
		require.NotNil(t, obj)
		assert.Equal(t, "Deployment", obj.Kind)
		assert.Equal(t, "apps/v1", obj.APIVersion)
		assert.Equal(t, "test", obj.Name)
	})

	t.Run("empty raw", func(t *testing.T) {
		tmpl := runtime.RawExtension{Raw: []byte{}}
		obj, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		require.NotEmpty(t, errs)
		assert.Nil(t, obj)
		assert.Contains(t, errs.ToAggregate().Error(), "template is required")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		tmpl := runtime.RawExtension{Raw: []byte(`{invalid`)}
		obj, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		require.NotEmpty(t, errs)
		assert.Nil(t, obj)
		assert.Contains(t, errs.ToAggregate().Error(), "failed to parse template")
	})

	t.Run("missing apiVersion", func(t *testing.T) {
		tmpl := runtime.RawExtension{
			Raw: []byte(`{"kind":"Deployment","metadata":{"name":"test"}}`),
		}
		_, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "apiVersion is required")
	})

	t.Run("missing kind", func(t *testing.T) {
		tmpl := runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"apps/v1","metadata":{"name":"test"}}`),
		}
		_, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "kind is required")
	})

	t.Run("CEL expression in kind forbidden", func(t *testing.T) {
		tmpl := runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"apps/v1","kind":"${parameters.kind}","metadata":{"name":"test"}}`),
		}
		_, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "kind must be a literal value")
	})

	t.Run("missing metadata.name nil", func(t *testing.T) {
		tmpl := runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{}}`),
		}
		_, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "metadata.name is required")
	})

	t.Run("missing metadata.name empty string", func(t *testing.T) {
		tmpl := runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":""}}`),
		}
		_, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "metadata.name is required")
	})

	t.Run("valid namespace with CEL", func(t *testing.T) {
		tmpl := runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test","namespace":"${metadata.namespace}"}}`),
		}
		_, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		assert.Empty(t, errs)
	})

	t.Run("valid namespace with inner whitespace", func(t *testing.T) {
		tmpl := runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test","namespace":"${ metadata.namespace }"}}`),
		}
		_, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		assert.Empty(t, errs)
	})

	t.Run("valid namespace with outer whitespace", func(t *testing.T) {
		tmpl := runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test","namespace":"  ${metadata.namespace}  "}}`),
		}
		_, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		assert.Empty(t, errs)
	})

	t.Run("invalid namespace hardcoded value", func(t *testing.T) {
		tmpl := runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test","namespace":"my-namespace"}}`),
		}
		_, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "must be \"${metadata.namespace}\"")
	})

	t.Run("invalid namespace non-string type", func(t *testing.T) {
		tmpl := runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test","namespace":42}}`),
		}
		_, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "must be \"${metadata.namespace}\"")
	})

	t.Run("name with CEL expression is valid", func(t *testing.T) {
		tmpl := runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"${metadata.name}-dep"}}`),
		}
		obj, errs := ValidateResourceTemplateStructure(tmpl, basePath)
		assert.Empty(t, errs)
		require.NotNil(t, obj)
		assert.Equal(t, "${metadata.name}-dep", obj.Name)
	})
}

func TestIsWorkloadResourceKind(t *testing.T) {
	t.Run("mixed case positive", func(t *testing.T) {
		assert.True(t, IsWorkloadResourceKind("Deployment"))
		assert.True(t, IsWorkloadResourceKind("STATEFULSET"))
		assert.True(t, IsWorkloadResourceKind("CronJob"))
	})

	t.Run("non-workload kinds", func(t *testing.T) {
		assert.False(t, IsWorkloadResourceKind("Service"))
		assert.False(t, IsWorkloadResourceKind("ConfigMap"))
		assert.False(t, IsWorkloadResourceKind("HTTPRoute"))
		assert.False(t, IsWorkloadResourceKind(""))
	})
}

func TestValidateTraitCreateTemplates(t *testing.T) {
	basePath := field.NewPath("spec", "creates")

	t.Run("nil template", func(t *testing.T) {
		creates := []v1alpha1.TraitCreate{
			{Template: nil},
		}
		errs := ValidateTraitCreateTemplates(creates, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "template is required")
	})

	t.Run("valid template", func(t *testing.T) {
		creates := []v1alpha1.TraitCreate{
			{
				Template: rawJSON(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
			},
		}
		errs := ValidateTraitCreateTemplates(creates, basePath)
		assert.Empty(t, errs)
	})

	t.Run("workload kind forbidden", func(t *testing.T) {
		creates := []v1alpha1.TraitCreate{
			{
				Template: rawJSON(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test"}}`),
			},
		}
		errs := ValidateTraitCreateTemplates(creates, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "traits must not create workload resources")
	})

	t.Run("empty creates", func(t *testing.T) {
		errs := ValidateTraitCreateTemplates(nil, basePath)
		assert.Empty(t, errs)
	})

	t.Run("multiple creates with mixed validity", func(t *testing.T) {
		creates := []v1alpha1.TraitCreate{
			{
				Template: rawJSON(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
			},
			{Template: nil},
			{
				Template: rawJSON(`{"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{"name":"test"}}`),
			},
		}
		errs := ValidateTraitCreateTemplates(creates, basePath)
		require.Len(t, errs, 2) // nil template + workload kind
	})
}

func TestIsAllowedNamespaceValue(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want bool
	}{
		{"exact match", "${metadata.namespace}", true},
		{"outer whitespace", "  ${metadata.namespace}  ", true},
		{"inner whitespace", "${ metadata.namespace }", true},
		{"wrong expression", "${metadata.name}", false},
		{"non-template string", "hardcoded-ns", false},
		{"empty string", "", false},
		{"missing closing brace", "${metadata.namespace", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isAllowedNamespaceValue(tt.val))
		})
	}
}

// rawJSON creates a *runtime.RawExtension from a JSON string.
func rawJSON(s string) *runtime.RawExtension {
	return &runtime.RawExtension{Raw: []byte(s)}
}
