/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-committer/api/protoblocktx"
	"github.com/hyperledger/fabric-x-common/protoutil"
	"go.uber.org/zap/zapcore"
)

func (t *Transaction) createSCEnvelope() (*cb.Envelope, error) {
	logger.Debugf("generate envelope with sc transaction [txID=%s]", t.ID())

	resps, err := t.ProposalResponses()
	if err != nil {
		return nil, err
	}

	// TODO: pick the correct signed proposal response; currently we assume "the last" response is the correct one
	response := resps[len(resps)-1]
	rawTx := response.Results()

	var tx protoblocktx.Tx
	if err := proto.Unmarshal(rawTx, &tx); err != nil {
		return nil, fmt.Errorf("failed to deserialize tx [%s]: %w", t.ID(), err)
	}

	var sigs [][]byte
	if err := json.Unmarshal(response.EndorserSignature(), &sigs); err != nil {
		return nil, err
	}

	tx.Signatures = sigs

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		str, _ := json.MarshalIndent(&tx, "", "\t")
		logger.Debugf("fabricx transaction: %s", str)
	}

	rawTx, err = proto.Marshal(&tx)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tx [%s]", t.ID())
	}

	// produce the envelope and sign it
	signerID := t.Creator()
	signer, err := t.fns.SignerService().GetSigner(signerID)
	if err != nil {
		e := fmt.Errorf("signer not found for %s while creating tx envelope for ordering: %w", signerID.UniqueID(), err)
		logger.Error(e)
		return nil, e
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
