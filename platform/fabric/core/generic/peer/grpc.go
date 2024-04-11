/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type GRPCClient struct {
	*grpc.Client
	Address string
	Sn      string
}
