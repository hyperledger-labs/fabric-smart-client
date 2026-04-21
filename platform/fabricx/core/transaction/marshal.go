/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/protoutil"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
)

func (t *Transaction) createSCEnvelope() (*cb.Envelope, error) {
	logger.Debugf("generate envelope with sc transaction [txID=%s]", t.ID())

	resps, err := t.ProposalResponses()
	if err != nil {
		return nil, errors.Wrap(err, "getting proposal responses")
	}
	if len(resps) < 1 {
		return nil, errors.Errorf("number of responses must be larger than 0, actual %d", len(resps))
	}

	tx, err := mergeProposalResponseEndorsements(resps)
	if err != nil {
		return nil, errors.Wrapf(err, "merge proposal response endorsements for tx [%s]", t.ID())
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		str, _ := json.MarshalIndent(tx, "", "\t")
		logger.Debugf("fabricx transaction: %s", str)
	}

	// marshall transaction
	rawTx, err := proto.Marshal(tx)
	if err != nil {
		return nil, errors.Wrapf(err, "marshal tx [txID=%s]", t.ID())
	}

	// produce the envelope and sign it
	signerID := t.Creator()
	signer, err := t.fns.SignerService().GetSigner(signerID)
	if err != nil {
		return nil, errors.Wrapf(err, "signer not found for %s while creating tx envelope for ordering", signerID.UniqueID())
	}

	// Use cached identity (cert ID hash) for the client signature's Creator
	// field when configured, matching the endorser identity format.
	creator := signerID
	if t.useCachedIdentities {
		mspID, err := toMSPSignerIdentityWithCertificateId(signerID)
		if err != nil {
			return nil, errors.Wrap(err, "converting client identity to cached format")
		}
		creator, err = proto.Marshal(mspID)
		if err != nil {
			return nil, errors.Wrap(err, "marshaling cached client identity")
		}
	}

	signatureHeader := &cb.SignatureHeader{Creator: creator, Nonce: t.Nonce()}
	channelHeader := protoutil.MakeChannelHeader(cb.HeaderType_MESSAGE, 0, t.Channel(), 0)
	channelHeader.TxId = t.ID()
	header := &cb.Header{
		ChannelHeader:   protoutil.MarshalOrPanic(channelHeader),
		SignatureHeader: protoutil.MarshalOrPanic(signatureHeader),
	}
	return fabricutils.CreateEnvelope(
		&signerWrapper{creator: creator, signer: signer},
		header,
		rawTx,
	)
}
