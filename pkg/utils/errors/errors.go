/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package errors

import "github.com/pkg/errors"

func HasCause(source, target error) bool {
	if source == nil || target == nil {
		return false
	}
	if errors.Is(source, target) {
		return true
	}
	cause := errors.Cause(source)
	if cause == source {
		return false
	}
	return HasCause(cause, target)
}
