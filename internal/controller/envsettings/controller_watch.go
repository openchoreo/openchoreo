// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package envsettings

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// envSettingsComponentIndex indexes EnvSettings by component name
	envSettingsComponentIndex = "spec.owner.componentName"
	// envSettingsEnvironmentIndex indexes EnvSettings by environment
	envSettingsEnvironmentIndex = "spec.environment"
)

// setupComponentIndex registers an index for EnvSettings by component name.
func (r *Reconciler) setupComponentIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.EnvSettings{},
		envSettingsComponentIndex, func(obj client.Object) []string {
			envSettings := obj.(*openchoreov1alpha1.EnvSettings)
			if envSettings.Spec.Owner.ComponentName == "" {
				return nil
			}
			return []string{envSettings.Spec.Owner.ComponentName}
		})
}

// setupEnvironmentIndex registers an index for EnvSettings by environment.
func (r *Reconciler) setupEnvironmentIndex(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &openchoreov1alpha1.EnvSettings{},
		envSettingsEnvironmentIndex, func(obj client.Object) []string {
			envSettings := obj.(*openchoreov1alpha1.EnvSettings)
			if envSettings.Spec.Environment == "" {
				return nil
			}
			return []string{envSettings.Spec.Environment}
		})
}

// listEnvSettingsForSnapshot enqueues EnvSettings that correspond to the given ComponentEnvSnapshot.
func (r *Reconciler) listEnvSettingsForSnapshot(ctx context.Context, obj client.Object) []reconcile.Request {
	snapshot := obj.(*openchoreov1alpha1.ComponentEnvSnapshot)

	var envSettingsList openchoreov1alpha1.EnvSettingsList
	if err := r.List(ctx, &envSettingsList,
		client.InNamespace(snapshot.Namespace),
		client.MatchingFields{
			envSettingsComponentIndex:   snapshot.Spec.Owner.ComponentName,
			envSettingsEnvironmentIndex: snapshot.Spec.Environment,
		}); err != nil {
		logger := ctrl.LoggerFrom(ctx)
		logger.Error(err, "Failed to list EnvSettings for ComponentEnvSnapshot", "snapshot", obj.GetName(), "namespace", obj.GetNamespace())
		return nil
	}

	requests := make([]reconcile.Request, 0, len(envSettingsList.Items))
	for _, envSettings := range envSettingsList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      envSettings.Name,
				Namespace: envSettings.Namespace,
			},
		})
	}
	return requests
}
