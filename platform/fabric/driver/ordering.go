/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "context"

// Ordering models the ordering service
type Ordering interface {
	// Broadcast sends the passed blob to the ordering service to be ordered
	Broadcast(context context.Context, blob interface{}) error
}
