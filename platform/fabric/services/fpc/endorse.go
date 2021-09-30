/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fpc

import (
	"strconv"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

// ChaincodeEndorse models the endorsement of an FPC
type ChaincodeEndorse struct {
	*Chaincode

	function string
	args     []interface{}
}

// Call invokes the chaincode and returns a Fabric envelope
func (i *ChaincodeEndorse) Call() (*fabric.Envelope, error) {
	args, err := i.prepareArgs()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed preparing arguments")
	}
	return i.EndorserContract.EndorseTransaction(i.function, args...)
}

func (i *ChaincodeEndorse) prepareArgs() ([]string, error) {
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

func (i *ChaincodeEndorse) toString(arg interface{}) (string, error) {
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
