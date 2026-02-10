/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package workload

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/grpc/remote"
	"google.golang.org/grpc"
)

const (
	ECDSA = "ecdsa"
)

var (
	sk *ecdsa.PrivateKey
)

func init() {
	sk, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

func CreateClientFuncECDSA(conn *grpc.ClientConn) ClientFunc {
	var msg = &remote.Request{
		Workload: ECDSA,
		Input:    []byte("Hello"),
	}

	client := remote.NewBenchmarkServiceClient(conn)
	return func(ctx context.Context) error {
		// DODO change
		resp, err := client.Process(ctx, msg)
		if err != nil {
			return err
		}
		_ = resp
		return nil
	}
}

func ProcessECDSA(_ context.Context, in *remote.Request) (*remote.Response, error) {
	hash := sha256.Sum256(in.Input)

	sig, err := ecdsa.SignASN1(rand.Reader, sk, hash[:])
	if err != nil {
		return nil, err
	}

	enc := base64.StdEncoding
	buf := make([]byte, enc.EncodedLen(len(sig)))
	enc.Encode(buf, sig)

	return &remote.Response{Output: buf}, nil
}
