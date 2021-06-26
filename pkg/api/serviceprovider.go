/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

type ServiceProvider interface {
	GetService(v interface{}) (interface{}, error)

	RegisterService(service interface{}) error
}
