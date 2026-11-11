/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
)

// WireBytes returns the bytes to send tx as, given raw, the JSON that tx.Bytes or
// tx.BytesNoTransient returned: the TransactionEnvelope of raw (see
// protos.EnvelopeFromJSONFor) if the driver of tx's network has a registered transaction
// format, and raw otherwise. Receivers read both forms. It fails if raw cannot be converted.
func WireBytes(tx *Transaction, raw []byte) ([]byte, error) {
	if !sendsEnvelope(tx) {
		return raw, nil
	}
	return protos.EnvelopeFromJSONFor(raw, driverName(tx))
}

// networkTx is a transaction whose network determines the formats it is exchanged in.
type networkTx interface {
	FabricNetworkService() *fabric.NetworkService
}

// sendsEnvelope reports whether the driver of tx's network has a registered transaction
// format. Such a transaction is sent as a TransactionEnvelope (see WireBytes) and its
// endorsements are answered as an EndorsementResponse; any other is sent and answered as JSON.
func sendsEnvelope(tx networkTx) bool {
	_, ok := protos.TransactionFormatOf(driverName(tx))
	return ok
}

// driverName returns the driver name of the network of tx.
func driverName(tx networkTx) string {
	return tx.FabricNetworkService().ConfigService().DriverName()
}

// decodeResponses returns the marshalled ProposalResponses an endorser answered with. raw is
// JSON if it starts with '[' or 'n' (an array of byte strings, or null, as endorseView sends
// for a transaction without a registered format) or '{' (a Response, as
// endorsementsOnProposalResponderView sends for one), and an EndorsementResponse otherwise.
// It fails on empty input.
func decodeResponses(raw []byte) ([][]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("empty endorsement response")
	}
	switch raw[0] {
	case '[', 'n':
		var prs [][]byte
		if err := json.Unmarshal(raw, &prs); err != nil {
			return nil, errors.Wrap(err, "failed unmarshalling endorsement response")
		}
		return prs, nil
	case '{':
		r := &Response{}
		if err := json.Unmarshal(raw, r); err != nil {
			return nil, errors.Wrap(err, "failed unmarshalling endorsement response")
		}
		return r.ProposalResponses, nil
	default:
		r := &protos.EndorsementResponse{}
		if err := proto.Unmarshal(raw, r); err != nil {
			return nil, errors.Wrap(err, "failed unmarshalling endorsement response")
		}
		return r.GetProposalResponses(), nil
	}
}
