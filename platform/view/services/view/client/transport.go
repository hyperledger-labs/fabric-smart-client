/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"crypto/tls"
	"io"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

// Transport carries signed view commands to a node's view service. The Client builds, signs
// and checks every command and response itself; a transport only moves the messages, so
// every transport gives the same results and errors.
type Transport interface {
	// Open returns a view service client and a closer that releases what Open acquired.
	Open() (protos.ViewServiceClient, io.Closer, error)
	// Certificate returns the client TLS certificate the transport presents, or nil. Commands
	// name its hash, which the view service checks against the connection.
	Certificate() *tls.Certificate
}

// TransportType selects how a Client reaches the view service.
type TransportType string

const (
	// TransportGRPC reaches the view service on the node's fsc.grpc listener.
	TransportGRPC TransportType = "grpc"
	// TransportHTTP reaches the view service's HTTP binding on the node's fsc.web listener.
	TransportHTTP TransportType = "http"
)

// grpcTransport opens one gRPC connection per command.
type grpcTransport struct {
	address string
	client  *grpc2.Client
}

func (t *grpcTransport) Open() (protos.ViewServiceClient, io.Closer, error) {
	conn, err := t.client.NewConnection(t.address)
	if err != nil {
		if conn != nil {
			_ = conn.Close()
		}
		return nil, nil, errors.Wrapf(err, "failed creating connection to [%s]", t.address)
	}
	return protos.NewViewServiceClient(conn), conn, nil
}

func (t *grpcTransport) Certificate() *tls.Certificate {
	cert := t.client.Certificate()
	return &cert
}
