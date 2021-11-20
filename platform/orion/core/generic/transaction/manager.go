/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

type Envelope struct {
}

func (e *Envelope) TxID() string {
	panic("implement me")
}

func (e *Envelope) Nonce() []byte {
	panic("implement me")
}

func (e *Envelope) Creator() []byte {
	panic("implement me")
}

func (e *Envelope) Results() []byte {
	panic("implement me")
}

func (e *Envelope) Bytes() ([]byte, error) {
	panic("implement me")
}

func (e *Envelope) FromBytes(raw []byte) error {
	panic("implement me")
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
