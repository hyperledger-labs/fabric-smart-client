/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"

// Templates can be used to provide custom templates to GenerateConfigTree.
type Templates struct {
	Node   string `yaml:"node,omitempty"`
	Core   string `yaml:"core,omitempty"`
	Crypto string `yaml:"crypto,omitempty"`
}

func (t *Templates) NodeTemplate() string {
	if t.Node != "" {
		return t.Node
	}
	return node.DefaultTemplate
}

func (t *Templates) CoreTemplate() string {
	if t.Core != "" {
		return t.Core
	}
	return node.CoreTemplate
}

func (t *Templates) CryptoTemplate() string {
	if t.Crypto != "" {
		return t.Crypto
	}
	return node.DefaultCryptoTemplate
}
