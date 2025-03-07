package kubernetes

import "context"

type ResourceHandler[T any] interface {
	// KindName returns the kind name.
	KindName() string

	// Name returns the name of the external resource.
	Name(ctx context.Context, resourceCtx *T) string

	// Get fetches the resource’s current state.
	Get(ctx context.Context, resourceCtx *T) (interface{}, error)

	// Create initializes the resource if needed.
	Create(ctx context.Context, resourceCtx *T) error
}
