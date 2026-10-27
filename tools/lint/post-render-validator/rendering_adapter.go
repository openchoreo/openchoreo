// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"context"
	"fmt"

	yamlv3 "gopkg.in/yaml.v3"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	componentpipeline "github.com/openchoreo/openchoreo/internal/pipeline/component"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

// Bundle holds the set of OpenChoreo resources needed to render a Component.
type Bundle struct {
	ComponentType  *v1alpha1.ComponentType
	Component      *v1alpha1.Component
	Traits         []v1alpha1.Trait
	Workload       *v1alpha1.Workload
	ReleaseBinding *v1alpha1.ReleaseBinding
	Environment    *v1alpha1.Environment
	DataPlane      *v1alpha1.DataPlane
}

// decodeTyped unmarshals a DocumentNode's root into a typed struct T.
// It decodes from doc.Source for reliable typed unmarshaling.
func decodeTyped[T any](doc *parser.DocumentNode) (T, error) {
	var zero T
	if doc == nil {
		return zero, fmt.Errorf("document is nil")
	}
	var data []byte
	if doc.Root != nil {
		// Decode the individual document, not doc.Source: Source is the whole
		// file and would make every document in a multi-document YAML file
		// decode as the first one.
		b, err := yamlv3.Marshal(doc.Root)
		if err != nil {
			return zero, fmt.Errorf("marshal doc root: %w", err)
		}
		data = b
	} else if len(doc.Source) > 0 {
		data = doc.Source
	} else {
		return zero, fmt.Errorf("no source data")
	}
	var v T
	if err := sigsyaml.Unmarshal(data, &v); err != nil {
		return zero, fmt.Errorf("unmarshal to typed: %w", err)
	}
	return v, nil
}

// BundleFromDocuments groups parser.DocumentNode entries into a Bundle keyed by Kind.
// Cluster variants are converted to their namespaced equivalents where applicable.
func BundleFromDocuments(docs []*parser.DocumentNode) (*Bundle, error) {
	b := &Bundle{
		Traits: make([]v1alpha1.Trait, 0, 4),
	}
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		switch doc.Kind {
		case "ComponentType":
			ct, err := decodeTyped[v1alpha1.ComponentType](doc)
			if err != nil {
				return nil, fmt.Errorf("decode ComponentType: %w", err)
			}
			b.ComponentType = &ct
		case "ClusterComponentType":
			cct, err := decodeTyped[v1alpha1.ClusterComponentType](doc)
			if err != nil {
				return nil, fmt.Errorf("decode ClusterComponentType: %w", err)
			}
			ct := v1alpha1.ComponentType{
				TypeMeta:   cct.TypeMeta,
				ObjectMeta: cct.ObjectMeta,
				Spec:       cct.Spec.ToComponentTypeSpec(),
			}
			b.ComponentType = &ct
		case "Trait":
			t, err := decodeTyped[v1alpha1.Trait](doc)
			if err != nil {
				return nil, fmt.Errorf("decode Trait: %w", err)
			}
			b.Traits = append(b.Traits, t)
		case "ClusterTrait":
			// Try to decode as ClusterTrait first
			ctrait, err := decodeTyped[v1alpha1.ClusterTrait](doc)
			if err == nil {
				t := v1alpha1.Trait{
					TypeMeta:   ctrait.TypeMeta,
					ObjectMeta: ctrait.ObjectMeta,
					Spec:       v1alpha1.TraitSpec(ctrait.Spec),
				}
				b.Traits = append(b.Traits, t)
				continue
			}
			// Fallback: decode as Trait
			t, err := decodeTyped[v1alpha1.Trait](doc)
			if err != nil {
				return nil, fmt.Errorf("decode ClusterTrait: %w", err)
			}
			b.Traits = append(b.Traits, t)
		case "Component":
			c, err := decodeTyped[v1alpha1.Component](doc)
			if err != nil {
				return nil, fmt.Errorf("decode Component: %w", err)
			}
			b.Component = &c
		case "Workload":
			w, err := decodeTyped[v1alpha1.Workload](doc)
			if err != nil {
				return nil, fmt.Errorf("decode Workload: %w", err)
			}
			b.Workload = &w
		case "ReleaseBinding":
			rb, err := decodeTyped[v1alpha1.ReleaseBinding](doc)
			if err != nil {
				return nil, fmt.Errorf("decode ReleaseBinding: %w", err)
			}
			b.ReleaseBinding = &rb
		case "Environment":
			e, err := decodeTyped[v1alpha1.Environment](doc)
			if err != nil {
				return nil, fmt.Errorf("decode Environment: %w", err)
			}
			b.Environment = &e
		case "DataPlane":
			dp, err := decodeTyped[v1alpha1.DataPlane](doc)
			if err != nil {
				return nil, fmt.Errorf("decode DataPlane: %w", err)
			}
			b.DataPlane = &dp
		default:
			// Skip unknown kinds
		}
	}
	return b, nil
}

