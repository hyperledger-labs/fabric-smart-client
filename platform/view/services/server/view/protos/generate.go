/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protos

//go:generate protoc commands.proto finality.proto service.proto --go-grpc_out=Mgoogle/protobuf/timestamp.proto=github.com/golang/protobuf/ptypes/timestamp:.
//go:generate protoc commands.proto finality.proto --go-grpc_out=:.
