package build

import "context"

// ResourceHandler is an interface that defines the operations that can be performed on the external (Dataplane) or
// internal (Control Plane) resources that are managed by the build controller during the reconciliation process.
type ResourceHandler[T any] interface {
	// KindName returns the kind name.
	KindName() string

	// Name returns the name of the external resource.
	Name(ctx context.Context, resourceCtx *T) string

	// Get fetches the resource’s current state.
	Get(ctx context.Context, resourceCtx *T) (interface{}, error)

	// Create initializes the resource if needed.
	Create(ctx context.Context, resourceCtx *T) error

	// Update updates the resource if neeeded.
	Update(ctx context.Context, resourceCtx *T, currentState interface{}) error
}
