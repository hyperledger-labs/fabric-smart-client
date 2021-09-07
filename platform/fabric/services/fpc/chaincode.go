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

type Contract interface {
	Name() string
	EvaluateTransaction(name string, args ...string) ([]byte, error)
	SubmitTransaction(name string, args ...string) ([]byte, error)
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
	args, err := i.prepareArgs()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed preparing arguments")
	}
	return i.contract.SubmitTransaction(i.function, args...)
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
	ch       *fabric.Channel
	er       *EnclaveRegistry
	contract Contract
	id       view.Identity
	ip       IdentityProvider

	cid string
}

func (c *Chaincode) Invoke(function string, args ...interface{}) *ChaincodeInvocation {
	return &ChaincodeInvocation{
		Chaincode: c,
		function:  function,
		args:      args,
	}
}
