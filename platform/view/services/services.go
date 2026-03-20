/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package services

//go:generate counterfeiter -o mock/provider.go -fake-name ServiceProvider . Provider

// Provider is used to return instances of a given type
type Provider interface {
	// GetService returns an instance of the given type
	GetService(v any) (any, error)
}

//go:generate counterfeiter -o mock/registry.go -fake-name ServiceRegistry . Registry

// Registry is a Provider that allows the developer to register services as well.
type Registry interface {
	Provider
	RegisterService(service any) error
}
