/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package state

// Ownable defines the methods that an ownable state should expose
type Ownable interface {
	// Owners returns the identities of the owners of this state
	Owners() Identities
}
