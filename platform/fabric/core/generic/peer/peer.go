/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"crypto/tls"

	"github.com/hyperledger/fabric-protos-go/discovery"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

type Client interface {
	Certificate() tls.Certificate

	Endorser() (pb.EndorserClient, error)

	Discovery() (discovery.DiscoveryClient, error)

	// TODO: improve by providing grpc connection pool
	Close()
}
