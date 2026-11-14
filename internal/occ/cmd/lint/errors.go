// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lint

import (
	"errors"
	"fmt"
)

// ExitError distinguishes three process outcomes, and CI gates are built on
// them: 0 for success, 1 when validation found problems, and 2 for a usage
// mistake such as an unknown flag or a missing path.
//
// Cobra reports failures by returning an error rather than by choosing an exit
// code, so a command that needs a specific status returns one of these instead.
// cmd/occ/main.go then recovers the code with errors.As; without that, every
// failure would collapse to occ's blanket os.Exit(1) and a "bad flags" run
// would be indistinguishable from "your resources are invalid".
type ExitError struct {
	Code int
}

// Error's message is never printed: the command has already written whatever
// diagnostic the user should see. It exists so the type satisfies error.
func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// ErrFindings reports that validation completed and left diagnostics behind.
// The diagnostics have already been written to stdout by the time this is
// returned, so the error itself carries no message.
var ErrFindings = &ExitError{Code: 1}

// ErrUsage reports a usage mistake: an unknown flag, a missing path, or a
// command used in a way its arguments do not allow. The diagnostic and the
// usage block are printed before this is returned.
var ErrUsage = &ExitError{Code: 2}

// ExitStatus reports the process exit code for err, and whether err is one of
// the linter's deliberate statuses.
//
// The bool matters as much as the code: a deliberate status has already been
// reported to the user by the command that produced it - the diagnostics for a
// failed validation, or the message and usage block for a usage error - so the
// caller must exit with the code without printing anything more. Anything else
// is an unexpected failure, which the caller reports itself.
func ExitStatus(err error) (int, bool) {
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code, true
	}
	return 1, false
}
