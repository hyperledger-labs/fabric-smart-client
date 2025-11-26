/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type Ordering struct {
	network driver.FabricNetworkService
}

func (n *Ordering) Broadcast(context context.Context, blob interface{}) error {
	switch b := blob.(type) {
	case *Envelope:
		return n.network.OrderingService().Broadcast(context, b.Envelope)
	case *Transaction:
		return n.network.OrderingService().Broadcast(context, b.tx)
	default:
		return n.network.OrderingService().Broadcast(context, blob)
	}
}
