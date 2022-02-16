/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

type Orderer struct {
	Name             string
	FullName         string
	ListeningAddress string
	TLSCACerts       []string
	OperationAddress string
}

type Peer struct {
	Name             string
	FullName         string
	ListeningAddress string
	TLSCACerts       []string
	Cert             string
	OperationAddress string
}

type Org struct {
	Name                  string
	MSPID                 string
	CACertsBundlePath     string
	PeerCACertificatePath string
}

type User struct {
	Name string
	Cert string
	Key  string
}

type Chaincode struct {
	Name      string
	OrgMSPIDs []string
}

type Channel struct {
	Name       string
	Chaincodes []*Chaincode
}
