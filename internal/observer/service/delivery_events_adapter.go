// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
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
// deliveryEventsPageSize; a window holding more than that returns a `total`
// exceeding the events returned, which is how the caller learns it was read
// short. The aggregator then resumes from where this read stopped on its next
// tick rather than this call looping until the window is drained.
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
		// 501 is the contract's "capability unavailable", not a failure: this
		// adapter cannot serve an unscoped sweep at all. Report it as such so the
		// aggregator stops asking rather than failing a tick every interval.
		if errors.Is(err, ErrEventsNotImplemented) {
			return nil, false, fmt.Errorf("%w: %w", aggregator.ErrEventsSourceUnavailable, err)
		}
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

	// The window was fully read when the match count equals what came back. The
	// comparison is against equality rather than "total <= len", so that a total
	// the adapter understated -- omitted entirely, or capped below limit+1 as the
	// contract forbids -- reads as incomplete and costs a re-read, instead of
	// advancing the watermark past events nobody saw.
	//
	// A total that overshoots because the adapter extended the page past limit to
	// avoid splitting a timestamp group also reads as incomplete: one extra query,
	// no loss.
	complete := result.Total == len(out)
	return out, complete, nil
}
