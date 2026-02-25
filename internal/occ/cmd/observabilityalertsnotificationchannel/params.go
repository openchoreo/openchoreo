// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

// ListParams defines parameters for listing observability alerts notification channels
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single observability alerts notification channel
type GetParams struct {
	Namespace   string
	ChannelName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single observability alerts notification channel
type DeleteParams struct {
	Namespace   string
	ChannelName string
}

func (p DeleteParams) GetNamespace() string   { return p.Namespace }
func (p DeleteParams) GetChannelName() string { return p.ChannelName }
