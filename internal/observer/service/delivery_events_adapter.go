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
	// deliveryEventsMaxPages bounds one sweep; a window with more pages is cut
	// short and the remainder is picked up by the aggregator's next tick (its
	// watermark only advances past what a tick returned).
	deliveryEventsMaxPages = 20
)

// deliveryEventReasons are the controller-emitted delivery lifecycle reasons
// the aggregator folds into deployment facts.
var deliveryEventReasons = []string{
	aggregator.ReasonDeploymentStarted,
	aggregator.ReasonDeploymentSucceeded,
	aggregator.ReasonDeploymentFailed,
	aggregator.ReasonDeploymentRecovered,
}

// FetchDeliveryEvents implements aggregator.EventsSource on the logs adapter:
// an unscoped, reason-filtered sweep of controller-emitted delivery lifecycle
// events in [fromMs, toMs), paged with the adapter's searchAfter cursor and
// returned in timestamp-ascending order.
func (p *LogsAdapter) FetchDeliveryEvents(ctx context.Context, fromMs, toMs int64) ([]aggregator.DeliveryEvent, error) {
	reasons := deliveryEventReasons
	limit := deliveryEventsPageSize
	sortOrder := logsadapterclientgen.EventsQueryRequestSortOrderAsc

	var out []aggregator.DeliveryEvent
	var cursor *string
	for page := 0; page < deliveryEventsMaxPages; page++ {
		adapterReq := logsadapterclientgen.EventsQueryRequest{
			StartTime:   time.UnixMilli(fromMs).UTC(),
			EndTime:     time.UnixMilli(toMs).UTC(),
			Reasons:     &reasons,
			Limit:       &limit,
			SortOrder:   &sortOrder,
			SearchAfter: cursor,
		}

		resp, err := p.adapterClient.QueryEvents(ctx, adapterReq)
		if err != nil {
			return nil, fmt.Errorf("failed to call logs adapter delivery events query: %w", err)
		}
		result, err := func() (*logsadapterclientgen.EventsQueryResponse, error) {
			defer resp.Body.Close()
			if err := mapAdapterHTTPError(resp, "logs adapter"); err != nil {
				return nil, err
			}
			return decodeEventsResponse(resp)
		}()
		if err != nil {
			return nil, err
		}

		if result.Events != nil {
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

		if result.NextCursor == nil || *result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}

	return out, nil
}
