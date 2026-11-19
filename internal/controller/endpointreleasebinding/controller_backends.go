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

	// policyTargets are the pods each resolved backend runs on, in spec order,
	// for the network policies that admit the Endpoint's visibility to them.
	policyTargets []policyTarget
}

// policyTarget is what a backend's network policy selects and opens: the pods
// of the backend's component line in the environment, on the workload
// endpoint's container port.
type policyTarget struct {
	backendName  string
	podSelectors map[string]string
	port         int32
	protocol     openchoreov1alpha1.EndpointProtocol
}

// backendResolution is the outcome of resolving one backendRef. Exactly one of
// backend or reason is set: reason explains why the backend cannot receive
// traffic in this environment.
type backendResolution struct {
	backend    *endpointpipeline.Backend
	serviceURL *openchoreov1alpha1.EndpointURL
	target     policyTarget
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
	md endpointpipeline.MetadataContext,
) (*resolvedBackends, error) {
	out := &resolvedBackends{}

	for _, ref := range release.Spec.BackendRefs {
		weight := int32(1)
		if ref.Weight != nil {
			weight = *ref.Weight
		}

		res, err := r.resolveBackend(ctx, binding, ref, md)
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
		out.policyTargets = append(out.policyTargets, res.target)
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
	md endpointpipeline.MetadataContext,
) (backendResolution, error) {
	switch {
	case ref.Kind == openchoreov1alpha1.EndpointBackendKindComponent && ref.Component != nil:
		return r.resolveComponentBackend(ctx, binding, ref, md)
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
// ReleaseBinding of the backend's release line in the environment. A backend
// without a release line routes to the component's main line, as a component
// reference does everywhere else.
//
// The workload endpoint (its ports, protocol and base path) comes from the
// release the line runs, and the pods from the component's and the line's
// UIDs. Those are all a type needs to render its own Service, and what the
// backend's network policy selects and opens.
//
// The component's own Service is optional. When the ComponentType renders one,
// the ReleaseBinding's status reports its in-cluster address for the endpoint,
// and the backend carries its name, URL, port and protocol from that status
// entry, so routes target the Service as it was rendered. When the
// ReleaseBinding has computed its endpoints for its current spec without
// reporting one, the ComponentType renders no Service for the endpoint, and
// the backend resolves without it. Until then the address may still come, so
// the backend waits. This does not wait on the ReleaseBinding's own
// connections or resource dependencies: like a component dependency reading
// the same status, an Endpoint resolves once the backend has rendered.
//
// TODO: once ComponentTypes no longer render Services, drop the ReleaseBinding
// address and resolve from the release and the pod selectors alone.
func (r *Reconciler) resolveComponentBackend(
	ctx context.Context,
	binding *openchoreov1alpha1.EndpointReleaseBinding,
	ref openchoreov1alpha1.EndpointBackendRef,
	md endpointpipeline.MetadataContext,
) (backendResolution, error) {
	dpNamespace := md.Namespace
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

	var releaseLine *openchoreov1alpha1.ReleaseLine
	if line != "" {
		var err error
		if releaseLine, err = r.getReleaseLine(ctx, binding.Namespace, binding.Spec.Owner.ProjectName, component, line); err != nil {
			return backendResolution{}, err
		}
		if releaseLine == nil {
			return unresolved(BackendReasonReleaseLineNotFound, "component %q has no release line %q", component, line)
		}
	}

	switch len(bindings) {
	case 0:
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

	// The component's own Service, when its ComponentType renders one. Its
	// address and protocol come from the same status entry, so they describe
	// the same rendered release even while the ReleaseBinding is rolling out a
	// new one.
	var serviceURL *openchoreov1alpha1.EndpointURL
	var serviceProtocol openchoreov1alpha1.EndpointProtocol
	for i := range rb.Status.Endpoints {
		if rb.Status.Endpoints[i].Name == endpoint {
			serviceURL = rb.Status.Endpoints[i].ServiceURL
			serviceProtocol = rb.Status.Endpoints[i].Type
			break
		}
	}
	if serviceURL == nil && rb.Spec.IsObserve() {
		// Not a wait: nothing changes until the binding leaves Observe mode.
		return unresolved(BackendReasonObservedComponent,
			"endpoint %q of %s has no in-cluster address in environment %q, and its ReleaseBinding is in "+
				"Observe mode, so no Service can be rendered in front of its pods", endpoint, subject, env)
	}
	if serviceURL == nil && !hasObservedCurrentEndpoints(rb) {
		return unresolved(BackendReasonEndpointNotAvailable,
			"endpoint %q of %s has no in-cluster address in environment %q yet", endpoint, subject, env)
	}

	var serviceName string
	if serviceURL != nil {
		var serviceNamespace string
		var ok bool
		serviceName, serviceNamespace, ok = splitServiceHost(serviceURL.Host)
		if !ok {
			return unresolved(BackendReasonEndpointNotAvailable,
				"endpoint %q of %s has an unrecognized service host %q", endpoint, subject, serviceURL.Host)
		}
		// Routes render into the project's shared data-plane namespace and
		// reference backend Services without a ReferenceGrant, so the backend
		// must live in that namespace too.
		if serviceNamespace != dpNamespace {
			return unresolved(BackendReasonNamespaceMismatch,
				"endpoint %q of %s is served from namespace %q, but routes render into %q",
				endpoint, subject, serviceNamespace, dpNamespace)
		}
		serviceURL = serviceURL.DeepCopy()
	}

	comp := &openchoreov1alpha1.Component{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: binding.Namespace, Name: component}, comp); err != nil {
		if apierrors.IsNotFound(err) {
			return unresolved(BackendReasonNotDeployed, "component %q not found", component)
		}
		return backendResolution{}, fmt.Errorf("get Component %q: %w", component, err)
	}
	wep, err := r.releasedEndpoint(ctx, rb, endpoint)
	if err != nil {
		return backendResolution{}, err
	}
	if wep == nil {
		return unresolved(BackendReasonEndpointNotAvailable,
			"endpoint %q of %s is not in the release deployed in environment %q", endpoint, subject, env)
	}
	protocol := wep.EffectiveProtocol()
	if serviceURL != nil && serviceProtocol != "" {
		protocol = serviceProtocol
	}
	if !isRoutableProtocol(protocol) {
		return unresolved(BackendReasonUnsupportedProtocol,
			"endpoint %q of %s uses protocol %s, which no route can carry", endpoint, subject, protocol)
	}
	targetPort := wep.TargetPort
	if targetPort == 0 {
		targetPort = wep.Port
	}

	selectorInput := controller.PodSelectorInput{
		NamespaceName:   binding.Namespace,
		ProjectName:     binding.Spec.Owner.ProjectName,
		ComponentName:   component,
		EnvironmentName: env,
		ComponentUID:    string(comp.UID),
		EnvironmentUID:  md.EnvironmentUID,
		ProjectUID:      md.ProjectUID,
	}
	if releaseLine != nil {
		selectorInput.ReleaseLine = line
		selectorInput.ReleaseLineUID = string(releaseLine.UID)
	}

	podSelectors := controller.ComponentPodSelectors(selectorInput)

	backend := &endpointpipeline.Backend{
		Name:          ref.Name,
		Kind:          string(ref.Kind),
		ComponentName: component,
		Endpoint:      endpoint,
		Protocol:      string(protocol),
		Namespace:     dpNamespace,
		Port:          wep.Port,
		TargetPort:    targetPort,
		PodSelectors:  podSelectors,
		BasePath:      basePathOf(wep.BasePath),
	}
	if serviceURL != nil {
		backend.ServiceName = serviceName
		backend.Port = serviceURL.Port
		backend.ServiceURL = formatURL(serviceURL)
		backend.BasePath = basePathOf(serviceURL.Path)
	}
	return backendResolution{
		backend:    backend,
		serviceURL: serviceURL,
		target: policyTarget{
			backendName:  ref.Name,
			podSelectors: podSelectors,
			port:         targetPort,
			protocol:     protocol,
		},
	}, nil
}

// hasObservedCurrentEndpoints reports whether the ReleaseBinding has computed
// status.endpoints from the release its current spec deploys. An endpoint
// without a service address then has no Service from the ComponentType, rather
// than one not reported yet.
//
// The ReleaseBinding records this as soon as the release is rendered, before
// it waits on its own connections and resource dependencies, so a backend
// never waits on what the backend component depends on. Waiting on
// ReleaseSynced instead would, and two components depending on each other
// through such Endpoints would never resolve.
//
// An Observe-mode binding never counts: it renders no release, and its pods
// are the observed workload's, which do not carry the pod selectors a type
// would render a Service with.
func hasObservedCurrentEndpoints(rb *openchoreov1alpha1.ReleaseBinding) bool {
	if rb.Spec.IsObserve() {
		return false
	}
	return rb.Status.EndpointsObservedGeneration != 0 && rb.Status.EndpointsObservedGeneration == rb.Generation
}

// getReleaseLine returns the component's release line, or nil when it has
// none by that name. A ReleaseLine is named after its line and belongs to one
// component, so a line of another component by that name does not count.
func (r *Reconciler) getReleaseLine(
	ctx context.Context, namespace, project, component, line string,
) (*openchoreov1alpha1.ReleaseLine, error) {
	rl := &openchoreov1alpha1.ReleaseLine{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: line}, rl); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get ReleaseLine %q: %w", line, err)
	}
	if rl.Spec.Owner.ProjectName != project || rl.Spec.Owner.ComponentName != component {
		return nil, nil
	}
	return rl, nil
}

// releasedEndpoint returns the workload endpoint in the release the
// ReleaseBinding deploys, or nil when the release or the endpoint is missing.
// A network policy is enforced at the pod, after a Service has forwarded port
// to targetPort, so policies open targetPort, which defaults to port as it
// does for the component's Service.
func (r *Reconciler) releasedEndpoint(
	ctx context.Context, rb *openchoreov1alpha1.ReleaseBinding, endpoint string,
) (*openchoreov1alpha1.WorkloadEndpoint, error) {
	if rb.Spec.ReleaseName == "" {
		return nil, nil
	}
	cr := &openchoreov1alpha1.ComponentRelease{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: rb.Namespace, Name: rb.Spec.ReleaseName}, cr); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get ComponentRelease %q: %w", rb.Spec.ReleaseName, err)
	}
	ep, ok := cr.Spec.Workload.Endpoints[endpoint]
	if !ok || ep.Port == 0 {
		return nil, nil
	}
	return &ep, nil
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
func basePathOf(path string) string {
	if path == "" {
		return "/"
	}
	return path
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
