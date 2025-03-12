/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package kubernetes

import (
	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/choreo-idp/choreo/internal/dataplane"
)

func MakeSecurityPolicy(epCtx *dataplane.EndpointContext, gwType GatewayType) *egv1a1.SecurityPolicy {
	return &egv1a1.SecurityPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeHTTPRouteName(epCtx, gwType),
			Namespace: MakeNamespaceName(epCtx),
			Labels:    MakeWorkloadLabels(epCtx),
		},
		Spec: MakeSecurityPolicySpec(epCtx, gwType),
	}
}

func MakeSecurityPolicySpec(epCtx *dataplane.EndpointContext, gwType GatewayType) egv1a1.SecurityPolicySpec {
	return egv1a1.SecurityPolicySpec{
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
						Kind:  gwapiv1.Kind("HTTPRoute"),
						Name:  gwapiv1a2.ObjectName(MakeHTTPRouteName(epCtx, gwType)),
					},
				},
			},
		},
	}
}
