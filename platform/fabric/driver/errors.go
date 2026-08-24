/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// ErrNotImplemented signals that a function is not implemented
var ErrNotImplemented = errors.New("not implemented")

// ErrNotInitialized signals that a service cannot answer yet because the
// configuration it depends on has not been loaded.
//
// This is a transient startup condition, not a permanent failure: channel
// configuration is loaded asynchronously after the owning service is
// constructed, so a caller racing that load observes it legitimately. Callers
// that can tolerate the race should test for it with errors.Is and retry.
var ErrNotInitialized = errors.New("not initialized")
