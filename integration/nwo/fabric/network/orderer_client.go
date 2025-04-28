/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"context"
	"os"
	"path"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/pkg/errors"
)

// Broadcast sends given env to Broadcast API of specified orderer.
func Broadcast(n *Network, o *topology.Orderer, env *common.Envelope) (*orderer.BroadcastResponse, error) {
	gRPCclient, err := createOrdererGRPCClient(n, o)
	if err != nil {
		return nil, err
	}

	addr := n.OrdererAddress(o, ListenPort)
	conn, err := gRPCclient.NewConnection(addr)
	if err != nil {
		return nil, err
	}
	defer utils.IgnoreErrorFunc(conn.Close)

	broadcaster, err := orderer.NewAtomicBroadcastClient(conn).Broadcast(context.Background())
	if err != nil {
		return nil, err
	}

	err = broadcaster.Send(env)
	if err != nil {
		return nil, err
	}

	resp, err := broadcaster.Recv()
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// Deliver sends given env to Deliver API of specified orderer.
func Deliver(n *Network, o *topology.Orderer, env *common.Envelope) (*common.Block, error) {
	gRPCclient, err := createOrdererGRPCClient(n, o)
	if err != nil {
		return nil, err
	}

	addr := n.OrdererAddress(o, ListenPort)
	conn, err := gRPCclient.NewConnection(addr)
	if err != nil {
		return nil, err
	}
	defer utils.IgnoreErrorFunc(conn.Close)

	deliverer, err := orderer.NewAtomicBroadcastClient(conn).Deliver(context.Background())
	if err != nil {
		return nil, err
	}

	err = deliverer.Send(env)
	if err != nil {
		return nil, err
	}

	resp, err := deliverer.Recv()
	if err != nil {
		return nil, err
	}

	blk := resp.GetBlock()
	if blk == nil {
		return nil, errors.Errorf("block not found")
	}

	return blk, nil
}

func createOrdererGRPCClient(n *Network, o *topology.Orderer) (*grpc.Client, error) {
	config := grpc.ClientConfig{}
	config.Timeout = 5 * time.Second

	secOpts := grpc.SecureOptions{
		UseTLS:            true,
		RequireClientCert: false,
	}

	caPEM, err := os.ReadFile(path.Join(n.OrdererLocalTLSDir(o), "ca.crt"))
	if err != nil {
		return nil, err
	}

	secOpts.ServerRootCAs = [][]byte{caPEM}
	config.SecOpts = secOpts

	grpcClient, err := grpc.NewGRPCClient(config)
	if err != nil {
		return nil, err
	}

	return grpcClient, nil
}
