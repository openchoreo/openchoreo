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

package login

import (
	"fmt"

	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

type AuthImpl struct{}

var _ api.LoginAPI = &AuthImpl{}

func NewAuthImpl() *AuthImpl {
	return &AuthImpl{}
}

func (i *AuthImpl) Login(params api.LoginParams) error {
	return fmt.Errorf("login functionality is not supported")
}

func (i *AuthImpl) IsLoggedIn() bool {
	return false
}

func (i *AuthImpl) GetLoginPrompt() string {
	return "login functionality is not supported"
}
