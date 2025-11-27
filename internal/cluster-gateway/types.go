// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"time"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// Dispatcher is an interface that both Server and RemoteServerClient implement
// It allows sending cluster agent requests to agents
type Dispatcher interface {
	SendClusterAgentRequest(
		planeName string,
		requestType messaging.RequestType,
		identifier string,
		payload map[string]interface{},
		timeout time.Duration,
	) (*messaging.ClusterAgentResponse, error)
}
