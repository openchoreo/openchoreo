// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteagent

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// valueReader reads one authorized key from a Secret or ConfigMap in the agent's own
// namespace. Implemented by *k8sValueReader; stubbed in tests.
type valueReader interface {
	read(ctx context.Context, grant remoteconnect.SecretGrant) ([]byte, error)
}

// k8sValueReader reads through the Kubernetes API using the agent's ServiceAccount. The
// token grants `get` on exactly the object names the control plane's provisioner put in
// the agent's Role, so an object no session asked for is refused by the API server even
// if the agent were induced to ask for it.
type k8sValueReader struct {
	client    kubernetes.Interface
	namespace string
}

// newK8sValueReader builds a reader from the in-cluster ServiceAccount credentials.
// Returns an error when the agent is not running in a cluster, which is not fatal to the
// agent: it serves tunnels and refuses fetches.
func newK8sValueReader(namespace string) (*k8sValueReader, error) {
	if namespace == "" {
		return nil, fmt.Errorf("remote-agent: namespace is required to read values")
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("remote-agent: in-cluster config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("remote-agent: build kubernetes client: %w", err)
	}
	return &k8sValueReader{client: cs, namespace: namespace}, nil
}

// read fetches grant's key. The namespace comes from the agent's own configuration, never
// from the grant: the grant is checked against it by the caller, and using the agent's
// own value here means a mismatch cannot become a cross-namespace read even if that
// check were ever removed.
func (r *k8sValueReader) read(ctx context.Context, grant remoteconnect.SecretGrant) ([]byte, error) {
	switch grant.SourceKind {
	case remoteconnect.SourceKindSecret:
		obj, err := r.client.CoreV1().Secrets(r.namespace).Get(ctx, grant.SourceName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		v, ok := obj.Data[grant.SourceKey]
		if !ok {
			return nil, fmt.Errorf("secret %q has no key %q", grant.SourceName, grant.SourceKey)
		}
		return v, nil
	case remoteconnect.SourceKindConfigMap:
		obj, err := r.client.CoreV1().ConfigMaps(r.namespace).Get(ctx, grant.SourceName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if v, ok := obj.Data[grant.SourceKey]; ok {
			return []byte(v), nil
		}
		// binaryData holds keys whose content is not valid UTF-8; a file binding may
		// legitimately reference one.
		if v, ok := obj.BinaryData[grant.SourceKey]; ok {
			return v, nil
		}
		return nil, fmt.Errorf("configmap %q has no key %q", grant.SourceName, grant.SourceKey)
	default:
		return nil, fmt.Errorf("unsupported source kind %q", grant.SourceKind)
	}
}
