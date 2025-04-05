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

package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/version"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
)

// NewVersionCmd creates the login command.
func NewVersionCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.Version,
		RunE: func(fg *builder.FlagGetter) error {
			v := version.Get()
			fmt.Printf("%s %s\n", v.Name, v.Version)
			fmt.Printf("Git revision: %s\n", v.GitRevision)
			fmt.Printf("Build time:   %s\n", v.BuildTime)
			fmt.Printf("Go version:   %s %s/%s\n",
				v.GoVersion, v.GoOS, v.GoArch)
			return nil
		},
	}).Build()
}
