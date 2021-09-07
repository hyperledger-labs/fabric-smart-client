/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fpc

import "C"
import (
	fpc "github.com/hyperledger/fabric-private-chaincode/client_sdk/go/pkg/core/contract"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type transaction struct {
	fns      *fabric.NetworkService
	ch       *fabric.Channel
	id       string
	function string
	peers    []string
}

func (t *transaction) Evaluate(args ...string) ([]byte, error) {
	var endorsers []view.Identity
	ip := t.fns.IdentityProvider()
	for _, peer := range t.peers {
		endorsers = append(endorsers, ip.Identity(peer))
	}

	// gateway.WithEndorsingPeers(peers...),
	var passedArgs []interface{}
	for _, arg := range args {
		passedArgs = append(passedArgs, arg)
	}
	raw, err := t.ch.Chaincode(t.id).Query(
		t.function, passedArgs...,
	).WithInvokerIdentity(
		t.fns.IdentityProvider().DefaultIdentity(),
	).WithEndorsers(
		endorsers...,
	).Call()
	if err != nil {
		return nil, err
	}
	return raw, nil
}

type contract struct {
	fns *fabric.NetworkService
	ch  *fabric.Channel
	id  string
}

func (c *contract) Name() string {
	return c.id
}

func (c *contract) EvaluateTransaction(name string, args ...string) ([]byte, error) {
	var passedArgs []interface{}
	for _, arg := range args {
		passedArgs = append(passedArgs, arg)
	}
	raw, err := c.ch.Chaincode(c.id).Query(
		name, passedArgs...,
	).WithInvokerIdentity(
		c.fns.IdentityProvider().DefaultIdentity(),
	).Call()
	if err != nil {
		return nil, err
	}
	return raw, err
}

func (c *contract) SubmitTransaction(name string, args ...string) ([]byte, error) {
	var passedArgs []interface{}
	for _, arg := range args {
		passedArgs = append(passedArgs, arg)
	}
	_, raw, err := c.ch.Chaincode(c.id).Invoke(
		name, passedArgs...,
	).WithInvokerIdentity(
		c.fns.IdentityProvider().DefaultIdentity(),
	).Call()
	if err != nil {
		return nil, err
	}
	return raw, err
}

func (c *contract) CreateTransaction(name string, peerEndpoints ...string) (fpc.Transaction, error) {
	return &transaction{
		fns:      c.fns,
		ch:       c.ch,
		id:       c.id,
		function: name,
		peers:    peerEndpoints,
	}, nil
}

type contractProvider struct {
	fns *fabric.NetworkService
	ch  *fabric.Channel
}

func (c *contractProvider) GetContract(id string) fpc.Contract {
	return &contract{
		fns: c.fns,
		ch:  c.ch,
		id:  id,
	}
}
