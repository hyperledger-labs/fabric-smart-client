/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

// Ordering models the ordering service
type Ordering interface {
	// Broadcast sends the passed blob to the ordering service to be ordered
	Broadcast(blob interface{}) error
}
