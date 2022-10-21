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

func (c *Builder) FSCCLI() string {
	return c.Build("github.com/hyperledger-labs/fabric-smart-client/cmd/fsccli")
}

func (c *Builder) Idemixgen() string {
	return c.Build("github.com/IBM/idemix/tools/idemixgen")
}

func (c *Builder) Build(path string) string {
	return c.client.Build(path)
}
