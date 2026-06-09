// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

const (
	testAuthzRoleName        = "developer"
	testAuthzRoleBindingName = "developer-binding"
)

// peAuthzSpecs returns test specs for the authz tool surface (roles, role bindings,
// and diagnostics). All CRUD tools are scope-collapsed; diagnostics are flat.
func peAuthzSpecs() []toolTestSpec {
	listGet := []toolTestSpec{
		authzListSpec("list_authz_roles", []string{"list", "authz", "role"}, "ListAuthzRoles"),
		authzGetSpec("get_authz_role", []string{"authz", "role", "spec"}, testAuthzRoleName, "GetAuthzRole"),
		authzListSpec(
			"list_authz_role_bindings",
			[]string{"list", "authz", "role", "binding"}, "ListAuthzRoleBindings",
		),
		authzGetSpec(
			"get_authz_role_binding",
			[]string{"authz", "role", "binding"}, testAuthzRoleBindingName, "GetAuthzRoleBinding",
		),
	}
	writes := peAuthzWriteAndSchemaSpecs()
	diagnostics := peAuthzDiagnosticsSpecs()
	specs := make([]toolTestSpec, 0, len(listGet)+len(writes)+len(diagnostics))
	specs = append(specs, listGet...)
	specs = append(specs, writes...)
	specs = append(specs, diagnostics...)
	return specs
}

func authzListSpec(name string, keywords []string, method string) toolTestSpec {
	return toolTestSpec{
		name:                name,
		toolset:             "pe",
		descriptionKeywords: keywords,
		descriptionMinLen:   10,
		optionalParams:      []string{"scope", "namespace_name", "limit", "cursor"},
		testArgs:            map[string]any{"namespace_name": testNamespaceName},
		expectedMethod:      method,
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testNamespaceName {
				t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
			}
		},
	}
}

func authzGetSpec(name string, keywords []string, resourceName, method string) toolTestSpec {
	return toolTestSpec{
		name:                name,
		toolset:             "pe",
		descriptionKeywords: keywords,
		descriptionMinLen:   10,
		requiredParams:      []string{"name"},
		optionalParams:      []string{"scope", "namespace_name"},
		testArgs:            map[string]any{"namespace_name": testNamespaceName, "name": resourceName},
		expectedMethod:      method,
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testNamespaceName || args[1] != resourceName {
				t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, resourceName, args[0], args[1])
			}
		},
	}
}

func peAuthzWriteAndSchemaSpecs() []toolTestSpec {
	roleSpec := map[string]any{"actions": []any{"component:view"}}
	bindingSpec := map[string]any{
		"entitlement": map[string]any{"claim": "groups", "value": "dev-team"},
		"roleMappings": []any{
			map[string]any{"roleRef": map[string]any{"kind": "AuthzRole", "name": testAuthzRoleName}},
		},
	}
	return []toolTestSpec{
		// Creation schemas (static, no handler call)
		{
			name:                "get_authz_role_creation_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"schema", "AuthzRole"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope"},
			testArgs:            map[string]any{},
		},
		{
			name:                "get_authz_role_binding_creation_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"schema", "binding"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope"},
			testArgs:            map[string]any{},
		},
		// Roles — writes
		authzWriteSpec(
			"create_authz_role", []string{"create", "authz", "role"},
			testAuthzRoleName, roleSpec, "CreateAuthzRole"),
		authzWriteSpec(
			"update_authz_role", []string{"update", "authz", "role"},
			testAuthzRoleName, roleSpec, "UpdateAuthzRole"),
		authzDeleteSpec("delete_authz_role", []string{"delete", "authz", "role"},
			testAuthzRoleName, "DeleteAuthzRole"),
		// Bindings — writes
		authzWriteSpec(
			"create_authz_role_binding", []string{"create", "authz", "role", "binding"},
			testAuthzRoleBindingName, bindingSpec, "CreateAuthzRoleBinding"),
		authzWriteSpec(
			"update_authz_role_binding", []string{"update", "authz", "role", "binding"},
			testAuthzRoleBindingName, bindingSpec, "UpdateAuthzRoleBinding"),
		authzDeleteSpec("delete_authz_role_binding", []string{"delete", "authz", "role", "binding"},
			testAuthzRoleBindingName, "DeleteAuthzRoleBinding"),
	}
}

func authzWriteSpec(
	name string, keywords []string, resourceName string, spec map[string]any, method string,
) toolTestSpec {
	return toolTestSpec{
		name:                name,
		toolset:             "pe",
		descriptionKeywords: keywords,
		descriptionMinLen:   10,
		requiredParams:      []string{"name", "spec"},
		optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
		testArgs: map[string]any{
			"namespace_name": testNamespaceName,
			"name":           resourceName,
			"spec":           spec,
		},
		expectedMethod: method,
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testNamespaceName {
				t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
			}
		},
	}
}

func authzDeleteSpec(name string, keywords []string, resourceName, method string) toolTestSpec {
	return toolTestSpec{
		name:                name,
		toolset:             "pe",
		descriptionKeywords: keywords,
		descriptionMinLen:   10,
		optionalParams:      []string{"scope", "namespace_name"},
		requiredParams:      []string{"name"},
		testArgs: map[string]any{
			"namespace_name": testNamespaceName,
			"name":           resourceName,
		},
		expectedMethod: method,
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testNamespaceName || args[1] != resourceName {
				t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, resourceName, args[0], args[1])
			}
		},
	}
}

func peAuthzDiagnosticsSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "evaluate_authz",
			toolset:             "pe",
			descriptionKeywords: []string{"evaluat", "authoriz"},
			descriptionMinLen:   10,
			requiredParams:      []string{"requests"},
			testArgs: map[string]any{
				"requests": []any{
					map[string]any{
						"action":   "component:view",
						"resource": map[string]any{"type": "component"},
						"subject_context": map[string]any{
							"type":               "user",
							"entitlement_claim":  "groups",
							"entitlement_values": []any{"dev-team"},
						},
					},
				},
			},
			expectedMethod: "EvaluateAuthz",
			validateCall:   func(t *testing.T, args []interface{}) {},
		},
		{
			name:                "list_authz_actions",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "action"},
			descriptionMinLen:   10,
			testArgs:            map[string]any{},
			expectedMethod:      "ListAuthzActions",
			validateCall:        func(t *testing.T, args []interface{}) {},
		},
	}
}
