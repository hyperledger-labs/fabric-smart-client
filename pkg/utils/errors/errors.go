/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package errors

import "github.com/cockroachdb/errors"

// Join returns an error that wraps the given errors.
func Join(errs ...error) error {
	return errors.Join(errs...)
}

// HasType recursively checks errors wrapped using Wrapf until it detects the target error type
func HasType(source, target error) bool {
	return source != nil && target != nil && errors.As(source, &target)
}

// HasCause recursively checks errors wrapped using Wrapf until it detects the target error
func HasCause(source, target error) bool {
	return source != nil && target != nil && errors.Is(source, target)
}

// Cause returns the underlying cause of the error, if possible.
func Cause(err error) error {
	return errors.Cause(err)
}

// Is recursively checks errors wrapped using Wrapf until it detects the target error
func Is(source, target error) bool {
	return source != nil && target != nil && errors.Is(source, target)
}

// Wrapf wraps an error in a way compatible with HasCause
func Wrapf(err error, format string, args ...any) error {
	return errors.Wrapf(err, format, args...)
}

// WithMessagef annotates err with the format specifier.
func WithMessagef(err error, format string, args ...any) error {
	return errors.WithMessagef(err, format, args...)
}

// WithMessage annotates err with the format specifier.
func WithMessage(err error, format string) error {
	return errors.WithMessage(err, format)
}

func WithStack(err error) error {
	return errors.WithStack(err)
}

// Wrap wraps an error in a way compatible with HasCause
func Wrap(err error, message string) error {
	return errors.Wrap(err, message)
}

// Errorf formats according to a format specifier and returns the string
// as a value that satisfies error.
// Errorf also records the stack trace at the point it was called.
func Errorf(format string, args ...any) error {
	return errors.Errorf(format, args...)
}

// New returns an error with the supplied message.
// New also records the stack trace at the point it was called.
func New(msg string) error {
	return errors.New(msg)
}
