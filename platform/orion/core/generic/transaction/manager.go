/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"
)

type Envelope struct {
	env     *types.DataTxEnvelope
	results []byte
}

func (e *Envelope) TxID() string {
	return e.env.Payload.TxId
}

func (e *Envelope) Nonce() []byte {
	return nil
}

func (e *Envelope) Creator() []byte {
	return []byte(e.env.Payload.MustSignUserIds[0])
}

func (e *Envelope) Results() []byte {
	return e.results
}

func (e *Envelope) Bytes() ([]byte, error) {
	return proto.Marshal(e.env)
}

func (e *Envelope) FromBytes(raw []byte) error {
	env := &types.DataTxEnvelope{}
	if err := proto.Unmarshal(raw, env); err != nil {
		return errors.Wrapf(err, "failed to unmarshal envelope [%d]", len(raw))
	}
	results, err := proto.Marshal(env.Payload)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal payload [%d]", len(raw))
	}

	e.env = env
	e.results = results
	return nil
}

type Manager struct {
	sp view.ServiceProvider
}

func NewManager(sp view.ServiceProvider) *Manager {
	return &Manager{sp: sp}
}

func (m *Manager) ComputeTxID(id *driver.TxID) string {
	return ComputeTxID(id)
}

func (m *Manager) NewEnvelope() driver.Envelope {
	return &Envelope{}
}
