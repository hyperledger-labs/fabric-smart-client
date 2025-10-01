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
	"github.com/hyperledger/fabric/protoutil"
	"go.uber.org/zap/zapcore"
)

func (t *Transaction) createSCEnvelope() (*cb.Envelope, error) {
	logger.Debugf("generate envelope with sc transaction [txID=%s]", t.ID())

	rawTx, err := t.Results()
	if err != nil {
		return nil, fmt.Errorf("failed to get rws from proposal response: %w", err)
	}

	// append signatures to tx
	var tx protoblocktx.Tx
	if err := proto.Unmarshal(rawTx, &tx); err != nil {
		return nil, fmt.Errorf("failed to deserialize tx [%s]: %w", t.ID(), err)
	}

	// for _, response := range t.ProposalResponses() {
	//	tx.Signatures = append(tx.Signatures, response.EndorserSignature())
	//	//tx.Signatures = append(tx.Signatures, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	// }

	// just take the first one as we only support single signatures, which are meant to be threshold signatures
	resps, err := t.ProposalResponses()
	if err != nil {
		return nil, err
	}

	response := resps[len(resps)-1]
	tx.Signatures = [][]byte{response.EndorserSignature()}

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
	channelHeader.TxId = protoutil.ComputeTxID(signatureHeader.Nonce, signatureHeader.Creator)
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
