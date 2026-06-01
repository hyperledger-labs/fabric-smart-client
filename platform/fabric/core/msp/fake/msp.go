/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fake

import (
	"time"

	pmsp "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	tmock "github.com/stretchr/testify/mock"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
)

type MSP struct {
	tmock.Mock
}

func (m *MSP) IsWellFormed(_ *pmsp.SerializedIdentity) error {
	return nil
}

func (m *MSP) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	args := m.Called(serializedIdentity)
	return args.Get(0).(msp.Identity), args.Error(1)
}

func (m *MSP) Setup(config *pmsp.MSPConfig) error {
	args := m.Called(config)
	return args.Error(0)
}

func (m *MSP) GetVersion() msp.MSPVersion {
	args := m.Called()
	return args.Get(0).(msp.MSPVersion)
}

func (m *MSP) GetType() msp.ProviderType {
	args := m.Called()
	return args.Get(0).(msp.ProviderType)
}

func (m *MSP) GetIdentifier() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *MSP) GetDefaultSigningIdentity() (msp.SigningIdentity, error) {
	args := m.Called()
	return args.Get(0).(msp.SigningIdentity), args.Error(1)
}

func (m *MSP) GetTLSRootCerts() [][]byte {
	args := m.Called()
	return args.Get(0).([][]byte)
}

func (m *MSP) GetTLSIntermediateCerts() [][]byte {
	args := m.Called()
	return args.Get(0).([][]byte)
}

func (m *MSP) Validate(id msp.Identity) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MSP) SatisfiesPrincipal(id msp.Identity, principal *pmsp.MSPPrincipal) error {
	args := m.Called(id, principal)
	return args.Error(0)
}

type Identity struct {
	tmock.Mock

	ID string
}

func (m *Identity) Anonymous() bool {
	panic("implement me")
}

func (m *Identity) ExpiresAt() time.Time {
	panic("implement me")
}

func (m *Identity) GetIdentifier() *msp.IdentityIdentifier {
	args := m.Called()
	return args.Get(0).(*msp.IdentityIdentifier)
}

func (*Identity) GetMSPIdentifier() string {
	panic("implement me")
}

func (m *Identity) Validate() error {
	return m.Called().Error(0)
}

func (*Identity) GetOrganizationalUnits() []*msp.OUIdentifier {
	panic("implement me")
}

func (*Identity) Verify(msg, sig []byte) error {
	return nil
}

func (*Identity) Serialize() ([]byte, error) {
	panic("implement me")
}

func (m *Identity) SatisfiesPrincipal(principal *pmsp.MSPPrincipal) error {
	return m.Called(principal).Error(0)
}

type MockSigningIdentity struct {
	tmock.Mock
	*Identity
}

func (m *MockSigningIdentity) Sign(msg []byte) ([]byte, error) {
	args := m.Called(msg)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockSigningIdentity) GetPublicVersion() msp.Identity {
	args := m.Called()
	return args.Get(0).(msp.Identity)
}
