/*
Copyright IBM Corp. All Rights Reserved.
Copyright 2020 Intel Corporation

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"encoding/base64"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/fpc/core/generic/protos"
)

// MarshallProto returns a serialized protobuf message encoded as base64 string
func MarshallProto(msg proto.Message) string {
	return base64.StdEncoding.EncodeToString(protoutil.MarshalOrPanic(msg))
}

func UnmarshalCredentials(credentialsBase64 string) (*protos.Credentials, error) {
	credentialsBytes, err := base64.StdEncoding.DecodeString(credentialsBase64)
	if err != nil {
		return nil, err
	}

	if len(credentialsBytes) == 0 {
		return nil, fmt.Errorf("credential input empty")
	}

	credentials := &protos.Credentials{}
	err = proto.Unmarshal(credentialsBytes, credentials)
	if err != nil {
		return nil, err
	}
	return credentials, nil
}
