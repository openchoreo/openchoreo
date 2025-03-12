/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package source

import (
	"context"
)

// SourceHandler is an interface that defines the operations that can be performed on the source provider
// (GitHub/BitBucket/GitLab/etc.) by the build controller during the reconciliation process.
type SourceHandler[T any] interface {
	// Name returns the name of the source provider.
	Name(ctx context.Context, resourceCtx *T) string

	// FetchComponentDescriptor fetches the component yaml from the source repository.
	FetchComponentDescriptor(ctx context.Context, resourceCtx *T) (*Config, error)
}
