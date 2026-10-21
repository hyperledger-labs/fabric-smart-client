/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"bytes"
	"context"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	grpcutil "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

// ExtractTLSCertHashFromCommand returns the TlsCertHash of a *protos.Command's Header, or
// nil for any other message.
func ExtractTLSCertHashFromCommand(msg proto.Message) []byte {
	cmd, ok := msg.(*protos.Command)
	if !ok || cmd.Header == nil {
		return nil
	}
	return cmd.Header.TlsCertHash
}

// BindToVerifiedClientCert is the view service's BindingInspector. When the connection a
// command arrived on carries a client TLS certificate the listener verified, the command's
// Header.TlsCertHash must be that certificate's SHA-256, so a command signed for one
// connection is rejected on another. A connection without a verified client certificate
// binds nothing. The rule depends only on the connection, so it holds for every listener
// that serves the view service, whatever its client-authentication setting.
func BindToVerifiedClientCert(ctx context.Context, msg proto.Message) error {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil
	}
	info, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(info.State.VerifiedChains) == 0 {
		return nil
	}
	if !bytes.Equal(ExtractTLSCertHashFromCommand(msg), grpcutil.ExtractCertificateHashFromContext(ctx)) {
		return errors.New("command does not name the client TLS certificate of the connection")
	}
	return nil
}
