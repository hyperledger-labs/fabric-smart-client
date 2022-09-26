/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
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
	if e.env == nil {
		return nil, nil
	}
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

func (e *Envelope) String() string {
	s, err := json.MarshalIndent(e.env, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(s)
}

type Manager struct {
	sp view.ServiceProvider
	sm driver.SessionManager
}

func NewManager(sp view.ServiceProvider, sm driver.SessionManager) *Manager {
	return &Manager{sp: sp, sm: sm}
}

func (m *Manager) ComputeTxID(id *driver.TxID) string {
	return ComputeTxID(id)
}

func (m *Manager) NewEnvelope() driver.Envelope {
	return &Envelope{}
}

func (m *Manager) CommitEnvelope(session driver.Session, envelope driver.Envelope) error {
	logger.Debugf("CommitEnvelope [%s]", envelope.TxID())
	env, ok := envelope.(*Envelope)
	if !ok {
		return errors.New("invalid envelope type")
	}
	ldtx, err := session.LoadDataTx(env.env)
	if err != nil {
		logger.Errorf("failed to load data tx [%s]", err)
		return errors.Wrapf(err, "failed to load data tx [%s]", envelope.TxID())
	}
	if err := ldtx.Commit(); err != nil {
		logger.Errorf("failed to commit data tx [%s]", envelope.TxID())
		return errors.Wrapf(err, "failed to commit data tx [%s]", envelope.TxID())
	}
	logger.Debugf("CommitEnvelope [%s:%s] done", envelope.TxID(), ldtx.ID())
	return nil
}
