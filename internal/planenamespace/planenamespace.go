// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package planenamespace names, labels and cleans up the plane namespaces that
// each belong to one control plane namespace.
package planenamespace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/labels"
)

// Kind is a family of plane namespaces, one per control plane namespace.
type Kind struct {
	prefix string
	marker string
}

var (
	// KV holds the K8s Secrets and PushSecrets the Secret API provisions.
	KV = Kind{prefix: "openchoreo-kv-", marker: labels.LabelKeyKVNamespace}
	// Workflow runs workflows and holds git secrets.
	Workflow = Kind{prefix: "workflows-", marker: labels.LabelKeyWorkflowNamespace}

	kinds = []Kind{KV, Workflow}
)

// listPageSize bounds each LIST of plane namespaces.
const listPageSize = 500

// Name returns the plane namespace of a control plane namespace.
func (k Kind) Name(cpNamespace string) string {
	return k.prefix + cpNamespace
}

// Labels returns the labels that identify the plane namespace of cpNamespace.
// The plane controllers find namespaces to delete by these labels.
func (k Kind) Labels(cpNamespace string) map[string]string {
	return map[string]string{
		k.marker:                     labels.LabelValueTrue,
		labels.LabelKeyNamespaceName: cpNamespace,
	}
}

// Ensure creates the plane namespace of cpNamespace if absent, recording
// createdBy on it, and adds the identifying labels to one that lacks them,
// such as one created before the labels existed.
func (k Kind) Ensure(ctx context.Context, planeClient client.Client, cpNamespace, createdBy string) error {
	name := k.Name(cpNamespace)
	want := k.Labels(cpNamespace)

	ns := &corev1.Namespace{}
	err := planeClient.Get(ctx, client.ObjectKey{Name: name}, ns)
	if apierrors.IsNotFound(err) {
		want[labels.LabelKeyCreatedBy] = createdBy
		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: want}}
		if err := planeClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create namespace %s: %w", name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get namespace %s: %w", name, err)
	}

	missing := make(map[string]string, len(want))
	for key, value := range want {
		if ns.Labels[key] != value {
			missing[key] = value
		}
	}
	if len(missing) == 0 {
		return nil
	}
	patch, err := json.Marshal(map[string]any{"metadata": map[string]any{"labels": missing}})
	if err != nil {
		return fmt.Errorf("failed to build namespace label patch: %w", err)
	}
	if err := planeClient.Patch(ctx, ns, client.RawPatch(types.MergePatchType, patch)); err != nil {
		return fmt.Errorf("failed to label namespace %s: %w", name, err)
	}
	return nil
}

// DeleteForNamespace deletes every plane namespace of cpNamespace in a plane.
// Deleting a namespace lets external-secrets remove the pushed values from the
// store as it finalizes each PushSecret.
func DeleteForNamespace(ctx context.Context, planeClient client.Client, cpNamespace string) error {
	errs := make([]error, 0, len(kinds))
	for _, k := range kinds {
		errs = append(errs, forEach(ctx, planeClient, k.Labels(cpNamespace), func(ns *corev1.Namespace) error {
			return deleteNamespace(ctx, planeClient, ns)
		}))
	}
	return errors.Join(errs...)
}

// Sweep deletes the plane namespaces in a plane whose control plane namespace
// is gone or terminating, and returns their names. A namespace the cached
// reader does not find is confirmed with the live reader, so one created after
// the cache last synced is not taken for gone.
func Sweep(ctx context.Context, planeClient client.Client, cached, live client.Reader) ([]string, error) {
	var deleted []string
	var errs []error
	for _, k := range kinds {
		err := forEach(ctx, planeClient, map[string]string{k.marker: labels.LabelValueTrue}, func(ns *corev1.Namespace) error {
			cpNamespace := ns.Labels[labels.LabelKeyNamespaceName]
			if cpNamespace == "" {
				return nil
			}
			gone, err := isGoneOrTerminating(ctx, cached, live, cpNamespace)
			if err != nil {
				errs = append(errs, err)
				return nil
			}
			if !gone {
				return nil
			}
			if err := deleteNamespace(ctx, planeClient, ns); err != nil {
				errs = append(errs, err)
				return nil
			}
			deleted = append(deleted, ns.Name)
			return nil
		})
		errs = append(errs, err)
	}
	return deleted, errors.Join(errs...)
}

// forEach calls fn for each namespace in the plane that matches the labels and
// is not already terminating.
func forEach(ctx context.Context, planeClient client.Client, match client.MatchingLabels, fn func(*corev1.Namespace) error) error {
	cont := ""
	for {
		opts := []client.ListOption{match, client.Limit(listPageSize)}
		if cont != "" {
			opts = append(opts, client.Continue(cont))
		}
		list := &corev1.NamespaceList{}
		if err := planeClient.List(ctx, list, opts...); err != nil {
			return fmt.Errorf("failed to list plane namespaces: %w", err)
		}
		for i := range list.Items {
			if !list.Items[i].DeletionTimestamp.IsZero() {
				continue
			}
			if err := fn(&list.Items[i]); err != nil {
				return err
			}
		}
		if list.Continue == "" {
			return nil
		}
		cont = list.Continue
	}
}

// isGoneOrTerminating confirms with the live reader whatever the cache reports
// other than an active namespace, as a stale cache may still hold a namespace
// that was deleted and recreated.
func isGoneOrTerminating(ctx context.Context, cached, live client.Reader, name string) (bool, error) {
	ns := &corev1.Namespace{}
	err := cached.Get(ctx, client.ObjectKey{Name: name}, ns)
	if err == nil && ns.DeletionTimestamp.IsZero() {
		return false, nil
	}
	if client.IgnoreNotFound(err) != nil {
		return false, fmt.Errorf("failed to get namespace %s: %w", name, err)
	}
	ns = &corev1.Namespace{}
	err = live.Get(ctx, client.ObjectKey{Name: name}, ns)
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get namespace %s: %w", name, err)
	}
	return !ns.DeletionTimestamp.IsZero(), nil
}

func deleteNamespace(ctx context.Context, planeClient client.Client, ns *corev1.Namespace) error {
	err := planeClient.Delete(ctx, ns)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete plane namespace %s: %w", ns.Name, err)
	}
	return nil
}
