/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type TxID struct {
	Nonce   []byte
	Creator []byte
}

type ChaincodeInvocationType int

const (
	ChaincodeInvoke ChaincodeInvocationType = iota
	ChaincodeQuery
	ChaincodeEndorse
)

// ChaincodeInvocation models a client-side chaincode invocation
type ChaincodeInvocation interface {
	Call() (interface{}, error)

	WithTransientEntry(k string, v interface{}) ChaincodeInvocation

	WithEndorsers(ids ...view.Identity) ChaincodeInvocation

	WithEndorsersByMSPIDs(mspIDs ...string) ChaincodeInvocation

	WithEndorsersFromMyOrg() ChaincodeInvocation

	WithSignerIdentity(id view.Identity) ChaincodeInvocation

	WithTxID(id TxID) ChaincodeInvocation

	WithEndorsersByConnConfig(ccs ...*grpc.ConnectionConfig) ChaincodeInvocation
}

// ChaincodeDiscover models a client-side chaincode's endorsers discovery operation
type ChaincodeDiscover interface {
	Call() ([]view.Identity, error)
	WithFilterByMSPIDs(mspIDs ...string) ChaincodeDiscover
}

// Chaincode exposes chaincode-related functions
type Chaincode interface {
	NewInvocation(typ ChaincodeInvocationType, function string, args ...interface{}) ChaincodeInvocation
	NewDiscover() ChaincodeDiscover
}

// ChaincodeManager manages chaincodes
type ChaincodeManager interface {
	// Chaincode returns a chaincode handler for the passed chaincode name
	Chaincode(name string) Chaincode
}
