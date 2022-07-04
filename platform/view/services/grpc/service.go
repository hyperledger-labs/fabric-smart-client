/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/pkg/errors"
)

var grpcServiceLookUp = &GRPCServer{}

func GetService(sp view.ServiceProvider) (*GRPCServer, error) {
	s, err := sp.GetService(grpcServiceLookUp)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get web Service from registry")
	}
	return s.(*GRPCServer), nil
}
