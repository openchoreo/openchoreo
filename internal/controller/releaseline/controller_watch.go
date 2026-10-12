// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/component"
)

// traitsIndex indexes ReleaseLine by the "kind:name" of the traits it adds or overrides.
const traitsIndex = "releaseline.spec.traits"

// SetupWithManager sets up the controller with the Manager. It relies on the shared indexes and on
// the indexes the component controller registers.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("releaseline-controller")
	}
	r.steps = component.NewSteps(r.Client, r.Scheme, r.Recorder)

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &openchoreov1alpha1.ReleaseLine{},
		traitsIndex, indexTraits); err != nil {
		return fmt.Errorf("failed to setup release line traits index: %w", err)
	}

	b := ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ReleaseLine{}).
		Watches(&openchoreov1alpha1.Component{}, handler.EnqueueRequestsFromMapFunc(r.linesOfComponent)).
		Watches(&openchoreov1alpha1.Workload{}, handler.EnqueueRequestsFromMapFunc(lineOf)).
		Watches(&openchoreov1alpha1.ComponentRelease{}, handler.EnqueueRequestsFromMapFunc(lineOf)).
		Watches(&openchoreov1alpha1.ReleaseBinding{}, handler.EnqueueRequestsFromMapFunc(lineOf))
	for _, w := range r.steps.ComponentWatches() {
		b = b.Watches(w.Object, handler.EnqueueRequestsFromMapFunc(r.linesOfComponents(w.Object, w.Map)))
	}
	return b.Named("releaseline").Complete(r)
}

func indexTraits(obj client.Object) []string {
	line := obj.(*openchoreov1alpha1.ReleaseLine)
	keys := make([]string, 0, len(line.Spec.Traits))
	for _, t := range line.Spec.Traits {
		kind := t.Kind
		if kind == "" {
			kind = openchoreov1alpha1.TraitRefKindTrait
		}
		keys = append(keys, string(kind)+":"+t.Name)
	}
	return keys
}

// lineOf enqueues the ReleaseLine a Workload, ComponentRelease or ReleaseBinding belongs to.
func lineOf(_ context.Context, obj client.Object) []reconcile.Request {
	var line string
	switch o := obj.(type) {
	case *openchoreov1alpha1.Workload:
		line = o.Spec.Owner.ReleaseLine
	case *openchoreov1alpha1.ComponentRelease:
		line = o.Spec.Owner.ReleaseLine
	case *openchoreov1alpha1.ReleaseBinding:
		line = o.Spec.Owner.ReleaseLine
	}
	if line == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: obj.GetNamespace(), Name: line}}}
}

// linesOfComponent enqueues every ReleaseLine of a Component.
func (r *Reconciler) linesOfComponent(ctx context.Context, obj client.Object) []reconcile.Request {
	comp := obj.(*openchoreov1alpha1.Component)
	return r.lines(ctx, comp.Namespace, client.MatchingFields{
		controller.IndexKeyReleaseLineOwner: controller.MakeReleaseLineOwnerKey(comp.Spec.Owner.ProjectName, comp.Name),
	})
}

// linesOfComponents enqueues the ReleaseLines of every Component mapComponents returns. For traits,
// it also enqueues lines that add the trait themselves.
func (r *Reconciler) linesOfComponents(watched client.Object, mapComponents handler.MapFunc) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		var requests []reconcile.Request
		for _, req := range mapComponents(ctx, obj) {
			comp := &openchoreov1alpha1.Component{}
			if err := r.Get(ctx, req.NamespacedName, comp); err != nil {
				continue
			}
			if comp.Spec.ReleaseLinesEnabled {
				requests = append(requests, r.linesOfComponent(ctx, comp)...)
			}
		}
		switch watched.(type) {
		case *openchoreov1alpha1.Trait:
			requests = append(requests, r.lines(ctx, obj.GetNamespace(),
				client.MatchingFields{traitsIndex: string(openchoreov1alpha1.TraitRefKindTrait) + ":" + obj.GetName()})...)
		case *openchoreov1alpha1.ClusterTrait:
			requests = append(requests, r.lines(ctx, "",
				client.MatchingFields{traitsIndex: string(openchoreov1alpha1.TraitRefKindClusterTrait) + ":" + obj.GetName()})...)
		}
		return requests
	}
}

func (r *Reconciler) lines(ctx context.Context, namespace string, selector client.MatchingFields) []reconcile.Request {
	var lines openchoreov1alpha1.ReleaseLineList
	opts := []client.ListOption{selector}
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	if err := r.List(ctx, &lines, opts...); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to list ReleaseLines")
		return nil
	}
	requests := make([]reconcile.Request, 0, len(lines.Items))
	for _, l := range lines.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: l.Namespace, Name: l.Name}})
	}
	return requests
}
