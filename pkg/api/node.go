/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package api

type FabricSmartClientNode interface {
	ServiceProvider
	ViewRegistry
	ContextProvider
	ViewClient

	Start() error

	Stop()
}
