/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	fabric2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver/relay/fabric"
)

type Relay struct {
	fns *fabric2.NetworkService
}

func (r *Relay) Fabric() *fabric.Fabric {
	return fabric.New(r.fns)
}
