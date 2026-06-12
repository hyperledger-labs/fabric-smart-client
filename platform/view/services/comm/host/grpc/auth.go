/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"context"
	"crypto/x509"

	ggrpc "google.golang.org/grpc/peer"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

func expectedPeerID(ctx context.Context) (string, error) {
	cert := grpc2.ExtractCertificateFromContext(ctx)
	if cert == nil {
		return "", errors.New("client didn't send a TLS certificate")
	}
	return peerIDFromCertificate(cert)
}

func peerIDFromCertificate(cert *x509.Certificate) (string, error) {
	synthesizer := PKIDSynthesizer{}
	raw, err := synthesizer.PublicKeyID(cert.PublicKey)
	if err != nil {
		return "", errors.Wrapf(err, "failed to derive peer ID from certificate")
	}
	return string(raw), nil
}

func remoteAddress(ctx context.Context) string {
	pr, ok := ggrpc.FromContext(ctx)
	if !ok || pr.Addr == nil {
		return ""
	}
	return pr.Addr.String()
}
