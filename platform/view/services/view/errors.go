/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

var (
	// ErrServiceNotFound occurs when a service is not found in the service provider.
	ErrServiceNotFound = errors.New("service not found")
	// ErrContextNotFound occurs when a view context is not found.
	ErrContextNotFound = errors.New("context not found")
	// ErrFactoryNotFound occurs when no factory is found for a given view ID.
	ErrFactoryNotFound = errors.New("no factory found for id")
	// ErrResponderNotFound occurs when no responder view is found for an initiator.
	ErrResponderNotFound = errors.New("responder not found")
	// ErrViewNotFound occurs when a view is not found.
	ErrViewNotFound = errors.New("view not found")
	// ErrSessionNotFound occurs when a communication session is not found.
	ErrSessionNotFound = errors.New("session not found")
	// ErrContextConversionFailed occurs when a context cannot be converted to another type.
	ErrContextConversionFailed = errors.New("context conversion failed")
	// ErrViewInstantiationFailed occurs when a view cannot be instantiated.
	ErrViewInstantiationFailed = errors.New("failed instantiating view")
	// ErrViewExecutionFailed occurs when a view execution fails.
	ErrViewExecutionFailed = errors.New("failed running view")
	// ErrInvalidView occurs when a view is invalid or nil.
	ErrInvalidView = errors.New("invalid view")
	// ErrInvalidIdentity occurs when an identity is invalid or nil.
	ErrInvalidIdentity = errors.New("invalid identity")
	// ErrCommandHeaderInvalid occurs when a command header is invalid or missing required fields.
	ErrCommandHeaderInvalid = errors.New("command header invalid")
	// ErrPanic occurs when a panic is recovered.
	ErrPanic = errors.New("panic recovered")
	// ErrCommandNotRecognized occurs when a command type is not recognized.
	ErrCommandNotRecognized = errors.New("command type not recognized")
	// ErrIdentityNotRecognized occurs when an identity is not recognized.
	ErrIdentityNotRecognized = errors.New("identity not recognized")
	// ErrInvalidConfig occurs when a configuration is invalid.
	ErrInvalidConfig = errors.New("invalid configuration")
)
