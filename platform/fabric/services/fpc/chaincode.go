/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fpc

import (
	"strconv"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type contractInvoker interface {
	Name() string
	EvaluateTransaction(name string, args ...string) ([]byte, error)
	SubmitTransaction(name string, args ...string) ([]byte, error)
}

type identityProvider interface {
	Identity(label string) view.Identity
}

// ChaincodeInvocation models the invocation of an FPC
type ChaincodeInvocation struct {
	*Chaincode

	function string
	args     []interface{}
}

// Call invokes the chaincode and returns the result
func (i *ChaincodeInvocation) Call() ([]byte, error) {
	args, err := i.prepareArgs()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed preparing arguments")
	}
	return i.Contract.SubmitTransaction(i.function, args...)
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

// Chaincode models a Fabric Private Chaincode
type Chaincode struct {
	Channel          *fabric.Channel
	EnclaveRegistry  *EnclaveRegistry
	Contract         contractInvoker
	Signer           view.Identity
	IdentityProvider identityProvider

	ID string
}

// NewChaincode returns a new chaincode instance
func NewChaincode(ch *fabric.Channel, er *EnclaveRegistry, contract contractInvoker, id view.Identity, ip identityProvider, cid string) *Chaincode {
	return &Chaincode{Channel: ch, EnclaveRegistry: er, Contract: contract, Signer: id, IdentityProvider: ip, ID: cid}
}

// Invoke returns an object that models an FPC invocation for the passed function and arguments
func (c *Chaincode) Invoke(function string, args ...interface{}) *ChaincodeInvocation {
	return &ChaincodeInvocation{
		Chaincode: c,
		function:  function,
		args:      args,
	}
}

// StringsToArgs converts a slice of strings into a slace of interface{}
func StringsToArgs(args []string) []interface{} {
	var res []interface{}
	for _, arg := range args {
		res = append(res, arg)
	}
	return res
}
