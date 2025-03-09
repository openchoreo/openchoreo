package source

import "context"

// SourceHandler is an interface that defines the operations that can be performed on the source provider
// (GitHub/BitBucket/GitLab/etc.) by the build controller during the reconciliation process.
type SourceHandler[T any] interface {
	// Name returns the name of the source provider.
	Name(ctx context.Context, resourceCtx *T) string

	// FetchComponentDescriptor fetches the component yaml from the source repository.
	FetchComponentDescriptor(ctx context.Context, resourceCtx *T) (interface{}, error)
}
