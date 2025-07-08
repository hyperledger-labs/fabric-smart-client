/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protos

//go:generate protoc --go_out=. --go-grpc_out=. --proto_path=. commands.proto service.proto
//go:generate protoc --go_out=. --proto_path=. commands.proto
