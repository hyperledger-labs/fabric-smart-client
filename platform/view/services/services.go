/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package services

// Provider is used to return instances of a given type
type Provider interface {
	// GetService returns an instance of the given type
	GetService(v any) (any, error)
}

// Registry is a Provider that allows the developer to register services as well.
type Registry interface {
	Provider
	RegisterService(service any) error
}
