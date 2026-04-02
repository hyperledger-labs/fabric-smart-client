/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	msp_proto "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-x-common/api/msppb"
	"github.com/hyperledger/fabric-x-common/common/channelconfig"
	"github.com/hyperledger/fabric-x-common/common/configtx"
	"github.com/hyperledger/fabric-x-common/core/config/configtest"
	fxmsp "github.com/hyperledger/fabric-x-common/msp"
	"github.com/hyperledger/fabric-x-common/protoutil"
	"github.com/hyperledger/fabric-x-common/tools/configtxgen"
	"github.com/stretchr/testify/require"
)

// --- minimal mocks ---

type mockConfigtxValidator struct {
	configtx.Validator
	id string
}

func (m *mockConfigtxValidator) ChannelID() string { return m.id }

type mockAppCapabilities struct {
	channelconfig.ApplicationCapabilities
	err error
}

func (m *mockAppCapabilities) Supported() error { return m.err }

type mockChannelCapabilities struct {
	channelconfig.ChannelCapabilities
	err error
}

func (m *mockChannelCapabilities) Supported() error { return m.err }

type mockChannel struct {
	channelconfig.Channel
	caps channelconfig.ChannelCapabilities
}

func (m *mockChannel) Capabilities() channelconfig.ChannelCapabilities { return m.caps }

type mockApplicationOrg struct {
	channelconfig.ApplicationOrg
	mspID string
}

func (m *mockApplicationOrg) MSPID() string { return m.mspID }

type mockApplication struct {
	channelconfig.Application
	orgs map[string]channelconfig.ApplicationOrg
	caps channelconfig.ApplicationCapabilities
}

func (m *mockApplication) Organizations() map[string]channelconfig.ApplicationOrg { return m.orgs }
func (m *mockApplication) Capabilities() channelconfig.ApplicationCapabilities    { return m.caps }

type mockMSP struct {
	fxmsp.MSP
	tlsRootCerts [][]byte
	tlsIntCerts  [][]byte
}

func (m *mockMSP) GetTLSRootCerts() [][]byte         { return m.tlsRootCerts }
func (m *mockMSP) GetTLSIntermediateCerts() [][]byte { return m.tlsIntCerts }

type mockOrdererOrg struct {
	channelconfig.OrdererOrg
	endpoints []string
	mspImpl   fxmsp.MSP
}

func (m *mockOrdererOrg) Endpoints() []string { return m.endpoints }
func (m *mockOrdererOrg) MSP() fxmsp.MSP      { return m.mspImpl }

type mockOrderer struct {
	channelconfig.Orderer
	consensusType string
	orgs          map[string]channelconfig.OrdererOrg
}

func (m *mockOrderer) ConsensusType() string                              { return m.consensusType }
func (m *mockOrderer) Organizations() map[string]channelconfig.OrdererOrg { return m.orgs }

type mockMSPIdentity struct {
	fxmsp.Identity
	validateErr error
}

func (m *mockMSPIdentity) Validate() error              { return m.validateErr }
func (m *mockMSPIdentity) Verify(msg, sig []byte) error { return nil }
func (m *mockMSPIdentity) GetMSPIdentifier() string     { return "Org1MSP" }

type mockMSPManager struct {
	fxmsp.MSPManager
	identity       fxmsp.Identity
	deserializeErr error
}

func (m *mockMSPManager) DeserializeIdentity(identity *msppb.Identity) (fxmsp.Identity, error) {
	return m.identity, m.deserializeErr
}

type mockResources struct {
	channelconfig.Resources
	appCfg      channelconfig.Application
	appCfgOK    bool
	ordCfg      channelconfig.Orderer
	ordCfgOK    bool
	mspMgr      fxmsp.MSPManager
	chanCfg     channelconfig.Channel
	txValidator configtx.Validator
}

func (m *mockResources) ApplicationConfig() (channelconfig.Application, bool) {
	return m.appCfg, m.appCfgOK
}
func (m *mockResources) OrdererConfig() (channelconfig.Orderer, bool) { return m.ordCfg, m.ordCfgOK }
func (m *mockResources) MSPManager() fxmsp.MSPManager                 { return m.mspMgr }
func (m *mockResources) ChannelConfig() channelconfig.Channel         { return m.chanCfg }
func (m *mockResources) ConfigtxValidator() configtx.Validator        { return m.txValidator }

type mockConfigService struct {
	fdriver.ConfigService
	orderingTLSEnabled      bool
	orderingTLSEnabledIsSet bool
	tlsEnabled              bool
	orderingClientAuth      bool
	orderingClientAuthIsSet bool
	tlsClientAuth           bool
	clientConnTimeout       time.Duration
}

