// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	choreov1 "github.com/openchoreo/openchoreo/api/v1"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	"github.com/google/go-cmp/cmp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/openchoreo/openchoreo/internal/controller/endpoint/integrations/kubernetes/visibility"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

type SecurityPoliciesHandler struct {
	client     client.Client
	visibility visibility.VisibilityStrategy
}

var _ dataplane.ResourceHandler[dataplane.EndpointContext] = (*SecurityPoliciesHandler)(nil)

func (h *SecurityPoliciesHandler) Name() string {
	return "SecurityPolicy"
}

func (h *SecurityPoliciesHandler) IsRequired(ctx *dataplane.EndpointContext) bool {
	return h.visibility.IsSecurityPolicyRequired(ctx)
}

func (h *SecurityPoliciesHandler) GetCurrentState(ctx context.Context, epCtx *dataplane.EndpointContext) (interface{}, error) {
	namespace := makeNamespaceName(epCtx)
	securityPolicies := MakeSecurityPolicies(epCtx, h.visibility.GetGatewayType())
	var out []*egv1a1.SecurityPolicy

	for _, policy := range securityPolicies {
		name := policy.Name
		p := &egv1a1.SecurityPolicy{}
		err := h.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, p)
		if apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (h *SecurityPoliciesHandler) Create(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	policies := MakeSecurityPolicies(epCtx, h.visibility.GetGatewayType())
	for _, policy := range policies {
		if err := h.client.Create(ctx, policy); err != nil {
			return fmt.Errorf("error while creating SecurityPolicy %s: %w", policy.Name, err)
		}
	}
	return nil
}

func (h *SecurityPoliciesHandler) Update(ctx context.Context, epCtx *dataplane.EndpointContext, currentState interface{}) error {
	currentSecurityPolicies, ok := currentState.([]*egv1a1.SecurityPolicy)
	if !ok {
		return errors.New("failed to cast current state to the list of SecurityPolicies")
	}

	desiredSecurityPolicies := MakeSecurityPolicies(epCtx, h.visibility.GetGatewayType())

	// Create a map of current SecurityPolicies for an easier lookup
	currentSecurityPolicyMap := make(map[string]*egv1a1.SecurityPolicy)
	for _, currentPolicy := range currentSecurityPolicies {
		currentSecurityPolicyMap[currentPolicy.Name] = currentPolicy
	}

	// Create a map of desired SecurityPolicies for an easier lookup
	desiredSecurityPolicyMap := make(map[string]*egv1a1.SecurityPolicy)
	for _, desiredPolicy := range desiredSecurityPolicies {
		desiredSecurityPolicyMap[desiredPolicy.Name] = desiredPolicy
	}

	// Update each SecurityPolicy if needed
	for name, newSecurityPolicy := range desiredSecurityPolicyMap {
		currentSecurityPolicy, exists := currentSecurityPolicyMap[name]
		if !exists {
			// If the SecurityPolicy does not exist, create it
			if err := h.client.Create(ctx, newSecurityPolicy); err != nil {
				return fmt.Errorf("error while creating SecurityPolicy %s: %w", newSecurityPolicy.Name, err)
			}
		}

		// Check if the current SecurityPolicy needs to be updated
		if !cmp.Equal(currentSecurityPolicy.Spec, newSecurityPolicy.Spec) ||
			!cmp.Equal(extractManagedLabels(currentSecurityPolicy.Labels), extractManagedLabels(newSecurityPolicy.Labels)) {
			updatedSecurityPolicy := currentSecurityPolicy.DeepCopy()
			updatedSecurityPolicy.Spec = newSecurityPolicy.Spec
			updatedSecurityPolicy.Labels = newSecurityPolicy.Labels

			if err := h.client.Update(ctx, updatedSecurityPolicy); err != nil {
				return fmt.Errorf("error while updating SecurityPolicy %s: %w", updatedSecurityPolicy.Name, err)
			}
		}
	}

	// Delete SecurityPolicies that exist in the current state but not in the desired state
	for name, currentSecurityPolicy := range currentSecurityPolicyMap {
		if _, exists := desiredSecurityPolicyMap[name]; !exists {
			if err := h.client.Delete(ctx, currentSecurityPolicy); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("error while deleting SecurityPolicy %s: %w", currentSecurityPolicy.Name, err)
			}
		}
	}

	return nil
}

