// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ObservabilityAlertsNotificationChannel implements observability alerts notification channel operations
type ObservabilityAlertsNotificationChannel struct{}

// New creates a new observability alerts notification channel implementation
func New() *ObservabilityAlertsNotificationChannel {
	return &ObservabilityAlertsNotificationChannel{}
}

// List lists all observability alerts notification channels in a namespace
func (o *ObservabilityAlertsNotificationChannel) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceObservabilityAlertsNotificationChannel, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ObservabilityAlertsNotificationChannel, string, error) {
		p := &gen.ListObservabilityAlertsNotificationChannelsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.ListObservabilityAlertsNotificationChannels(ctx, params.Namespace, p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
	if err != nil {
		return err
	}
	return printList(items)
}

// Get retrieves a single observability alerts notification channel and outputs it as YAML
func (o *ObservabilityAlertsNotificationChannel) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceObservabilityAlertsNotificationChannel, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetObservabilityAlertsNotificationChannel(ctx, params.Namespace, params.ChannelName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal observability alerts notification channel to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single observability alerts notification channel
func (o *ObservabilityAlertsNotificationChannel) Delete(params DeleteParams) error {
	if err := validation.ValidateParams(validation.CmdDelete, validation.ResourceObservabilityAlertsNotificationChannel, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := c.DeleteObservabilityAlertsNotificationChannel(ctx, params.Namespace, params.ChannelName); err != nil {
		return err
	}

	fmt.Printf("ObservabilityAlertsNotificationChannel '%s' deleted\n", params.ChannelName)
	return nil
}

func printList(items []gen.ObservabilityAlertsNotificationChannel) error {
	if len(items) == 0 {
		fmt.Println("No observability alerts notification channels found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, ch := range items {
		age := ""
		if ch.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*ch.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			ch.Metadata.Name,
			age)
	}

	return w.Flush()
}