// synthesizeMetadata creates a minimal required MetadataContext for linting.
// Resource names and namespaces are taken from the Component when available so
// rule messages refer to the real objects rather than lint placeholders.
func synthesizeMetadata(b *Bundle) pipelinecontext.MetadataContext {
	componentName := "lint-component"
	namespace := "lint-ns"
	projectName := "lint-project"
	dataPlaneName := "lint-dp"
	environmentName := "lint-env"
	labels := map[string]string{}
	annotations := map[string]string{}

	if b != nil && b.Component != nil {
		if b.Component.Name != "" {
			componentName = b.Component.Name
		}
		if b.Component.Namespace != "" {
			namespace = b.Component.Namespace
		}
		if b.Component.Spec.Owner.ProjectName != "" {
			projectName = b.Component.Spec.Owner.ProjectName
		}
		for k, v := range b.Component.Labels {
			labels[k] = v
		}
		for k, v := range b.Component.Annotations {
			annotations[k] = v
		}
	}
	if b != nil && b.DataPlane != nil && b.DataPlane.Name != "" {
		dataPlaneName = b.DataPlane.Name
	}
	if b != nil && b.Environment != nil && b.Environment.Name != "" {
		environmentName = b.Environment.Name
	}

	return pipelinecontext.MetadataContext{
		Name:               componentName,
		Namespace:          namespace,
		ComponentName:      componentName,
		ComponentUID:       "lint-uid-1",
		ComponentNamespace: namespace,
		ProjectName:        projectName,
		ProjectUID:         "lint-uid-2",
		DataPlaneName:      dataPlaneName,
		DataPlaneUID:       "lint-uid-3",
		EnvironmentName:    environmentName,
		EnvironmentUID:     "lint-uid-4",
		Labels:             labels,
		Annotations:        annotations,
		PodSelectors:       map[string]string{"openchoreo.dev/component": componentName},
	}
}

// RenderInput converts the Bundle into a componentpipeline.RenderInput.
func (b *Bundle) RenderInput() (*componentpipeline.RenderInput, error) {
	meta := synthesizeMetadata(b)
	// Ensure required pointers exist for pipeline validation
	if b.Environment == nil {
		b.Environment = &v1alpha1.Environment{}
	}
	if b.DataPlane == nil {
		b.DataPlane = &v1alpha1.DataPlane{}
	}
	// Without a ReleaseBinding there is no Environment or DataPlane to provide
	// gateway configuration. ComponentType templates commonly read
	// gateway.ingress.external.name and gateway.ingress.internal.name, so fill
	// in placeholders for any endpoint the DataPlane does not define. Real
	// Environment gateway values still take precedence when merging.
	namespace := "default"
	if b.Component != nil && b.Component.Namespace != "" {
		namespace = b.Component.Namespace
	}
	ensureGatewayEndpoints(&b.DataPlane.Spec.Gateway, namespace)
	ri := &componentpipeline.RenderInput{
		ComponentType:  b.ComponentType,
		Component:      b.Component,
		Traits:         b.Traits,
		Workload:       b.Workload,
		ReleaseBinding: b.ReleaseBinding,
		Environment:    b.Environment,
		DataPlane:      b.DataPlane,
		Metadata:       meta,
	}
	return ri, nil
}

func ensureGatewayEndpoints(gw *v1alpha1.GatewaySpec, namespace string) {
	if gw.Ingress == nil {
		gw.Ingress = &v1alpha1.GatewayNetworkSpec{}
	}
	if gw.Ingress.External == nil {
		gw.Ingress.External = &v1alpha1.GatewayEndpointSpec{Name: "lint-gateway", Namespace: namespace}
	}
	if gw.Ingress.Internal == nil {
		gw.Ingress.Internal = &v1alpha1.GatewayEndpointSpec{Name: "lint-gateway", Namespace: namespace}
	}
}

// Render renders the bundle using the real pipeline in lint mode.
func Render(ctx context.Context, input *componentpipeline.RenderInput) (*componentpipeline.RenderOutput, []componentpipeline.PostRenderFailure, error) {
	p := componentpipeline.NewPipeline()
	return p.RenderForLint(ctx, input)
}

// RenderBundle renders a bundle directly.
func RenderBundle(ctx context.Context, b *Bundle) (*componentpipeline.RenderOutput, []componentpipeline.PostRenderFailure, error) {
	if b == nil {
		return nil, nil, fmt.Errorf("bundle is nil")
	}
	ri, err := b.RenderInput()
	if err != nil {
		return nil, nil, err
	}
	return Render(ctx, ri)
}
