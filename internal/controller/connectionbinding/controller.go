// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package connectionbinding

import (
	"context"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Reconciler reconciles a ConnectionBinding object.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=connectionbindings,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=connectionbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releasebindings,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releasebindings/status,verbs=get

// Reconcile resolves connection URLs by looking up dependency ReleaseBindings
// and extracting endpoint URLs from their status.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cb := &openchoreov1alpha1.ConnectionBinding{}
	if err := r.Get(ctx, req.NamespacedName, cb); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get ConnectionBinding")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	old := cb.DeepCopy()

	// Resolve connections
	var resolved []openchoreov1alpha1.ResolvedConnection
	var pending []openchoreov1alpha1.PendingConnection

	for _, conn := range cb.Spec.Connections {
		resolvedConn, pendingConn, err := r.resolveConnection(ctx, cb, conn)
		if err != nil {
			logger.Error(err, "Failed to resolve connection", "target", conn.Component)
			return ctrl.Result{}, err
		}
		if pendingConn != nil {
			pending = append(pending, *pendingConn)
		} else if resolvedConn != nil {
			resolved = append(resolved, *resolvedConn)
		}
	}

	cb.Status.Resolved = resolved
	cb.Status.Pending = pending

	// Set conditions
	if len(cb.Spec.Connections) == 0 {
		controller.MarkTrueCondition(cb, ConditionAllResolved, ReasonNoConnections, "No connections to resolve")
	} else if len(pending) == 0 {
		controller.MarkTrueCondition(cb, ConditionAllResolved, ReasonAllResolved, "All connections resolved")
	} else {
		msg := fmt.Sprintf("%d of %d connections pending", len(pending), len(cb.Spec.Connections))
		controller.MarkFalseCondition(cb, ConditionAllResolved, ReasonConnectionsPending, msg)
	}

	// Update status only if changed
	if !apiequality.Semantic.DeepEqual(old.Status, cb.Status) {
		if err := r.Status().Update(ctx, cb); err != nil {
			logger.Error(err, "Failed to update ConnectionBinding status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// resolveConnection attempts to resolve a single connection target by looking up the
// dependency ReleaseBinding and extracting the endpoint URL from its status.
// It returns a non-nil error only for transient API failures that should trigger a requeue.
func (r *Reconciler) resolveConnection(
	ctx context.Context,
	cb *openchoreov1alpha1.ConnectionBinding,
	conn openchoreov1alpha1.ConnectionTarget,
) (*openchoreov1alpha1.ResolvedConnection, *openchoreov1alpha1.PendingConnection, error) {
	// Look up the target ReleaseBinding using the shared composite index
	indexKey := controller.MakeReleaseBindingOwnerEnvKey(conn.Project, conn.Component, cb.Spec.Environment)

	var rbList openchoreov1alpha1.ReleaseBindingList
	if err := r.List(ctx, &rbList,
		client.InNamespace(conn.Namespace),
		client.MatchingFields{controller.IndexKeyReleaseBindingOwnerEnv: indexKey}); err != nil {
		return nil, nil, fmt.Errorf("failed to list ReleaseBindings for component %s/%s: %w", conn.Project, conn.Component, err)
	}

	if len(rbList.Items) == 0 {
		return nil, &openchoreov1alpha1.PendingConnection{
			Namespace: conn.Namespace,
			Project:   conn.Project,
			Component: conn.Component,
			Endpoint:  conn.Endpoint,
			Reason:    fmt.Sprintf("ReleaseBinding not found for component %s/%s", conn.Project, conn.Component),
		}, nil
	}

	if len(rbList.Items) > 1 {
		return nil, &openchoreov1alpha1.PendingConnection{
			Namespace: conn.Namespace,
			Project:   conn.Project,
			Component: conn.Component,
			Endpoint:  conn.Endpoint,
			Reason:    fmt.Sprintf("multiple ReleaseBindings found for component %s/%s in environment %s", conn.Project, conn.Component, cb.Spec.Environment),
		}, nil
	}

	rb := &rbList.Items[0]

	// Check if the component is undeployed
	if rb.Spec.State == openchoreov1alpha1.ReleaseStateUndeploy {
		return nil, &openchoreov1alpha1.PendingConnection{
			Namespace: conn.Namespace,
			Project:   conn.Project,
			Component: conn.Component,
			Endpoint:  conn.Endpoint,
			Reason:    "component is undeployed",
		}, nil
	}

	// Look for the endpoint by name in status.endpoints
	for _, ep := range rb.Status.Endpoints {
		if ep.Name != conn.Endpoint {
			continue
		}
		url := r.resolveURLForVisibility(ep, conn.Visibility)
		if url == nil {
			return nil, &openchoreov1alpha1.PendingConnection{
				Namespace: conn.Namespace,
				Project:   conn.Project,
				Component: conn.Component,
				Endpoint:  conn.Endpoint,
				Reason:    fmt.Sprintf("endpoint %q has no URL for visibility %s", conn.Endpoint, conn.Visibility),
			}, nil
		}
		return &openchoreov1alpha1.ResolvedConnection{
			Namespace: conn.Namespace,
			Project:   conn.Project,
			Component: conn.Component,
			Endpoint:  conn.Endpoint,
			URL:       *url,
		}, nil, nil
	}

	return nil, &openchoreov1alpha1.PendingConnection{
		Namespace: conn.Namespace,
		Project:   conn.Project,
		Component: conn.Component,
		Endpoint:  conn.Endpoint,
		Reason:    fmt.Sprintf("endpoint %q not yet resolved", conn.Endpoint),
	}, nil
}

// resolveURLForVisibility extracts the appropriate URL from an EndpointURLStatus
// based on the requested visibility level.
func (r *Reconciler) resolveURLForVisibility(
	ep openchoreov1alpha1.EndpointURLStatus,
	visibility openchoreov1alpha1.EndpointVisibility,
) *openchoreov1alpha1.EndpointURL {
	switch visibility {
	case openchoreov1alpha1.EndpointVisibilityProject, openchoreov1alpha1.EndpointVisibilityNamespace:
		// For project and namespace visibility, use the in-cluster service URL
		return ep.ServiceURL
	case openchoreov1alpha1.EndpointVisibilityInternal:
		if ep.InternalURLs != nil {
			if ep.InternalURLs.HTTPS != nil {
				return ep.InternalURLs.HTTPS
			}
			if ep.InternalURLs.HTTP != nil {
				return ep.InternalURLs.HTTP
			}
			if ep.InternalURLs.TLS != nil {
				return ep.InternalURLs.TLS
			}
		}
		return nil
	case openchoreov1alpha1.EndpointVisibilityExternal:
		if ep.ExternalURLs != nil {
			if ep.ExternalURLs.HTTPS != nil {
				return ep.ExternalURLs.HTTPS
			}
			if ep.ExternalURLs.HTTP != nil {
				return ep.ExternalURLs.HTTP
			}
			if ep.ExternalURLs.TLS != nil {
				return ep.ExternalURLs.TLS
			}
		}
		return nil
	default:
		return nil
	}
}
