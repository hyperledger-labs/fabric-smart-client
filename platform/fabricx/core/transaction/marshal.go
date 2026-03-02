/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/hyperledger/fabric-x-common/protoutil"
	"go.uber.org/zap/zapcore"
)

func (t *Transaction) createSCEnvelope() (*cb.Envelope, error) {
	logger.Debugf("generate envelope with sc transaction [txID=%s]", t.ID())

	resps, err := t.ProposalResponses()
	if err != nil {
		return nil, errors.Wrap(err, "getting proposal responses")
	}

	// TODO: to implement MSP-based endorsement, we need to merge all proposal responses received from the endorsers.
	// Do the following steps:
	// 1. Get the "original" transaction we sent to the endorsers
	// 2. For each proposal response
	//   - extract endorsements and merge into the "original" transaction

	// TODO: pick the correct signed proposal response; currently we assume "the last" response is the correct one
	// Note that this will be changed once the committer-x fully supports msp-based endorsement policies
	if len(resps) < 1 {
		return nil, errors.Errorf("number of responses must be larger than 0, actual %d", len(resps))
	}
	response := resps[len(resps)-1]
	rawTx := response.Results()

	var tx applicationpb.Tx
	if err := proto.Unmarshal(rawTx, &tx); err != nil {
		return nil, errors.Wrapf(err, "unmarshal tx [txID=%s]", t.ID())
	}

	// get endorsements from response
	var endorsements []*applicationpb.Endorsements
	if err := json.Unmarshal(response.EndorserSignature(), &endorsements); err != nil {
		return nil, errors.Wrap(err, "unmarshal endorser signatures")
	}
	tx.Endorsements = endorsements

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		str, _ := json.MarshalIndent(&tx, "", "\t")
		logger.Debugf("fabricx transaction: %s", str)
	}

	// marshall transaction
	rawTx, err = proto.Marshal(&tx)
	if err != nil {
		return nil, errors.Wrapf(err, "marshal tx [txID=%s]", t.ID())
	}

	// produce the envelope and sign it
	signerID := t.Creator()
	signer, err := t.fns.SignerService().GetSigner(signerID)
	if err != nil {
		return nil, errors.Wrapf(err, "signer not found for %s while creating tx envelope for ordering", signerID.UniqueID())
	}

	signatureHeader := &cb.SignatureHeader{Creator: signerID, Nonce: t.Nonce()}
	channelHeader := protoutil.MakeChannelHeader(cb.HeaderType_MESSAGE, 0, t.Channel(), 0)
	channelHeader.TxId = t.ID()
	header := &cb.Header{
		ChannelHeader:   protoutil.MarshalOrPanic(channelHeader),
		SignatureHeader: protoutil.MarshalOrPanic(signatureHeader),
	}
	return fabricutils.CreateEnvelope(
		&signerWrapper{creator: t.Creator(), signer: signer},
		header,
		rawTx,
	)
}
