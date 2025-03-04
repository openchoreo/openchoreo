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

package auth

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

// CheckLoginStatus ensures the user is logged in before executing any command.
func CheckLoginStatus(impl api.CommandImplementationInterface) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if cmd.Name() != "login" && cmd.Name() != "logout" && !impl.IsLoggedIn() {
			fmt.Println(impl.GetLoginPrompt())
			os.Exit(1)
		}
		return nil
	}
}
