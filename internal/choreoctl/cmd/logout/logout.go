/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

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
