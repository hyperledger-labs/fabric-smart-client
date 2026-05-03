/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership/channelconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Service struct {
	// resourcesLock is used to serialize access to channelResources
	resourcesLock sync.RWMutex
	// channelResources is used to acquire ChannelConfig.
	channelResources *channelconfig.ChannelConfig

	channelName string
}

func NewService(channelName string) *Service {
	return &Service{channelName: channelName}
}

func (c *Service) resources() *channelconfig.ChannelConfig {
	c.resourcesLock.RLock()
	res := c.channelResources
	c.resourcesLock.RUnlock()
	return res
}

func (c *Service) Update(env *cb.Envelope) error {
	c.resourcesLock.Lock()
	defer c.resourcesLock.Unlock()

	b, err := c.parseConfig(env)
	if err != nil {
		return err
	}

	c.channelResources = b
	return nil
}

func (c *Service) parseConfig(env *cb.Envelope) (*channelconfig.ChannelConfig, error) {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get payload from config transaction")
	}

	cenv, err := protoutil.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, errors.Wrapf(err, "error unmarshalling config which passed initial validity checks")
	}

	return channelconfig.NewChannelConfig(cenv.Config.ChannelGroup, factory.GetDefault())
}

func (c *Service) IsValid(identity view.Identity) error {
	res := c.resources()
	if res == nil {
		return errors.Errorf("cannot check identity validity, channel resources not yet initialized for channel [%s]", c.channelName)
	}
	id, err := res.MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}

	return id.Validate()
}

func (c *Service) GetVerifier(identity view.Identity) (driver.Verifier, error) {
	res := c.resources()
	if res == nil {
		return nil, errors.Errorf("cannot get verifier, channel resources not yet initialized for channel [%s]", c.channelName)
	}
	id, err := res.MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}
	return id, nil
}

// GetMSPIDs retrieves the MSP IDs of the organizations in the current Channel
// configuration.
func (c *Service) GetMSPIDs() []string {
	res := c.resources()
	if res == nil {
		return nil
	}
	ac := res.ApplicationConfig()
	if ac == nil || ac.Organizations() == nil {
		return nil
	}

	var mspIDs []string
	for _, org := range ac.Organizations() {
		mspIDs = append(mspIDs, org.MSPID())
	}

	return mspIDs
}

func (c *Service) OrdererConfig(cs driver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
	res := c.resources()
	if res == nil {
		return "", nil, errors.Errorf("cannot get orderer config, channel resources not yet initialized for channel [%s]", c.channelName)
	}
	oc := res.OrdererConfig()
	if oc == nil {
		return "", nil, fmt.Errorf("orderer config does not exist")
	}

	tlsEnabled, isSet := cs.OrderingTLSEnabled()
	if !isSet {
		tlsEnabled = cs.TLSEnabled()
	}
	tlsClientSideAuth, isSet := cs.OrderingTLSClientAuthRequired()
	if !isSet {
		tlsClientSideAuth = cs.TLSClientAuthRequired()
	}
	connectionTimeout := cs.ClientConnTimeout()

	var newOrderers []*grpc.ConnectionConfig
	orgs := oc.Organizations()
	for _, org := range orgs {
		m := org.MSP()
		var tlsRootCerts [][]byte
		tlsRootCerts = append(tlsRootCerts, m.GetTLSRootCerts()...)
		tlsRootCerts = append(tlsRootCerts, m.GetTLSIntermediateCerts()...)
		for _, endpoint := range org.Endpoints() {
			if len(endpoint) == 0 {
				continue
			}
			newOrderers = append(newOrderers, &grpc.ConnectionConfig{
				Address:           endpoint,
				ConnectionTimeout: connectionTimeout,
				TLSEnabled:        tlsEnabled,
				TLSClientSideAuth: tlsClientSideAuth,
				TLSRootCertBytes:  tlsRootCerts,
			})
		}
	}

	return oc.ConsensusType(), newOrderers, nil
}

// MSPManager returns the msp.MSPManager that reflects the current Channel
// configuration. Users should not memoize references to this object.
func (c *Service) MSPManager() driver.MSPManager {
	res := c.resources()
	if res == nil {
		return &mspManager{FabricMSPManager: nil}
	}
	return &mspManager{FabricMSPManager: res.MSPManager()}
}

func (c *Service) CheckACL(signedProp driver.SignedProposal) error {
	return driver.ErrNotImplemented
}

type FabricMSPManager interface {
	DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error)
}

type mspManager struct {
	FabricMSPManager
}

func (m *mspManager) DeserializeIdentity(serializedIdentity []byte) (driver.MSPIdentity, error) {
	if m.FabricMSPManager == nil {
		return nil, errors.New("MSP manager not initialized")
	}
	return m.FabricMSPManager.DeserializeIdentity(serializedIdentity)
}
