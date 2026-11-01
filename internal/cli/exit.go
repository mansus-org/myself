package cli

import "github.com/mansus-org/myself/internal/brain"

// Exit codes, SPEC section 6.
const (
	ExitOK       = brain.ExitOK
	ExitUsage    = brain.ExitUsage
	ExitNotFound = brain.ExitNotFound
	ExitStore    = brain.ExitStore
)

// ExitError is an error that carries the process exit code.
type ExitError = brain.ExitError

// Usagef returns an error that exits with ExitUsage.
func Usagef(format string, a ...any) error { return brain.Usagef(format, a...) }

// NotFoundf returns an error that exits with ExitNotFound.
func NotFoundf(format string, a ...any) error { return brain.NotFoundf(format, a...) }

// StoreErr wraps err so it exits with ExitStore.
func StoreErr(err error) error { return brain.StoreErr(err) }

// exitCode maps an error to an exit code. Errors without a code come from
// cobra (unknown command, bad flag, bad args), which are all usage errors.
func exitCode(err error) int { return brain.Code(err) }
