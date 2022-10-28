/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

type Manager struct {
	sp  view.ServiceProvider
	fns driver.FabricNetworkService
}

func NewManager(sp view.ServiceProvider, fns driver.FabricNetworkService) *Manager {
	return &Manager{sp: sp, fns: fns}
}

func (m *Manager) ComputeTxID(id *driver.TxID) string {
	return ComputeTxID(id)
}

func (m *Manager) NewEnvelope() driver.Envelope {
	return NewEnvelope()
}

func (m *Manager) NewProposalResponseFromBytes(raw []byte) (driver.ProposalResponse, error) {
	return NewProposalResponseFromBytes(raw)
}

func (m *Manager) NewTransaction(creator view2.Identity, nonce []byte, txid string, channel string) (driver.Transaction, error) {
	ch, err := m.fns.Channel(channel)
	if err != nil {
		return nil, err
	}

	if len(nonce) == 0 {
		nonce, err = getRandomNonce()
		if err != nil {
			return nil, err
		}
	}
	if len(txid) == 0 {
		txid = protoutil.ComputeTxID(nonce, creator)
	}

	return &Transaction{
		sp:         m.sp,
		fns:        m.fns,
		channel:    ch,
		TCreator:   creator,
		TNonce:     nonce,
		TTxID:      txid,
		TNetwork:   m.fns.Name(),
		TChannel:   channel,
		TTransient: map[string][]byte{},
	}, nil
}

func (m *Manager) NewTransactionFromBytes(channel string, raw []byte) (driver.Transaction, error) {
	ch, err := m.fns.Channel(channel)
	if err != nil {
		return nil, err
	}

	tx := &Transaction{
		sp:         m.sp,
		fns:        m.fns,
		channel:    ch,
		TChannel:   channel,
		TNetwork:   m.fns.Name(),
		TTransient: map[string][]byte{},
	}
	err = tx.SetFromBytes(raw)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (m *Manager) ToEnvelope(tx driver.Transaction) (*common.Envelope, error) {
	signerID := tx.Creator()
	signer, err := m.fns.SignerService().GetSigner(signerID)
	if err != nil {
		logger.Errorf("signer not found for %s while creating tx envelope for ordering [%s]", signerID.UniqueID(), err)
		return nil, errors.Wrapf(err, "signer not found for %s while creating tx envelope for ordering", signerID.UniqueID())
	}
	env, err := createSignedTx(
		tx.Proposal(),
		&signerWrapper{signerID, signer},
		tx.ProposalResponses()...,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "could not assemble transaction")
	}
	return env, nil
}

func getRandomNonce() ([]byte, error) {
	key := make([]byte, 24)

	_, err := rand.Read(key)
	if err != nil {
		return nil, errors.Wrap(err, "error getting random bytes")
	}
	return key, nil
}

// createSignedTx assembles an Envelope message from proposal, endorsements,
// and a signer. This function should be called by a client when it has
// collected enough endorsements for a proposal to create a transaction and
// submit it to peers for ordering
func createSignedTx(proposal driver.Proposal, signer SerializableSigner, resps ...driver.ProposalResponse) (*common.Envelope, error) {
	if len(resps) == 0 {
		return nil, errors.New("at least one proposal response is required")
	}

	// the original header
	hdr, err := protoutil.UnmarshalHeader(proposal.Header())
	if err != nil {
		return nil, err
	}

	// the original payload
	pPayl, err := protoutil.UnmarshalChaincodeProposalPayload(proposal.Payload())
	if err != nil {
		return nil, err
	}

	// check that the signer is the same that is referenced in the header
	// TODO: maybe worth removing?
	signerBytes, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(signerBytes, shdr.Creator) {
		return nil, errors.New("signer must be the same as the one referenced in the header")
	}

	// ensure that all actions are bitwise equal and that they are successful
	var a1 []byte
	var first driver.ProposalResponse
	for n, r := range resps {
		if r.ResponseStatus() < 200 || r.ResponseStatus() >= 400 {
			return nil, errors.Errorf("proposal response was not successful, error code %d, msg %s", r.ResponseStatus(), r.ResponseMessage())
		}

		if n == 0 {
			a1 = r.Payload()
			first = r
			continue
		}

		if !bytes.Equal(a1, r.Payload()) {
			upr1, err := UnpackProposalResponse(first.PR())
			if err != nil {
				return nil, err
			}
			rwset1, err := json.MarshalIndent(upr1.TxRwSet, "", "  ")
			if err != nil {
				return nil, err
			}

			upr2, err := UnpackProposalResponse(r.PR())
			if err != nil {
				return nil, err
			}
			rwset2, err := json.MarshalIndent(upr2.TxRwSet, "", "  ")
			if err != nil {
				return nil, err
			}

			if !bytes.Equal(rwset1, rwset2) {
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("ProposalResponsePayloads do not match (%v) \n[%s]\n!=\n[%s]",
						bytes.Equal(rwset1, rwset2), string(rwset1), string(rwset2),
					)
				}
			} else {
				pr1, err := json.MarshalIndent(first, "", "  ")
				if err != nil {
					return nil, err
				}
				pr2, err := json.MarshalIndent(r, "", "  ")
				if err != nil {
					return nil, err
				}

				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("ProposalResponse do not match  \n[%s]\n!=\n[%s]",
						bytes.Equal(pr1, pr2), string(pr1), string(pr2),
					)
				}
			}

			return nil, errors.Errorf(
				"ProposalResponsePayloads do not match [%s]!=[%s]",
				base64.StdEncoding.EncodeToString(a1),
				base64.StdEncoding.EncodeToString(r.Payload()),
			)
		}
	}

	// fill endorsements
	endorsements := make([]*peer.Endorsement, len(resps))
	for n, r := range resps {
		endorsements[n] = &peer.Endorsement{
			Endorser:  r.Endorser(),
			Signature: r.EndorserSignature(),
		}
	}

	// create ChaincodeEndorsedAction
	cea := &peer.ChaincodeEndorsedAction{ProposalResponsePayload: resps[0].Payload(), Endorsements: endorsements}

	// obtain the bytes of the proposal payload that will go to the transaction
	propPayloadBytes, err := protoutil.GetBytesProposalPayloadForTx(pPayl)
	if err != nil {
		return nil, err
	}

	// serialize the chaincode action payload
	chaincodeActionPayload := &peer.ChaincodeActionPayload{ChaincodeProposalPayload: propPayloadBytes, Action: cea}
	capBytes, err := protoutil.GetBytesChaincodeActionPayload(chaincodeActionPayload)
	if err != nil {
		return nil, err
	}

	// create a transaction
	taa := &peer.TransactionAction{Header: hdr.SignatureHeader, Payload: capBytes}
	taas := make([]*peer.TransactionAction, 1)
	taas[0] = taa
	tx := &peer.Transaction{Actions: taas}

	// serialize the tx
	txBytes, err := protoutil.GetBytesTransaction(tx)
	if err != nil {
		return nil, err
	}

	// create the payload
	payl := &common.Payload{Header: hdr, Data: txBytes}
	paylBytes, err := protoutil.GetBytesPayload(payl)
	if err != nil {
		return nil, err
	}

	// sign the payload
	sig, err := signer.Sign(paylBytes)
	if err != nil {
		return nil, err
	}

	// here's the envelope
	return &common.Envelope{Payload: paylBytes, Signature: sig}, nil
}
