/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package errors

import "github.com/pkg/errors"

// HasCause recursively checks errors wrapped using Wrapf until it detects the target error
func HasCause(source, target error) bool {
	if source == nil || target == nil {
		return false
	}
	if source == target {
		return true
	}
	cause := errors.Cause(source)
	if cause == source {
		return false
	}
	return HasCause(cause, target)
}

// Wrapf wraps an error in a way compatible with HasCause
func Wrapf(err error, format string, args ...any) error {
	return errors.Wrapf(err, format, args...)
}

func Errorf(format string, args ...any) error {
	return errors.Errorf(format, args...)
}
