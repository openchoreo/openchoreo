// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/aggregator"
	"github.com/openchoreo/openchoreo/internal/observer/api/logsadapterclientgen"
)

const (
	// deliveryEventsPageSize is the adapter's maximum page size.
	deliveryEventsPageSize = 1000
)

// deliveryEventReasons are the controller-emitted delivery lifecycle reasons
// the aggregator folds into deployment facts.
var deliveryEventReasons = []string{
	aggregator.ReasonDeploymentStarted,
	aggregator.ReasonDeploymentSucceeded,
	aggregator.ReasonDeploymentFailed,
	aggregator.ReasonDeploymentRecovered,
}

// FetchDeliveryEvents implements aggregator.EventsSource on the logs adapter: one
// reason-filtered read of controller-emitted delivery lifecycle events in
// [fromMs, toMs) across every namespace, in timestamp-ascending order.
//
// One request per call, with no paging loop. The adapter caps a single read at
// deliveryEventsPageSize, so a window holding more than that comes back with
// complete=false, and the aggregator resumes from where this read stopped on its
// next tick rather than this call looping until the window is drained.
func (p *LogsAdapter) FetchDeliveryEvents(
	ctx context.Context, fromMs, toMs int64,
) ([]aggregator.DeliveryEvent, bool, error) {
	reasons := deliveryEventReasons
	limit := deliveryEventsPageSize
	sortOrder := logsadapterclientgen.EventsQueryRequestSortOrderAsc

	adapterReq := logsadapterclientgen.EventsQueryRequest{
		StartTime: time.UnixMilli(fromMs).UTC(),
		EndTime:   time.UnixMilli(toMs).UTC(),
		Reasons:   &reasons,
		Limit:     &limit,
		SortOrder: &sortOrder,
	}

	resp, err := p.adapterClient.QueryEvents(ctx, adapterReq)
	if err != nil {
		return nil, false, fmt.Errorf("failed to call logs adapter delivery events query: %w", err)
	}
	result, err := func() (*logsadapterclientgen.EventsQueryResponse, error) {
		defer resp.Body.Close()
		if err := mapAdapterHTTPError(resp, "logs adapter"); err != nil {
			return nil, err
		}
		return decodeEventsResponse(resp)
	}()
	if err != nil {
		return nil, false, err
	}

	var out []aggregator.DeliveryEvent
	if result.Events != nil {
		out = make([]aggregator.DeliveryEvent, 0, len(*result.Events))
		for _, e := range *result.Events {
			event := aggregator.DeliveryEvent{
				Reason:  stringPtrVal(e.Reason),
				Message: stringPtrVal(e.Message),
			}
			if e.Timestamp != nil {
				event.TimestampMs = e.Timestamp.UnixMilli()
			}
			if e.Metadata != nil {
				event.Namespace = stringPtrVal(e.Metadata.NamespaceName)
				event.ProjectName = stringPtrVal(e.Metadata.ProjectName)
				event.ComponentName = stringPtrVal(e.Metadata.ComponentName)
				event.EnvironmentName = stringPtrVal(e.Metadata.EnvironmentName)
			}
			out = append(out, event)
		}
	}

	// Absent means not complete. An adapter that does not set the field makes the
	// caller resume and re-read, which is the safe direction -- treating silence as
	// "fully swept" would advance the watermark past events nobody read.
	complete := result.Complete != nil && *result.Complete
	return out, complete, nil
}
