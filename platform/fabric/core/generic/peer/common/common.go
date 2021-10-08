/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type CommonClient struct {
	*grpc.Client
	Address string
	Sn      string
}
