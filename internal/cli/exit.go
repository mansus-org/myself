package cli

import (
	"errors"
	"fmt"
)

// Exit codes, SPEC section 6.
const (
	ExitOK       = 0
	ExitUsage    = 1
	ExitNotFound = 2
	ExitStore    = 3
)

// ExitError is an error that carries the process exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// Usagef returns an error that exits with ExitUsage.
func Usagef(format string, a ...any) error {
	return &ExitError{ExitUsage, fmt.Errorf(format, a...)}
}

// NotFoundf returns an error that exits with ExitNotFound.
func NotFoundf(format string, a ...any) error {
	return &ExitError{ExitNotFound, fmt.Errorf(format, a...)}
}

// StoreErr wraps err so it exits with ExitStore.
func StoreErr(err error) error {
	return &ExitError{ExitStore, err}
}

// exitCode maps an error to an exit code. Errors without a code come from
// cobra (unknown command, bad flag, bad args), which are all usage errors.
func exitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return ExitUsage
}
