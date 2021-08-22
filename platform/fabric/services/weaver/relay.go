/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	fabric2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver/relay/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("weaver.relay")

// Relay gives access to the services offered by the Relay server
type Relay struct {
	fns *fabric2.NetworkService
}

// ToFabric gives access to the Relay services towards a Fabric network
func (r *Relay) ToFabric() *fabric.Fabric {
	ch, err := r.fns.Channel(r.fns.DefaultChannel())
	if err != nil {
		logger.Errorf("cannot get channel [%s:%s]: [%s]", r.fns.Name(), r.fns.DefaultChannel())
		return nil
	}

	return fabric.New(r.fns, ch)
}
