/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

type BuilderClient interface {
	Build(path string) string
}

type Builder struct {
	client BuilderClient
}

func (c *Builder) ConfigTxGen() string {
	return c.Build("github.com/hyperledger/fabric/cmd/configtxgen")
}

func (c *Builder) FSCCLI() string {
	return c.Build("github.com/hyperledger-labs/fabric-smart-client/cmd/fsccli")
}

func (c *Builder) Cryptogen() string {
	return c.Build("github.com/hyperledger-labs/fabric-smart-client/cmd/cryptogen")
}

func (c *Builder) Discover() string {
	return c.Build("github.com/hyperledger/fabric/cmd/discover")
}

func (c *Builder) Idemixgen() string {
	return c.Build("github.com/hyperledger/fabric/cmd/idemixgen")
}

func (c *Builder) Orderer() string {
	return c.Build("github.com/hyperledger/fabric/cmd/orderer")
}

func (c *Builder) Peer(peerPath string) string {
	if len(peerPath) != 0 {
		return c.Build(peerPath)
	}
	return c.Build("github.com/hyperledger/fabric/cmd/peer")
}

func (c *Builder) Build(path string) string {
	return c.client.Build(path)
}
