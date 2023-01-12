/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type TxID struct {
	Nonce   []byte
	Creator []byte
}

// ChaincodeInvocation models a client-side chaincode invocation
type ChaincodeInvocation interface {
	Endorse() (Envelope, error)

	Query() ([]byte, error)

	Submit() (string, []byte, error)

	WithTransientEntry(k string, v interface{}) ChaincodeInvocation

	WithEndorsersByMSPIDs(mspIDs ...string) ChaincodeInvocation

	WithEndorsersFromMyOrg() ChaincodeInvocation

	WithSignerIdentity(id view.Identity) ChaincodeInvocation

	WithTxID(id TxID) ChaincodeInvocation

	WithEndorsersByConnConfig(ccs ...*grpc.ConnectionConfig) ChaincodeInvocation

	WithImplicitCollections(mspIDs ...string) ChaincodeInvocation

	// WithDiscoveredEndorsersByEndpoints sets the endpoints to be used to filter the result of
	// discovery. Discovery is used to identify the chaincode's endorsers, if not set otherwise.
	WithDiscoveredEndorsersByEndpoints(endpoints ...string) ChaincodeInvocation

	// WithMatchEndorsementPolicy enforces that the query is perfomed against a set of peers that satisfy the
	// endorsement policy of the chaincode
	WithMatchEndorsementPolicy() ChaincodeInvocation

	// WithNumRetries sets the number of times the chaincode operation should be retried before returning a failure
	WithNumRetries(numRetries uint) ChaincodeInvocation

	// WithRetrySleep sets the time interval between each retry
	WithRetrySleep(duration time.Duration) ChaincodeInvocation

	WithContext(context context.Context) ChaincodeInvocation
}

// DiscoveredPeer contains the information of a discovered peer
type DiscoveredPeer struct {
	// Identity is the identity of the peer (MSP Identity)
	Identity view.Identity
	// MSPID is the MSP ID of the peer
	MSPID string
	// Endpoint is the endpoint of the peer
	Endpoint string
	// TLSRootCerts is the TLS root certs of the peer
	TLSRootCerts [][]byte
}

// ChaincodeDiscover models a client-side chaincode's endorsers discovery operation
type ChaincodeDiscover interface {
	// Call invokes discovery service and returns the discovered peers
	Call() ([]DiscoveredPeer, error)
	WithFilterByMSPIDs(mspIDs ...string) ChaincodeDiscover
	WithImplicitCollections(mspIDs ...string) ChaincodeDiscover
}

// Chaincode exposes chaincode-related functions
type Chaincode interface {
	NewInvocation(function string, args ...interface{}) ChaincodeInvocation
	NewDiscover() ChaincodeDiscover
	IsAvailable() (bool, error)
	IsPrivate() bool
	// Version returns the version of this chaincode.
	// It returns an error if a failure happens during the computation.
	Version() (string, error)
}

// ChaincodeManager manages chaincodes
type ChaincodeManager interface {
	// Chaincode returns a chaincode handler for the passed chaincode name
	Chaincode(name string) Chaincode
}
