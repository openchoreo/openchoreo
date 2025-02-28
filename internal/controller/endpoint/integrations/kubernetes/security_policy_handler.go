package kubernetes

import (
	"context"
	"errors"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	choreov1 "github.com/wso2-enterprise/choreo-cp-declarative-api/api/v1"
	"github.com/wso2-enterprise/choreo-cp-declarative-api/internal/dataplane"
)

// TODO Add error logging

type SecurityPolicyHandler struct {
	client client.Client
}

var _ dataplane.ResourceHandler[dataplane.EndpointContext] = (*SecurityPolicyHandler)(nil)

func (h *SecurityPolicyHandler) Name() string {
	return "SecurityPolicy"
}

func (h *SecurityPolicyHandler) IsRequired(ctx *dataplane.EndpointContext) bool {
	if secSchemes := ctx.Endpoint.Spec.APISettings.SecuritySchemes; secSchemes != nil {
		for _, scheme := range secSchemes {
			return scheme == choreov1.Oauth
		}
	}
	return false
}

func (h *SecurityPolicyHandler) GetCurrentState(ctx context.Context, epCtx *dataplane.EndpointContext) (interface{}, error) {
	namespace := makeNamespaceName(epCtx)
	name := makeHTTPRouteName(epCtx)
	out := &egv1a1.SecurityPolicy{}
	err := h.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, out)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *SecurityPolicyHandler) Create(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	securityPolicy := makeSecurityPolicy(epCtx)
	return h.client.Create(ctx, securityPolicy)
}

func (h *SecurityPolicyHandler) Update(ctx context.Context, epCtx *dataplane.EndpointContext, currentState interface{}) error {
	current, ok := currentState.(*egv1a1.SecurityPolicy)
	if !ok {
		return errors.New("failed to cast current state to SecurityPolicy")
	}
	new := makeSecurityPolicy(epCtx)
	if shouldUpdate(current, new) {
		new.ResourceVersion = current.ResourceVersion
		return h.client.Update(ctx, new)
	}
	return nil
}

func NewSecurityPolicyHandler(client client.Client) dataplane.ResourceHandler[dataplane.EndpointContext] {
	return &SecurityPolicyHandler{
		client: client,
	}
}

func (h *SecurityPolicyHandler) Delete(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	return nil
}

func makeSecurityPolicy(epCtx *dataplane.EndpointContext) *egv1a1.SecurityPolicy {
	return &egv1a1.SecurityPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeHTTPRouteName(epCtx),
			Namespace: makeNamespaceName(epCtx),
			Labels:    makeWorkloadLabels(epCtx),
		},
		Spec: makeSecurityPolicySpec(epCtx),
	}
}

func makeSecurityPolicySpec(epCtx *dataplane.EndpointContext) egv1a1.SecurityPolicySpec {
	// Find a better way TODO
	// idpUrl := "idp.choreo-system.svc.cluster.local:8080/.well-known/jwks.json"
	return egv1a1.SecurityPolicySpec{
		JWT: &egv1a1.JWT{
			Providers: []egv1a1.JWTProvider{
				{
					Name:       "default",
					RemoteJWKS: epCtx.Environment.Spec.Gateway.Security.RemoteJWKS, // Get from environment
				},
			},
		},
		PolicyTargetReferences: egv1a1.PolicyTargetReferences{
			TargetRefs: []gwapiv1a2.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gwapiv1a2.LocalPolicyTargetReference{
						Group: gwapiv1.GroupName,
						Kind:  gwapiv1.Kind("HTTPRoute"),
						Name:  gwapiv1a2.ObjectName(makeHTTPRouteName(epCtx)),
					},
				},
			},
		},
	}
}

func shouldUpdate(current, new *egv1a1.SecurityPolicy) bool {
	// Compare the labels
	if !cmp.Equal(extractManagedLabels(current.Labels), extractManagedLabels(new.Labels)) {
		return true
	}

	return !cmp.Equal(current.Spec, new.Spec, cmpopts.EquateEmpty())
}
