/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "context"

// Ordering models the ordering service
type Ordering interface {
	// Broadcast sends the passed blob to the ordering service to be ordered. It retries a
	// failed attempt after the configured retry interval; when ctx ends during that wait,
	// Broadcast stops and returns an error that wraps ctx.Err().
	Broadcast(ctx context.Context, blob any) error

	// SetConsensusType sets the consensus type the ordering service should use
	SetConsensusType(consensusType string) error
}
