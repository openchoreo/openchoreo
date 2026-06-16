// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"fmt"
	"time"

	"github.com/onsi/gomega"
)

// Parameterized ClusterAuthzRole / ClusterAuthzRoleBinding builders shared by e2e
// suites that need a custom subject and cleanup label. The subject and label
// key/value are parameters so suites running concurrently don't collide on a
// single subject.

// ClusterAuthzRoleYAML renders a ClusterAuthzRole with the given actions and an
// arbitrary label (key/value) for cleanup selectors.
func ClusterAuthzRoleYAML(name, labelKey, labelValue string, actions []string) string {
	actionsYAML := ""
	for _, a := range actions {
		actionsYAML += fmt.Sprintf("    - %q\n", a)
	}
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRole
metadata:
  name: %s
  labels:
    %s: %q
spec:
  actions:
%s`, name, labelKey, labelValue, actionsYAML)
}

// ClusterAuthzRoleBindingYAML renders a ClusterAuthzRoleBinding mapping the
// entitlement claim `sub` == subject to the named ClusterAuthzRole.
// effect is "allow" or "deny".
func ClusterAuthzRoleBindingYAML(name, labelKey, labelValue, roleName, subject, effect string) string {
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: %s
  labels:
    %s: %q
spec:
  roleMappings:
    - roleRef:
        name: %s
        kind: ClusterAuthzRole
  entitlement:
    claim: sub
    value: %s
  effect: %s
`, name, labelKey, labelValue, roleName, subject, effect)
}

// ScopedClusterAuthzRoleBindingYAML is ClusterAuthzRoleBindingYAML with a
// `scope.namespace` restriction on the role mapping.
func ScopedClusterAuthzRoleBindingYAML(name, labelKey, labelValue, roleName, subject, effect, scopeNamespace string) string {
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: %s
  labels:
    %s: %q
spec:
  roleMappings:
    - roleRef:
        name: %s
        kind: ClusterAuthzRole
      scope:
        namespace: %s
  entitlement:
    claim: sub
    value: %s
  effect: %s
`, name, labelKey, labelValue, roleName, scopeNamespace, subject, effect)
}

// DeleteClusterAuthzRoleBindingAndWaitForRevocation deletes the binding and
// polls probe() (which must return true once the subject is denied again)
// until the Casbin PDP has observed the revocation.
// Mirrors test/e2e/suites/authz/authz_test.go:23-32.
func DeleteClusterAuthzRoleBindingAndWaitForRevocation(kubeContext, name string, probe func() bool) {
	output, err := Kubectl(kubeContext, "delete", "clusterauthzrolebinding", name, "--ignore-not-found", "--wait=false")
	// Fail fast: a failed delete leaves the binding in place, so the probe below
	// would never observe revocation and would mask the real cause behind a timeout.
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(),
		"failed to delete clusterauthzrolebinding %s: %s", name, output)
	gomega.EventuallyWithOffset(1, probe, DefaultTimeout, 2*time.Second).Should(gomega.BeTrue(),
		"binding %s revocation did not propagate in time", name)
}
