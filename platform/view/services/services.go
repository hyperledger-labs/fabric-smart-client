/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package services

// Provider is used to return instances of a given type
type Provider interface {
	// GetService returns an instance of the given type
	GetService(v interface{}) (interface{}, error)
}

type Registry interface {
	Provider
	RegisterService(service interface{}) error
}
