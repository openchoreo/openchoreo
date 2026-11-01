// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"context"
	"fmt"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	endpointpipeline "github.com/openchoreo/openchoreo/internal/pipeline/endpoint"
)

// clusterLocalSuffix is the cluster DNS suffix the ReleaseBinding controller
// builds in-cluster service hosts with ("<svc>.<ns>.svc.cluster.local").
const clusterLocalSuffix = "svc.cluster.local"

// resolvedBackends is the outcome of resolving every backendRef of a release in
// the binding's environment.
type resolvedBackends struct {
	// statuses is one entry per backendRef, in spec order, for status.backends.
	statuses []openchoreov1alpha1.ResolvedEndpointBackend

	// backends are the resolved backends handed to the pipeline, in spec order.
	backends []endpointpipeline.Backend

	// blocking describes each backend that should receive traffic but could not
	// be resolved. Non-empty blocks rendering.
	blocking []string
}

// backendResolution is the outcome of resolving one backendRef. Exactly one of
// backend or reason is set: reason explains why the backend cannot receive
// traffic in this environment.
type backendResolution struct {
	backend    *endpointpipeline.Backend
	serviceURL *openchoreov1alpha1.EndpointURL
	reason     string
	message    string
}

// resolveBackends resolves each backendRef of the release for the binding's
// environment.
//
// A backend that cannot be resolved is reported on status with a reason rather
// than failing the reconcile. It blocks rendering only when it should receive
// traffic: an unresolved weight-0 backend is skipped, which is what lets a
// canary be staged at 0% before it is deployed in every environment.
func (r *Reconciler) resolveBackends(
	ctx context.Context,
	binding *openchoreov1alpha1.EndpointReleaseBinding,
	release *openchoreov1alpha1.EndpointRelease,
	dpNamespace string,
) (*resolvedBackends, error) {
	out := &resolvedBackends{}

	for _, ref := range release.Spec.BackendRefs {
		weight := int32(1)
		if ref.Weight != nil {
			weight = *ref.Weight
		}

		res, err := r.resolveBackend(ctx, binding, ref, dpNamespace)
		if err != nil {
			return nil, err
		}

		status := openchoreov1alpha1.ResolvedEndpointBackend{
			Name:   ref.Name,
			Kind:   ref.Kind,
			Weight: weight,
		}
		if res.backend == nil {
			status.Reason = res.reason
			status.Message = res.message
			out.statuses = append(out.statuses, status)
			if weight > 0 {
				out.blocking = append(out.blocking, fmt.Sprintf("%s (%s)", ref.Name, res.message))
			}
			continue
		}

		res.backend.Weight = weight
		status.Resolved = true
		status.ServiceURL = res.serviceURL
		out.statuses = append(out.statuses, status)
		out.backends = append(out.backends, *res.backend)
	}

	return out, nil
}

// resolveBackend resolves one backendRef to the data-plane Service that
// serves it in the binding's environment. It is the single seam between the
// binding and how a backend is located, so the strategy behind a kind can
// change without touching the pipeline or templates. Errors are reserved for
// transient API failures; a backend that cannot be used is a resolution with a
// reason.
func (r *Reconciler) resolveBackend(
	ctx context.Context,
	binding *openchoreov1alpha1.EndpointReleaseBinding,
	ref openchoreov1alpha1.EndpointBackendRef,
	dpNamespace string,
) (backendResolution, error) {
	switch {
	case ref.Kind == openchoreov1alpha1.EndpointBackendKindComponent && ref.Component != nil:
		return r.resolveComponentBackend(ctx, binding, ref, dpNamespace)
	default:
		// The CRD admits only Component today; reported rather than returned as
		// an error so a kind this controller predates cannot wedge the binding.
		return backendResolution{
			reason:  BackendReasonUnsupportedKind,
			message: fmt.Sprintf("backend kind %q is not supported", ref.Kind),
		}, nil
	}
}

