/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"reflect"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

// FabricNetworkService gives access to a Fabric network components
type FabricNetworkService interface {
	Config
	Ordering

	Name() string

	TransactionManager() TransactionManager

	ProcessorManager() ProcessorManager

	LocalMembership() LocalMembership

	IdentityProvider() IdentityProvider

	// Channel returns the channel whose name is the passed one.
	// If the empty string is passed, the default channel is returned, if defined.
	Channel(name string) (Channel, error)

	// Ledger returns the ledger for the channel whose name is the passed one.
	Ledger(name string) (Ledger, error)

	// Committer returns the committer for the channel whose name is the passed one.
	Committer(name string) (Committer, error)

	SignerService() SignerService

	ConfigService() ConfigService
}

type FabricNetworkServiceProvider interface {
	Names() []string
	DefaultName() string
	// FabricNetworkService returns a FabricNetworkService instance for the passed parameters
	FabricNetworkService(id string) (FabricNetworkService, error)
}

func GetFabricManagementService(ctx view2.ServiceProvider) FabricNetworkServiceProvider {
	s, err := ctx.GetService(reflect.TypeOf((*FabricNetworkServiceProvider)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(FabricNetworkServiceProvider)
}
