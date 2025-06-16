package trace

import (
	"context"
)

const HeaderCorrelationID = "X-Correlation-ID"

type correlationIDKey struct{}

func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey{}, correlationID)
}

func CorrelationIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(correlationIDKey{}).(string); ok {
		return v
	}
	return ""
}