// resolveComponentBackend resolves a Component backend through the
// ReleaseBinding of the backend's release line in the environment, reading the
// in-cluster address its status reports for the workload endpoint. This reuses
// the lookup the connection/dependency path already relies on. A backend
// without a release line routes to the component's main line, as a component
// reference does everywhere else.
//
// TODO: resolve by selector equality instead: compute the line's podSelectors
// for (component, line, environment) independently and match them against the
// Services on its RenderedRelease. That does not depend on how a ComponentType
// names or shapes its Service.
func (r *Reconciler) resolveComponentBackend(
	ctx context.Context,
	binding *openchoreov1alpha1.EndpointReleaseBinding,
	ref openchoreov1alpha1.EndpointBackendRef,
	dpNamespace string,
) (backendResolution, error) {
	env := binding.Spec.Environment
	component := ref.Component.Name
	line := ref.Component.ReleaseLine
	endpoint := ref.Component.Endpoint
	unresolved := func(reason, format string, args ...any) (backendResolution, error) {
		return backendResolution{reason: reason, message: fmt.Sprintf(format, args...)}, nil
	}
	// subject names the backend's line in messages: the component for its
	// main line, "release line L of component C" otherwise.
	subject := fmt.Sprintf("component %q", component)
	if line != "" {
		subject = fmt.Sprintf("release line %q of component %q", line, component)
	}

	var rbList openchoreov1alpha1.ReleaseBindingList
	if err := r.List(ctx, &rbList,
		client.InNamespace(binding.Namespace),
		client.MatchingFields{controller.IndexKeyReleaseBindingOwnerEnv: controller.MakeReleaseBindingOwnerEnvKey(
			binding.Spec.Owner.ProjectName, component, env)},
	); err != nil {
		return backendResolution{}, fmt.Errorf("list ReleaseBindings for component %q: %w", component, err)
	}
	// The index covers every line of the component; keep the backend's own.
	bindings := slices.DeleteFunc(rbList.Items, func(rb openchoreov1alpha1.ReleaseBinding) bool {
		return rb.Spec.Owner.ReleaseLine != line
	})

	switch len(bindings) {
	case 0:
		if line != "" {
			exists, err := r.releaseLineExists(ctx, binding.Namespace, binding.Spec.Owner.ProjectName, component, line)
			if err != nil {
				return backendResolution{}, err
			}
			if !exists {
				return unresolved(BackendReasonReleaseLineNotFound, "component %q has no release line %q", component, line)
			}
		}
		return unresolved(BackendReasonNotDeployed, "%s is not deployed in environment %q", subject, env)
	case 1:
	default:
		return unresolved(BackendReasonAmbiguous, "%s has %d ReleaseBindings in environment %q",
			subject, len(bindings), env)
	}

	rb := &bindings[0]
	if rb.Spec.State == openchoreov1alpha1.ReleaseStateUndeploy {
		return unresolved(BackendReasonNotDeployed, "%s is undeployed in environment %q", subject, env)
	}

	var epStatus *openchoreov1alpha1.EndpointURLStatus
	for i := range rb.Status.Endpoints {
		if rb.Status.Endpoints[i].Name == endpoint {
			epStatus = &rb.Status.Endpoints[i]
			break
		}
	}
	if epStatus == nil || epStatus.ServiceURL == nil {
		return unresolved(BackendReasonEndpointNotAvailable,
			"endpoint %q of %s has no in-cluster address in environment %q yet", endpoint, subject, env)
	}
	if !isRoutableProtocol(epStatus.Type) {
		return unresolved(BackendReasonUnsupportedProtocol,
			"endpoint %q of %s uses protocol %s, which no route can carry", endpoint, subject, epStatus.Type)
	}

	serviceName, serviceNamespace, ok := splitServiceHost(epStatus.ServiceURL.Host)
	if !ok {
		return unresolved(BackendReasonEndpointNotAvailable,
			"endpoint %q of %s has an unrecognized service host %q", endpoint, subject, epStatus.ServiceURL.Host)
	}
	// Routes render into the project's shared data-plane namespace and
	// reference backend Services without a ReferenceGrant, so the backend must
	// live in that namespace too.
	if serviceNamespace != dpNamespace {
		return unresolved(BackendReasonNamespaceMismatch,
			"endpoint %q of %s is served from namespace %q, but routes render into %q",
			endpoint, subject, serviceNamespace, dpNamespace)
	}

	url := epStatus.ServiceURL.DeepCopy()
	return backendResolution{
		backend: &endpointpipeline.Backend{
			Name:          ref.Name,
			Kind:          string(ref.Kind),
			ComponentName: component,
			Endpoint:      endpoint,
			Protocol:      string(epStatus.Type),
			ServiceName:   serviceName,
			Namespace:     serviceNamespace,
			Port:          url.Port,
			ServiceURL:    formatURL(url),
			BasePath:      basePathOf(url),
		},
		serviceURL: url,
	}, nil
}

// releaseLineExists reports whether the component has the named release line.
// A ReleaseLine is named after its line and belongs to one component, so a
// line of another component by that name does not count.
func (r *Reconciler) releaseLineExists(ctx context.Context, namespace, project, component, line string) (bool, error) {
	rl := &openchoreov1alpha1.ReleaseLine{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: line}, rl); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get ReleaseLine %q: %w", line, err)
	}
	return rl.Spec.Owner.ProjectName == project && rl.Spec.Owner.ComponentName == component, nil
}

// isRoutableProtocol reports whether any route kind can carry the protocol.
// Mirrors the ReleaseBinding controller's routeKindCompat: HTTP-family and gRPC
// ride HTTPRoute/GRPCRoute, TCP rides TLSRoute, and nothing carries UDP. A type
// that supports a narrower set says so through its validations, since the
// protocol reaches templates as backends[].protocol.
func isRoutableProtocol(p openchoreov1alpha1.EndpointProtocol) bool {
	return p != openchoreov1alpha1.EndpointProtocolUDP
}

// splitServiceHost splits "<svc>.<ns>.svc.cluster.local" into its Service name
// and namespace.
func splitServiceHost(host string) (name, namespace string, ok bool) {
	suffix := "." + clusterLocalSuffix
	if !strings.HasSuffix(host, suffix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimSuffix(host, suffix), ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// basePathOf returns the path the backend serves under, defaulting to "/".
func basePathOf(u *openchoreov1alpha1.EndpointURL) string {
	if u.Path == "" {
		return "/"
	}
	return u.Path
}

// formatURL renders an EndpointURL as scheme://host:port/path.
func formatURL(u *openchoreov1alpha1.EndpointURL) string {
	var b strings.Builder
	if u.Scheme != "" {
		b.WriteString(u.Scheme)
		b.WriteString("://")
	}
	b.WriteString(u.Host)
	if u.Port != 0 {
		fmt.Fprintf(&b, ":%d", u.Port)
	}
	if u.Path != "" && u.Path != "/" {
		if !strings.HasPrefix(u.Path, "/") {
			b.WriteString("/")
		}
		b.WriteString(u.Path)
	}
	return b.String()
}
