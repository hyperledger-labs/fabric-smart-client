/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

// ExtractTLSCertHashFromCommand is a CertHashExtractor that returns the
// TlsCertHash field from a *protos.Command's Header.
func ExtractTLSCertHashFromCommand(msg proto.Message) []byte {
	cmd, ok := msg.(*protos.Command)
	if !ok || cmd.Header == nil {
		return nil
	}
	return cmd.Header.TlsCertHash
}
