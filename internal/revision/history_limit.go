// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// EnvironmentHistoryLimit returns the history limit for the bindings of
// environment that set no spec.revisionHistoryLimit of their own: the
// Environment's spec.componentRevisionHistoryLimit when set, otherwise
// controllerLimit (the controller's --component-revision-history-limit). It is
// what a binding controller passes as Options.HistoryLimit, so the order is
// binding, Environment, controller.
func EnvironmentHistoryLimit(environment *openchoreov1alpha1.Environment, controllerLimit int) int {
	if environment != nil && environment.Spec.ComponentRevisionHistoryLimit != nil {
		return int(*environment.Spec.ComponentRevisionHistoryLimit)
	}
	return controllerLimit
}

// EnvironmentHistoryLimitChanged passes the Environment updates that change
// spec.componentRevisionHistoryLimit, and Environment creates -- one deleted and
// created again can change the limit with no update -- and nothing else: the
// binding controllers watch Environments only to trim the history of an
// environment's bindings when its limit changes.
func EnvironmentHistoryLimitChanged() predicate.Predicate {
	return predicate.Funcs{
		// An Environment created, or deleted and created again, can set a
		// limit its bindings have not applied yet.
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldEnv, ok := e.ObjectOld.(*openchoreov1alpha1.Environment)
			if !ok {
				return false
			}
			newEnv, ok := e.ObjectNew.(*openchoreov1alpha1.Environment)
			if !ok {
				return false
			}
			oldLimit, newLimit := oldEnv.Spec.ComponentRevisionHistoryLimit, newEnv.Spec.ComponentRevisionHistoryLimit
			if oldLimit == nil || newLimit == nil {
				return oldLimit != newLimit
			}
			return *oldLimit != *newLimit
		},
	}
}