func (m *mockConfigService) OrderingTLSEnabled() (bool, bool) {
	return m.orderingTLSEnabled, m.orderingTLSEnabledIsSet
}
func (m *mockConfigService) TLSEnabled() bool { return m.tlsEnabled }
func (m *mockConfigService) OrderingTLSClientAuthRequired() (bool, bool) {
	return m.orderingClientAuth, m.orderingClientAuthIsSet
}
func (m *mockConfigService) TLSClientAuthRequired() bool      { return m.tlsClientAuth }
func (m *mockConfigService) ClientConnTimeout() time.Duration { return m.clientConnTimeout }

// --- helpers ---

func serializedIdentity(t *testing.T, mspID string) []byte {
	t.Helper()
	data, err := proto.Marshal(&msp_proto.SerializedIdentity{Mspid: mspID, IdBytes: []byte("cert")})
	require.NoError(t, err)
	return data
}

func appChannelGenesisEnvelope(t *testing.T, channelID string) *cb.Envelope {
	t.Helper()
	conf := configtxgen.Load(configtxgen.SampleAppChannelEtcdRaftProfile, configtest.GetDevConfigDir())
	gb := configtxgen.New(conf).GenesisBlockForChannel(channelID)
	return protoutil.ExtractEnvelopeOrPanic(gb, 0)
}

// --- tests ---

func TestNewService(t *testing.T) {
	s := NewService("mychannel")
	require.NotNil(t, s)
	require.Equal(t, "mychannel", s.channelID)
	require.Nil(t, s.channelResources)
}

func TestToMSPIdentity(t *testing.T) {
	t.Run("valid identity", func(t *testing.T) {
		data := serializedIdentity(t, "Org1MSP")
		result, err := toMSPIdentity(data)
		require.NoError(t, err)
		require.Equal(t, "Org1MSP", result.MspId)
	})

	t.Run("empty bytes yield empty identity", func(t *testing.T) {
		result, err := toMSPIdentity([]byte{})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Empty(t, result.MspId)
	})

	t.Run("invalid proto bytes return error", func(t *testing.T) {
		// 0xff is an incomplete varint — invalid protobuf
		_, err := toMSPIdentity([]byte{0xff})
		require.Error(t, err)
	})
}

func TestCapabilitiesSupported(t *testing.T) {
	t.Run("no application config returns error with channel id", func(t *testing.T) {
		r := &mockResources{
			appCfgOK:    false,
			txValidator: &mockConfigtxValidator{id: "ch1"},
		}
		err := capabilitiesSupported(r)
		require.Error(t, err)
		require.Contains(t, err.Error(), "ch1")
	})

	t.Run("application capabilities not supported", func(t *testing.T) {
		r := &mockResources{
			appCfg:      &mockApplication{caps: &mockAppCapabilities{err: errors.New("app cap unsupported")}},
			appCfgOK:    true,
			txValidator: &mockConfigtxValidator{id: "ch1"},
		}
		err := capabilitiesSupported(r)
		require.Error(t, err)
		require.Contains(t, err.Error(), "app cap unsupported")
	})

	t.Run("channel capabilities not supported", func(t *testing.T) {
		r := &mockResources{
			appCfg:      &mockApplication{caps: &mockAppCapabilities{}},
			appCfgOK:    true,
			chanCfg:     &mockChannel{caps: &mockChannelCapabilities{err: errors.New("chan cap unsupported")}},
			txValidator: &mockConfigtxValidator{id: "ch1"},
		}
		err := capabilitiesSupported(r)
		require.Error(t, err)
		require.Contains(t, err.Error(), "chan cap unsupported")
	})

	t.Run("all capabilities supported", func(t *testing.T) {
		r := &mockResources{
			appCfg:   &mockApplication{caps: &mockAppCapabilities{}},
			appCfgOK: true,
			chanCfg:  &mockChannel{caps: &mockChannelCapabilities{}},
		}
		err := capabilitiesSupported(r)
		require.NoError(t, err)
	})
}

func TestService_Update(t *testing.T) {
	t.Run("invalid envelope payload returns error", func(t *testing.T) {
		s := NewService("ch1")
		err := s.Update(&cb.Envelope{Payload: []byte("not-a-proto")})
		require.Error(t, err)
	})

	t.Run("valid genesis envelope succeeds", func(t *testing.T) {
		s := NewService("testchannel")
		env := appChannelGenesisEnvelope(t, "testchannel")
		err := s.Update(env)
		require.NoError(t, err)
		require.NotNil(t, s.channelResources)
	})
}

