/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

type ServiceLocator interface {
	GetService(v interface{}) (interface{}, error)
}

type Registry interface {
	ServiceLocator

	RegisterService(service interface{}) error
}
