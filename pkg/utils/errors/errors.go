/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package errors

import "github.com/pkg/errors"

// HasType recursively checks errors wrapped using Wrapf until it detects the target error type
func HasType(source, target error) bool {
	return source != nil && target != nil && errors.As(source, &target)
}

// HasCause recursively checks errors wrapped using Wrapf until it detects the target error
func HasCause(source, target error) bool {
	return source != nil && target != nil && errors.Is(source, target)
}

// Is recursively checks errors wrapped using Wrapf until it detects the target error
func Is(source, target error) bool {
	return source != nil && target != nil && errors.Is(source, target)
}

// Wrapf wraps an error in a way compatible with HasCause
func Wrapf(err error, format string, args ...any) error {
	return errors.Wrapf(err, format, args...)
}

// Wrap wraps an error in a way compatible with HasCause
func Wrap(err error, message string) error {
	return errors.Wrap(err, message)
}

func Errorf(format string, args ...any) error {
	return errors.Errorf(format, args...)
}

func New(msg string) error {
	return errors.New(msg)
}