func TestService_DryUpdate(t *testing.T) {
	t.Run("invalid envelope payload returns error", func(t *testing.T) {
		s := NewService("ch1")
		err := s.DryUpdate(&cb.Envelope{Payload: []byte("not-a-proto")})
		require.Error(t, err)
	})

	t.Run("valid genesis envelope succeeds without mutating resources", func(t *testing.T) {
		s := NewService("testchannel")
		env := appChannelGenesisEnvelope(t, "testchannel")
		err := s.DryUpdate(env)
		require.NoError(t, err)
		require.Nil(t, s.channelResources, "DryUpdate must not mutate channelResources")
	})
}

func TestService_IsValid(t *testing.T) {
	t.Run("invalid identity bytes return error", func(t *testing.T) {
		s := NewService("ch1")
		s.channelResources = &mockResources{}
		err := s.IsValid([]byte{0xff})
		require.Error(t, err)
	})

	t.Run("deserialization error is propagated", func(t *testing.T) {
		s := NewService("ch1")
		s.channelResources = &mockResources{
			mspMgr: &mockMSPManager{deserializeErr: errors.New("deserialization failed")},
		}
		err := s.IsValid(serializedIdentity(t, "Org1MSP"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "deserialization failed")
	})

	t.Run("validate error is propagated", func(t *testing.T) {
		s := NewService("ch1")
		s.channelResources = &mockResources{
			mspMgr: &mockMSPManager{identity: &mockMSPIdentity{validateErr: errors.New("invalid cert")}},
		}
		err := s.IsValid(serializedIdentity(t, "Org1MSP"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid cert")
	})

	t.Run("valid identity returns nil", func(t *testing.T) {
		s := NewService("ch1")
		s.channelResources = &mockResources{
			mspMgr: &mockMSPManager{identity: &mockMSPIdentity{}},
		}
		err := s.IsValid(serializedIdentity(t, "Org1MSP"))
		require.NoError(t, err)
	})
}

func TestService_GetVerifier(t *testing.T) {
	t.Run("invalid identity bytes return error", func(t *testing.T) {
		s := NewService("ch1")
		s.channelResources = &mockResources{}
		_, err := s.GetVerifier([]byte{0xff})
		require.Error(t, err)
	})

	t.Run("deserialization error is propagated", func(t *testing.T) {
		s := NewService("ch1")
		s.channelResources = &mockResources{
			mspMgr: &mockMSPManager{deserializeErr: errors.New("deserialization failed")},
		}
		_, err := s.GetVerifier(serializedIdentity(t, "Org1MSP"))
		require.Error(t, err)
	})

	t.Run("success returns verifier", func(t *testing.T) {
		s := NewService("ch1")
		identity := &mockMSPIdentity{}
		s.channelResources = &mockResources{
			mspMgr: &mockMSPManager{identity: identity},
		}
		v, err := s.GetVerifier(serializedIdentity(t, "Org1MSP"))
		require.NoError(t, err)
		require.Equal(t, identity, v)
	})
}

func TestService_GetMSPIDs(t *testing.T) {
	t.Run("no application config returns nil", func(t *testing.T) {
		s := NewService("ch1")
		s.channelResources = &mockResources{appCfgOK: false}
		require.Nil(t, s.GetMSPIDs())
	})

	t.Run("nil organizations returns nil", func(t *testing.T) {
		s := NewService("ch1")
		s.channelResources = &mockResources{
			appCfg:   &mockApplication{orgs: nil},
			appCfgOK: true,
		}
		require.Nil(t, s.GetMSPIDs())
	})

	t.Run("returns MSP IDs from all organizations", func(t *testing.T) {
		s := NewService("ch1")
		s.channelResources = &mockResources{
			appCfg: &mockApplication{
				orgs: map[string]channelconfig.ApplicationOrg{
					"Org1": &mockApplicationOrg{mspID: "Org1MSP"},
					"Org2": &mockApplicationOrg{mspID: "Org2MSP"},
				},
			},
			appCfgOK: true,
		}
		ids := s.GetMSPIDs()
		require.Len(t, ids, 2)
		require.ElementsMatch(t, []string{"Org1MSP", "Org2MSP"}, ids)
	})
}

func TestService_OrdererConfig(t *testing.T) {
	t.Run("no orderer config returns error", func(t *testing.T) {
		s := NewService("ch1")
		s.channelResources = &mockResources{ordCfgOK: false}
		_, _, err := s.OrdererConfig(&mockConfigService{})
		require.Error(t, err)
	})

	t.Run("nil organizations returns error", func(t *testing.T) {
		s := NewService("ch1")
		s.channelResources = &mockResources{
			ordCfg:   &mockOrderer{consensusType: "etcdraft", orgs: nil},
			ordCfgOK: true,
		}
		_, _, err := s.OrdererConfig(&mockConfigService{})
		require.Error(t, err)
	})

	t.Run("empty endpoint is skipped", func(t *testing.T) {
		mspImpl := &mockMSP{tlsRootCerts: [][]byte{[]byte("root")}}
		s := NewService("ch1")
		s.channelResources = &mockResources{
			ordCfg: &mockOrderer{
				consensusType: "etcdraft",
				orgs: map[string]channelconfig.OrdererOrg{
					"Org1": &mockOrdererOrg{endpoints: []string{""}, mspImpl: mspImpl},
				},
			},
			ordCfgOK: true,
		}
		consType, conns, err := s.OrdererConfig(&mockConfigService{})
		require.NoError(t, err)
		require.Equal(t, "etcdraft", consType)
		require.Empty(t, conns)
	})

	t.Run("tls settings taken from ordering config when set", func(t *testing.T) {
		mspImpl := &mockMSP{
			tlsRootCerts: [][]byte{[]byte("root")},
			tlsIntCerts:  [][]byte{[]byte("int")},
		}
		s := NewService("ch1")
		s.channelResources = &mockResources{
			ordCfg: &mockOrderer{
				consensusType: "etcdraft",
				orgs: map[string]channelconfig.OrdererOrg{
					"Org1": &mockOrdererOrg{endpoints: []string{"orderer:7050"}, mspImpl: mspImpl},
				},
			},
			ordCfgOK: true,
		}
		cs := &mockConfigService{
			orderingTLSEnabled:      true,
			orderingTLSEnabledIsSet: true,
			orderingClientAuth:      true,
			orderingClientAuthIsSet: true,
			clientConnTimeout:       5 * time.Second,
		}
		consType, conns, err := s.OrdererConfig(cs)
		require.NoError(t, err)
		require.Equal(t, "etcdraft", consType)
		require.Len(t, conns, 1)
		require.Equal(t, "orderer:7050", conns[0].Address)
		require.True(t, conns[0].TLSEnabled)
		require.True(t, conns[0].TLSClientSideAuth)
		require.Equal(t, 5*time.Second, conns[0].ConnectionTimeout)
		require.Equal(t, "broadcast", conns[0].Usage)
		require.Len(t, conns[0].TLSRootCertBytes, 2) // root + int
	})

	t.Run("tls falls back to cs when not set in ordering config", func(t *testing.T) {
		mspImpl := &mockMSP{}
		s := NewService("ch1")
		s.channelResources = &mockResources{
			ordCfg: &mockOrderer{
				consensusType: "etcdraft",
				orgs: map[string]channelconfig.OrdererOrg{
					"Org1": &mockOrdererOrg{endpoints: []string{"orderer:7050"}, mspImpl: mspImpl},
				},
			},
			ordCfgOK: true,
		}
		cs := &mockConfigService{
			// ordering TLS not set → fallback to TLSEnabled
			orderingTLSEnabledIsSet: false,
			tlsEnabled:              true,
			// ordering client auth not set → fallback to TLSClientAuthRequired
			orderingClientAuthIsSet: false,
			tlsClientAuth:           false,
		}
		_, conns, err := s.OrdererConfig(cs)
		require.NoError(t, err)
		require.Len(t, conns, 1)
		require.True(t, conns[0].TLSEnabled)
		require.False(t, conns[0].TLSClientSideAuth)
	})
}

func TestService_MSPManager(t *testing.T) {
	t.Run("wraps resources MSPManager", func(t *testing.T) {
		identity := &mockMSPIdentity{}
		s := NewService("ch1")
		s.channelResources = &mockResources{
			mspMgr: &mockMSPManager{identity: identity},
		}
		mgr := s.MSPManager()
		require.NotNil(t, mgr)

		id, err := mgr.DeserializeIdentity(serializedIdentity(t, "Org1MSP"))
		require.NoError(t, err)
		require.Equal(t, identity, id)
	})

	t.Run("invalid bytes return error from DeserializeIdentity", func(t *testing.T) {
		s := NewService("ch1")
		s.channelResources = &mockResources{
			mspMgr: &mockMSPManager{},
		}
		mgr := s.MSPManager()
		_, err := mgr.DeserializeIdentity([]byte{0xff})
		require.Error(t, err)
	})
}
