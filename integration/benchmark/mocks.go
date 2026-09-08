/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package benchmark

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type MockSigner struct {
	SerializeFunc func() ([]byte, error)
	SignFunc      func([]byte) ([]byte, error)
}

func (m *MockSigner) Serialize() ([]byte, error) {
	return m.SerializeFunc()
}

func (m *MockSigner) Sign(msg []byte) ([]byte, error) {
	return m.SignFunc(msg)
}

type MockIdentityProvider struct {
	DefaultSigner view.Identity
}

func (m *MockIdentityProvider) DefaultIdentity() view.Identity {
	return m.DefaultSigner
}

func (*MockIdentityProvider) Admins() []view.Identity {
	panic("implement me")
}

func (*MockIdentityProvider) Clients() []view.Identity {
	panic("implement me")
}

type MockSignerProvider struct {
	DefaultSigner sig.Signer
}

func (m *MockSignerProvider) GetSigner(_ view.Identity) (sig.Signer, error) {
	return m.DefaultSigner, nil
}

type MockViewManager struct {
	Constructor func() view.View
	Identity    view.Identity
}

func (m *MockViewManager) NewView(_ string, _ []byte) (view.View, error) {
	return m.Constructor(), nil
}

func (*MockViewManager) InitiateView(_ context.Context, view view.View) (any, error) {
	return view.Call(nil)
}

func (*MockViewManager) InitiateContext(_ context.Context, _ view.View) (view.Context, error) {
	panic("implement me")
}

func (*MockViewManager) DeleteContext(_ string) {
}
