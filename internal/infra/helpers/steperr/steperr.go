// Package steperr marks which stage of a multi-step flow failed.
//
// Its own package because both site packages raise these and the command layer
// reads them, and because the name is the point: steperr.Error read as
// though a browser had a notion of steps, when what has steps is the flow being
// driven through it.
package steperr

import "errors"

// Error identifies which stage of a multi-step flow failed.
//
// Both site flows have the same shape — navigate, fill, submit, answer a
// challenge, land — and callers need to react to *where* one broke rather than
// to the message text. The debug commands use Step to label a page dump; sync
// uses it to say which stage to go look at.
type Error struct {
	Step string
	Err  error
}

// Error implements the error interface.
func (e *Error) Error() string { return e.Step + ": " + e.Err.Error() }

// Unwrap exposes the underlying cause to [errors.Is] and [errors.AsType].
func (e *Error) Unwrap() error { return e.Err }

// Wrap marks err as having failed at step. It returns nil when err is nil,
// so it can wrap a call's result directly.
func Wrap(step string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Step: step, Err: err}
}

// Of returns the failing step name, or "" if err carries no [Error].
func Of(err error) string {
	if se, ok := errors.AsType[*Error](err); ok {
		return se.Step
	}
	return ""
}
