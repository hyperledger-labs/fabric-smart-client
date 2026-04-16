/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"sync"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-x-common/api/msppb"
	"github.com/hyperledger/fabric-x-common/common/channelconfig"
	"github.com/hyperledger/fabric-x-common/common/configtx"
	"github.com/hyperledger/fabric-x-common/common/policies"
	"github.com/hyperledger/fabric-x-common/core/aclmgmt"
	"github.com/hyperledger/fabric-x-common/core/aclmgmt/resources"
	"github.com/hyperledger/fabric-x-common/core/policy"
	"github.com/hyperledger/fabric-x-common/msp"
	"github.com/hyperledger/fabric-x-common/msp/mgmt"
	"github.com/hyperledger/fabric-x-common/protoutil"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

type Service struct {
	// ResourcesLock is used to serialize access to resources
	resourcesLock sync.RWMutex
	// resources is used to acquire configuration bundle resources.
	channelResources channelconfig.Resources

	ACLProvider aclmgmt.ACLProvider

	channelID string
}

func NewService(channelID string) *Service {
	s := &Service{
		channelID:   channelID,
		ACLProvider: nil,
	}
	policyChecker := policy.NewPolicyChecker(
		&policyManagerGetterFunc{channelID: channelID, resourcesGetter: s.resources},
		mgmt.GetLocalMSP(factory.GetDefault()),
	)
	s.ACLProvider = aclmgmt.NewACLProvider(
		func(cid string) channelconfig.Resources {
			if cid == s.channelID {
				return s.resources()
			}
			return nil
		},
		policyChecker,
	)

	return s
}

func (s *Service) resources() channelconfig.Resources {
	s.resourcesLock.RLock()
	res := s.channelResources
	s.resourcesLock.RUnlock()
	return res
}

func (s *Service) Update(env *cb.Envelope) error {
	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	logger.Infof("updating channel [%s]", s.channelID)

	b, err := s.validateConfig(env)
	if err != nil {
		logger.Errorf("failed validating config for channel [%s]: [%s]", s.channelID, err)
		return err
	}

	s.channelResources = b

	logger.Infof("updating channel [%s], done", s.channelID)
	return nil
}

func (s *Service) DryUpdate(env *cb.Envelope) error {
	s.resourcesLock.RLock()
	defer s.resourcesLock.RUnlock()

	if _, err := s.validateConfig(env); err != nil {
		return err
	}

	return nil
}

func (s *Service) validateConfig(env *cb.Envelope) (*channelconfig.Bundle, error) {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal common payload")
	}

	cenv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal config envelope")
	}

	// check if config tx is valid
	if s.channelResources != nil {
		v := s.channelResources.ConfigtxValidator()
		if err := v.Validate(cenv); err != nil {
			return nil, errors.Wrap(err, "validate config transaction")
		}
	}

	bundle, err := channelconfig.NewBundle(s.channelID, cenv.Config, factory.GetDefault())
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

func (s *Service) IsValid(identity view.Identity) error {
	sid, err := toMSPIdentity(identity)
	if err != nil {
		return err
	}

	id, err := s.resources().MSPManager().DeserializeIdentity(sid)
	if err != nil {
		return errors.Wrapf(err, "deserializing identity [%s]", identity.String())
	}

	return id.Validate()
}

func (s *Service) GetVerifier(identity view.Identity) (driver.Verifier, error) {
	sid, err := toMSPIdentity(identity)
	if err != nil {
		return nil, err
	}

	id, err := s.resources().MSPManager().DeserializeIdentity(sid)
	if err != nil {
		return nil, errors.Wrapf(err, "deserializing identity [%s]", identity.String())
	}
	return id, nil
}

// GetMSPIDs retrieves the MSP IDs of the organizations in the current Channel
// configuration.
func (s *Service) GetMSPIDs() []string {
	ac, ok := s.resources().ApplicationConfig()
	if !ok || ac.Organizations() == nil {
		return nil
	}

	mspIDs := make([]string, 0, len(ac.Organizations()))
	for _, org := range ac.Organizations() {
		mspIDs = append(mspIDs, org.MSPID())
	}

	return mspIDs
}

func (s *Service) OrdererConfig(cs driver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
	oc, ok := s.resources().OrdererConfig()
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

			newOrderers = append(newOrderers, &grpc.ConnectionConfig{
				Address:           epStr,
				ConnectionTimeout: connectionTimeout,
				TLSEnabled:        tlsEnabled,
				TLSClientSideAuth: tlsClientSideAuth,
				TLSRootCertBytes:  tlsRootCerts,
				Usage:             "broadcast",
			})
		}
	}

	return oc.ConsensusType(), newOrderers, nil
}

// MSPManager returns the msp.MSPManager that reflects the current Channel
// configuration. Users should not memoize references to this object.
func (s *Service) MSPManager() driver.MSPManager {
	return &mspManager{s.resources().MSPManager()}
}

// CheckACL checks the ACL for the resource for the Channel using the
// SignedProposal from which an id can be extracted for testing against a policy
func (s *Service) CheckACL(signedProp driver.SignedProposal) error {
	return s.ACLProvider.CheckACL(resources.Peer_Propose, s.channelID, signedProp.Internal())
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

type policyManagerGetterFunc struct {
	channelID       string
	resourcesGetter func() channelconfig.Resources
}

func (p *policyManagerGetterFunc) Manager(channelID string) policies.Manager {
	if p.channelID == channelID {
		return p.resourcesGetter().PolicyManager()
	}
	return nil
}