func NewSecurityPolicyHandler(client client.Client, visibility visibility.VisibilityStrategy) dataplane.ResourceHandler[dataplane.EndpointContext] {
	return &SecurityPoliciesHandler{
		client:     client,
		visibility: visibility,
	}
}

func (h *SecurityPoliciesHandler) Delete(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	namespace := makeNamespaceName(epCtx)
	labels := makeWorkloadLabels(epCtx, h.visibility.GetGatewayType())
	deleteAllOption := []client.DeleteAllOfOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}

	err := h.client.DeleteAllOf(ctx, &egv1a1.SecurityPolicy{}, deleteAllOption...)
	if err != nil {
		return fmt.Errorf("error while deleting SecurityPolicies: %w", err)
	}

	return nil
}

func MakeSecurityPolicies(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType) []*egv1a1.SecurityPolicy {
	out := make([]*egv1a1.SecurityPolicy, 0)

	policies := extractPoliciesFromCtx(epCtx, gwType)

	for _, policy := range policies {
		// Skip policies without specs or if not OAuth2 type
		if policy.PolicySpec == nil || policy.Type != "oauth2" {
			continue
		}

		// Skip if OAuth2 config is missing or JWT operations are not configured
		if policy.PolicySpec.OAuth2 == nil ||
			policy.PolicySpec.OAuth2.JWT.Authorization.Rest.Operations == nil {
			continue
		}

		// Generate separate security policies for each operation
		operations := policy.PolicySpec.OAuth2.JWT.Authorization.Rest.Operations
		for _, operation := range *operations {
			securityPolicy := makeSecurityPolicyForOperation(epCtx, &operation, gwType)
			out = append(out, securityPolicy)
		}
	}

	return out
}

func makeSecurityPolicyForOperation(epCtx *dataplane.EndpointContext, RESTOperation *choreov1.RESTOperation,
	gwType visibility.GatewayType) *egv1a1.SecurityPolicy {

	// Using the same name as HTTPRoute for consistency
	name := makeHTTPRouteNameForOperation(epCtx, gwType, string(RESTOperation.Method), RESTOperation.Target)
	actionDeny := egv1a1.AuthorizationActionDeny
	actionAllow := egv1a1.AuthorizationActionAllow

	// Convert RESTOperation.Scopes to []egv1a1.JWTScope
	jwtScopes := make([]egv1a1.JWTScope, len(RESTOperation.Scopes))
	for i, scope := range RESTOperation.Scopes {
		jwtScopes[i] = egv1a1.JWTScope(scope)
	}

	return &egv1a1.SecurityPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: makeNamespaceName(epCtx),
			Labels:    makeWorkloadLabels(epCtx, gwType),
		},
		Spec: egv1a1.SecurityPolicySpec{
			JWT: &egv1a1.JWT{
				Providers: []egv1a1.JWTProvider{
					{
						Name: "default",
						RemoteJWKS: egv1a1.RemoteJWKS{
							URI: epCtx.Environment.Spec.Gateway.Security.RemoteJWKS.URI,
						},
					},
				},
			},
			PolicyTargetReferences: egv1a1.PolicyTargetReferences{
				TargetRefs: []gwapiv1a2.LocalPolicyTargetReferenceWithSectionName{
					{
						LocalPolicyTargetReference: gwapiv1a2.LocalPolicyTargetReference{
							Group: gwapiv1.GroupName,
							Kind:  "HTTPRoute",
							Name:  gwapiv1a2.ObjectName(name),
						},
					},
				},
			},
			Authorization: &egv1a1.Authorization{
				Rules: []egv1a1.AuthorizationRule{
					{
						Principal: egv1a1.Principal{
							JWT: &egv1a1.JWTPrincipal{
								Provider: "default",
								Scopes:   jwtScopes,
							},
						},
						Action: actionAllow,
					},
				},
				DefaultAction: &actionDeny,
			},
		},
	}
}
