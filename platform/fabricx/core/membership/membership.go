/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"fmt"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/common/channelconfig"
	"github.com/hyperledger/fabric-x-common/common/configtx"
	"github.com/hyperledger/fabric-x-common/msp"
	"github.com/hyperledger/fabric-x-common/protoutil"
)

var logger = logging.MustGetLogger()

type Service struct {
	// ResourcesLock is used to serialize access to resources
	resourcesLock sync.RWMutex
	// resources is used to acquire configuration bundle resources.
	channelResources channelconfig.Resources

	channelID string
}

func NewService(channelID string) *Service {
	return &Service{channelID: channelID}
}

func (c *Service) resources() channelconfig.Resources {
	c.resourcesLock.RLock()
	res := c.channelResources
	c.resourcesLock.RUnlock()
	return res
}

func (c *Service) Update(env *cb.Envelope) error {
	c.resourcesLock.Lock()
	defer c.resourcesLock.Unlock()

	b, err := c.validateConfig(env)
	if err != nil {
		return err
	}

	c.channelResources = b
	return nil
}

func (c *Service) DryUpdate(env *cb.Envelope) error {
	c.resourcesLock.RLock()
	defer c.resourcesLock.RUnlock()

	if _, err := c.validateConfig(env); err != nil {
		return err
	}

	return nil
}

func (c *Service) validateConfig(env *cb.Envelope) (*channelconfig.Bundle, error) {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get payload from config transaction")
	}

	cenv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, errors.Wrapf(err, "error unmarshalling config which passed initial validity checks")
	}

	// check if config tx is valid
	if c.channelResources != nil {
		v := c.channelResources.ConfigtxValidator()
		if err := v.Validate(cenv); err != nil {
			return nil, errors.Wrap(err, "failed to validate config transaction")
		}
	}

	bundle, err := channelconfig.NewBundle(c.channelID, cenv.Config, factory.GetDefault())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build a new bundle")
	}

	channelconfig.LogSanityChecks(bundle)
	if err := capabilitiesSupported(bundle); err != nil {
		return nil, err
	}

	return bundle, nil
}

func capabilitiesSupported(res channelconfig.Resources) error {
	ac, ok := res.ApplicationConfig()
	if !ok {
		return errors.Errorf("[Channel %s] does not have application config so is incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := ac.Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "[Channel %s] incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := res.ChannelConfig().Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "[Channel %s] incompatible", res.ConfigtxValidator().ChannelID())
	}

	return nil
}

func (c *Service) IsValid(identity view.Identity) error {
	id, err := c.resources().MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}

	return id.Validate()
}

func (c *Service) GetVerifier(identity view.Identity) (driver.Verifier, error) {
	id, err := c.resources().MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}
	return id, nil
}

// GetMSPIDs retrieves the MSP IDs of the organizations in the current Channel
// configuration.
func (c *Service) GetMSPIDs() []string {
	ac, ok := c.resources().ApplicationConfig()
	if !ok || ac.Organizations() == nil {
		return nil
	}

	mspIDs := make([]string, 0, len(ac.Organizations()))
	for _, org := range ac.Organizations() {
		mspIDs = append(mspIDs, org.MSPID())
	}

	return mspIDs
}

func (c *Service) OrdererConfig(cs driver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
	oc, ok := c.resources().OrdererConfig()
	if !ok || oc.Organizations() == nil {
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
		for _, epStr := range org.Endpoints() {
			if len(epStr) == 0 {
				continue
			}

			ep, err := parseEndpoint(epStr)
			if err != nil {
				return "", nil, fmt.Errorf("cannot parse orderer endpoint [%s]: %w", epStr, err)
			}
			logger.Debugf("new OS endpoint: %s", epStr)

			// skip all endpoints which are not of type OrdererBroadcastType
			if ep.Type != OrdererBroadcastType {
				continue
			}

			// TODO: what should we do with the endpoint id?

			newOrderers = append(newOrderers, &grpc.ConnectionConfig{
				Address:           ep.Endpoint,
				ConnectionTimeout: connectionTimeout,
				TLSEnabled:        tlsEnabled,
				TLSClientSideAuth: tlsClientSideAuth,
				TLSRootCertBytes:  tlsRootCerts,
				Usage:             ep.Type,
			})
		}
	}

	return oc.ConsensusType(), newOrderers, nil
}

// MSPManager returns the msp.MSPManager that reflects the current Channel
// configuration. Users should not memoize references to this object.
func (c *Service) MSPManager() driver.MSPManager {
	return &mspManager{FabricMSPManager: c.resources().MSPManager()}
}

type FabricMSPManager interface {
	DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error)
}

type mspManager struct {
	FabricMSPManager
}

func (m *mspManager) DeserializeIdentity(serializedIdentity []byte) (driver.MSPIdentity, error) {
	return m.FabricMSPManager.DeserializeIdentity(serializedIdentity)
}
