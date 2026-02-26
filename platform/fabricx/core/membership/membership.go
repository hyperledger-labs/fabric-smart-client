/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-x-common/api/msppb"
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
		return nil, errors.Wrapf(err, "unmarshal common payload")
	}

	cenv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal config envelope")
	}

	// check if config tx is valid
	if c.channelResources != nil {
		v := c.channelResources.ConfigtxValidator()
		if err := v.Validate(cenv); err != nil {
			return nil, errors.Wrap(err, "validate config transaction")
		}
	}

	bundle, err := channelconfig.NewBundle(c.channelID, cenv.Config, factory.GetDefault())
	if err != nil {
		return nil, errors.Wrapf(err, "build a new bundle")
	}

	channelconfig.LogSanityChecks(bundle)
	if err := capabilitiesSupported(bundle); err != nil {
		return nil, errors.Wrapf(err, "check bundle capabilities")
	}

	return bundle, nil
}

func capabilitiesSupported(res channelconfig.Resources) error {
	ac, ok := res.ApplicationConfig()
	if !ok {
		return errors.Errorf("[Channel %s] does not have application config so is incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := ac.Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "[Channel %s] application config capabilities incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := res.ChannelConfig().Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "[Channel %s] channel config capabilities incompatible", res.ConfigtxValidator().ChannelID())
	}

	return nil
}

func toMSPIdentity(identity view.Identity) (*msppb.Identity, error) {
	sId := &m.SerializedIdentity{}
	err := proto.Unmarshal(identity, sId)
	if err != nil {
		return nil, err
	}

	sid := &msppb.Identity{
		MspId: sId.GetMspid(),
		Creator: &msppb.Identity_Certificate{
			Certificate: sId.GetIdBytes(),
		},
	}

	return sid, nil
}

func (c *Service) IsValid(identity view.Identity) error {

	sid, err := toMSPIdentity(identity)
	if err != nil {
		return err
	}

	id, err := c.resources().MSPManager().DeserializeIdentity(sid)
	if err != nil {
		return errors.Wrapf(err, "deserializing identity [%s]", identity.String())
	}

	return id.Validate()
}

func (c *Service) GetVerifier(identity view.Identity) (driver.Verifier, error) {
	sid, err := toMSPIdentity(identity)
	if err != nil {
		return nil, err
	}

	id, err := c.resources().MSPManager().DeserializeIdentity(sid)
	if err != nil {
		return nil, errors.Wrapf(err, "deserializing identity [%s]", identity.String())
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
		return "", nil, errors.New("orderer config does not exist")
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
				return "", nil, errors.Wrapf(err, "parse orderer endpoint [%s]", epStr)
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
	return &mspManager{c.resources().MSPManager()}
}

type mspManager struct {
	msp.MSPManager
}

func (m *mspManager) DeserializeIdentity(serializedIdentity []byte) (driver.MSPIdentity, error) {
	sid, err := toMSPIdentity(serializedIdentity)
	if err != nil {
		return nil, err
	}

	return m.MSPManager.DeserializeIdentity(sid)
}
