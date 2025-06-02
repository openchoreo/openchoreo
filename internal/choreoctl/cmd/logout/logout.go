// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logout

import (
	"fmt"
)

type LogoutImpl struct{}

func NewLogoutImpl() *LogoutImpl {
	return &LogoutImpl{}
}

func (i *LogoutImpl) Logout() error {
	return fmt.Errorf("logout functionality is not supported")
}
