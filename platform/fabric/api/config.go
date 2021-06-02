/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package api

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"

type Config interface {
	DefaultChannel() string

	Channels() []string

	Orderers() []*grpc.ConnectionConfig

	Peers() []*grpc.ConnectionConfig
}
