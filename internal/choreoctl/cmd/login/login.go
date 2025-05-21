/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package login

import (
	"fmt"

	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
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
