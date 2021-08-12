/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fpc

import (
	"strconv"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/fpc/core/generic/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type EncryptionProvider interface {
	NewEncryptionContext() (crypto.EncryptionContext, error)
}

type IdentityProvider interface {
	Identity(label string) view.Identity
}

type ChaincodeInvocation struct {
	*Chaincode

	function string
	args     []interface{}
}

func (i *ChaincodeInvocation) Call() ([]byte, error) {
	ctx, err := i.ep.NewEncryptionContext()
	if err != nil {
		return nil, err
	}

	args, err := i.prepareArgs()
	if err != nil {
		return nil, err
	}

	encryptedRequest, err := ctx.Conceal(i.function, args)
	if err != nil {
		return nil, err
	}

	// call __invoke
	encryptedResponse, err := i.evaluateTransaction(encryptedRequest)
	if err != nil {
		return nil, err
	}

	logger.Debugf("calling __endorse!")
	_, _, err = i.ch.Chaincode(i.cid).Invoke(
		"__endorse", string(encryptedResponse),
	).WithInvokerIdentity(
		i.id,
	).Call()
	if err != nil {
		return nil, err
	}

	res, err := ctx.Reveal(encryptedResponse)
	if err != nil {
		return nil, errors.Wrapf(err, "failed revealing result [%s]", string(encryptedResponse))
	}
	return res, nil
}

func (i *ChaincodeInvocation) evaluateTransaction(args ...string) ([]byte, error) {
	peers, err := i.er.PeerEndpoints(i.cid)
	if err != nil {
		return nil, err
	}

	var endorsers []view.Identity
	for _, peer := range peers {
		endorsers = append(endorsers, i.ip.Identity(peer))
	}

	// gateway.WithEndorsingPeers(peers...),
	var passedArgs []interface{}
	for _, arg := range args {
		passedArgs = append(passedArgs, arg)
	}
	raw, err := i.ch.Chaincode(i.cid).Query(
		"__invoke", passedArgs...,
	).WithInvokerIdentity(
		i.id,
	).WithEndorsers(
		endorsers...,
	).Call()
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (i *ChaincodeInvocation) prepareArgs() ([]string, error) {
	var args []string
	for _, arg := range i.args {
		b, err := i.toString(arg)
		if err != nil {
			return nil, err
		}
		args = append(args, b)
	}
	return args, nil
}

func (i *ChaincodeInvocation) toString(arg interface{}) (string, error) {
	switch v := arg.(type) {
	case []byte:
		return string(v), nil
	case string:
		return v, nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	default:
		return "", errors.Errorf("arg type [%T] not recognized.", v)
	}
}

type Chaincode struct {
	ch *fabric.Channel
	er *EnclaveRegistry
	id view.Identity
	ep EncryptionProvider
	ip IdentityProvider

	cid string
}

func (c *Chaincode) Invoke(function string, args ...interface{}) (*ChaincodeInvocation, error) {
	return &ChaincodeInvocation{
		Chaincode: c,
		function:  function,
		args:      args,
	}, nil
}
