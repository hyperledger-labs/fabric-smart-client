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
//
// A service that was offered a configuration and refused it reports
// ErrConfigRejected instead, so that a caller acting on this one is never told
// to retry something that retrying cannot fix.
var ErrNotInitialized = errors.New("not initialized")

// ErrConfigRejected signals that a service cannot answer because the only
// configuration it has been offered was refused: a configuration block that
// would not parse or failed validation, or one naming something the service
// cannot serve, such as an unsupported consensus type.
//
// Unlike ErrNotInitialized this is not a startup race, and retrying the call
// will not clear it. It is not permanent either: the service recovers as soon as
// a later configuration update is accepted, so a caller should surface it rather
// than either retrying in a loop or treating the node as dead.
//
// The error returned by the update that was refused is wrapped, so the reason
// travels with the sentinel.
var ErrConfigRejected = errors.New("configuration rejected")
