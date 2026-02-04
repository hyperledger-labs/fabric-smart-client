/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"os"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

var ErrInvalidAddress = errors.New("empty address")

func GrpcClient(c *Config) (*grpc.ClientConn, error) {
	// no endpoints in config
	if len(c.Endpoints) != 1 {
		return nil, errors.New("we need a single endpoint")
	}

	// currently we only support connections to a single query service
	endpoint := c.Endpoints[0]

	// check endpoint address
	if len(endpoint.Address) == 0 {
		return nil, ErrInvalidAddress
	}

	var opts []grpc.DialOption
	opts = append(opts, WithConnectionTime(endpoint.ConnectionTimeout))
	opts = append(opts, WithTLS(endpoint))
	opts = append(opts, grpc2.ClientKeepaliveOptions(
		grpc2.KeepaliveOptions{
			ClientInterval: 20 * time.Hour,
			ClientTimeout:  21 * time.Hour,
		})...)
	opts = append(opts, grpc.WithStatsHandler(&grpc2.DisconnectTracker{ClientName: "FinalityService"}))

	return grpc.NewClient(endpoint.Address, opts...)
}

func WithTLS(endpoint Endpoint) grpc.DialOption {
	if !endpoint.TLSEnabled {
		return grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	if _, err := os.Stat(endpoint.TLSRootCertFile); errors.Is(err, os.ErrNotExist) {
		if err != nil {
			panic(err)
		}
	}

	creds, err := credentials.NewClientTLSFromFile(endpoint.TLSRootCertFile, endpoint.TLSServerNameOverride)
	if err != nil {
		panic(err)
	}

	return grpc.WithTransportCredentials(creds)
}

func WithConnectionTime(timeout time.Duration) grpc.DialOption {
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	return grpc.WithConnectParams(grpc.ConnectParams{
		Backoff:           backoff.DefaultConfig,
		MinConnectTimeout: timeout,
	})
}
