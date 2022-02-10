/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"crypto/tls"
	"github.com/hyperledger/fabric-protos-go/discovery"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	discovery2 "github.com/hyperledger/fabric/discovery/client"
	"golang.org/x/net/context"
	grpc2 "google.golang.org/grpc"
)

type DiscoveryClient interface {
	Send(ctx context.Context, req *discovery2.Request, auth *discovery.AuthInfo) (discovery2.Response, error)
}

type Client interface {
	Certificate() tls.Certificate

	Connection() (*grpc2.ClientConn, error)

	Endorser() (pb.EndorserClient, error)

	Discovery() (discovery.DiscoveryClient, error)

	DiscoveryClient() (DiscoveryClient, error)

	Close()
}
