/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

//go:generate protoc --proto_path=testdata/grpc --go_out=paths=source_relative:testpb --go-grpc_out=paths=source_relative:testpb testdata/grpc/test.proto

package grpc_test
