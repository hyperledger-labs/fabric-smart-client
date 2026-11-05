/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// WireBytes returns the bytes to send tx on s, given raw, the JSON that tx.Bytes or
// tx.BytesNoTransient returned. At version 1 of the component of the transaction format of
// tx's driver (protos.TransactionFormat.Component) on s, that is raw unchanged; from version 2,
// the TransactionEnvelope of raw (see protos.EnvelopeFromJSONFor). Receivers read both forms. A
// tx on a network whose driver has no registered transaction format has no envelope and is
// sent as raw. It fails if the version cannot be negotiated or raw cannot be converted.
func WireBytes(ctx context.Context, s view.Session, tx *Transaction, raw []byte) ([]byte, error) {
	env, err := sendsEnvelope(ctx, s, tx)
	if err != nil || !env {
		return raw, err
	}
	return protos.EnvelopeFromJSONFor(raw, driverName(tx))
}

// networkTx is a transaction whose network determines the formats it is exchanged in.
type networkTx interface {
	FabricNetworkService() *fabric.NetworkService
}

// sendsEnvelope reports whether s negotiated version 2 or later of the component of tx. From
// that version, tx is sent on s as a TransactionEnvelope (see WireBytes) and its endorsements
// are answered as an EndorsementResponse.
func sendsEnvelope(ctx context.Context, s view.Session, tx networkTx) (bool, error) {
	c := component(tx)
	if c == "" {
		return false, nil
	}
	v, err := view.SessionVersion(ctx, s, c)
	if err != nil {
		return false, errors.WithMessagef(err, "failed getting version of [%s]", c)
	}
	return v >= 2, nil
}

// driverName returns the driver name of the network of tx.
func driverName(tx networkTx) string {
	return tx.FabricNetworkService().ConfigService().DriverName()
}

// component returns the name of the component that owns the transaction formats of the
// network of tx, or "" if its driver has no registered transaction format.
func component(tx networkTx) string {
	f, _ := protos.TransactionFormatOf(driverName(tx))
	return f.Component
}

// decodeResponses returns the marshalled ProposalResponses an endorser answered with. raw is
// JSON if it starts with '[' or 'n' (an array of byte strings, or null, as endorseView sends
// at version 1) or '{' (a Response, as endorsementsOnProposalResponderView sends at version
// 1), and an EndorsementResponse otherwise. It fails on empty input.
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
