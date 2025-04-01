/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

type FabricSmartClientNode interface {
	ViewRegistry
	ContextProvider
	ViewClient

	Start() error

	Stop()
}
