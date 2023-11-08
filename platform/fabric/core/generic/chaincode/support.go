/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"

	pb "github.com/hyperledger/fabric-protos-go/peer"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("fabric-sdk.chaincode")

//go:generate counterfeiter -o mocks/sp.go -fake-name SignerProvider . SignerProvider
//go:generate counterfeiter -o mocks/sersig.go -fake-name SerializableSigner . SerializableSigner
//go:generate counterfeiter -o mocks/network.go -fake-name Network . Network
//go:generate counterfeiter -o mocks/channel.go -fake-name Channel . Channel
//go:generate counterfeiter -o mocks/cd.go -fake-name Discover . Discover
//go:generate counterfeiter -o mocks/mspm.go -fake-name MSPManager . MSPManager
//go:generate counterfeiter -o mocks/mspid.go -fake-name MSPIdentity . MSPIdentity
//go:generate counterfeiter -o mocks/pc.go -fake-name PeerClient . PeerClient
//go:generate counterfeiter -o mocks/ec.go -fake-name EndorserClient . EndorserClient
//go:generate counterfeiter -o mocks/sc.go -fake-name SignerService . SignerService
//go:generate counterfeiter -o mocks/si.go -fake-name SigningIdentity . SigningIdentity

type SigningIdentity = driver.SigningIdentity
type SignerService = driver.SignerService
type EndorserClient = pb.EndorserClient
type PeerClient = peer.Client
type MSPIdentity = driver.MSPIdentity
type MSPManager = driver.MSPManager

type Discover = driver.ChaincodeDiscover

type SignerProvider interface {
	GetSigningIdentity(identity view.Identity) (*view2.SigningIdentity, error)
}

type SerializableSigner interface {
	Sign(message []byte) ([]byte, error)

	Serialize() ([]byte, error)
}

type Network interface {
	Name() string
	PickPeer(funcType driver.PeerFunctionType) *grpc.ConnectionConfig
	LocalMembership() driver.LocalMembership
	// Broadcast sends the passed blob to the ordering service to be ordered
	Broadcast(context context.Context, blob interface{}) error
	SignerService() driver.SignerService
	Config() *config.Config
}

type Channel interface {
	// Name returns the name of the channel
	Name() string

	// Config returns the channel configuration
	Config() *config.Channel

	// NewPeerClientForAddress creates an instance of a Client using the
	// provided peer connection config
	NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer.Client, error)

	// IsFinal takes in input a transaction id and waits for its confirmation
	// with the respect to the passed context that can be used to set a deadline
	// for the waiting time.
	IsFinal(ctx context.Context, txID string) error

	MSPManager() driver.MSPManager

	Chaincode(name string) driver.Chaincode
}
