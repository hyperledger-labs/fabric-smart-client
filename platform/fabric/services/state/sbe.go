/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"sort"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"
)

type sbeMetaHandler struct {
	forceSBE bool
}

func (s2 *sbeMetaHandler) StoreMeta(ns *Namespace, s interface{}, namespace string, key string, options *addOutputOptions) error {
	if !s2.forceSBE && !options.sbe {
		return nil
	}

	os, ok := s.(Ownable)
	if !ok {
		// Nothing to set
		return nil
	}

	// parse policy
	ep, err := newStateEP(nil)
	if err != nil {
		return errors.Wrap(err, "failed creating new EP state")
	}
	for _, owner := range os.Owners() {
		ep.addOwner(owner)
	}
	policyBytes, err := ep.Policy()
	if err != nil {
		return errors.Wrap(err, "failed creating policy")
	}

	// update meta
	rws, err := ns.RWSet()
	if err != nil {
		return errors.Wrap(err, "filed getting rw set")
	}

	meta, err := rws.GetStateMetadata(namespace, key, driver.FromIntermediate)
	if err != nil {
		return errors.Wrap(err, "filed getting metadata")
	}
	if len(meta) == 0 {
		meta = map[string][]byte{}
	}
	meta[peer.MetaDataKeys_VALIDATION_PARAMETER.String()] = policyBytes
	err = rws.SetStateMetadata(namespace, key, meta)
	if err != nil {
		return errors.Wrap(err, "failed setting metadata")
	}

	return nil
}

// stateEP implements the KeyEndorsementPolicy
type stateEP struct {
	orgs       map[string]msp.MSPRole_MSPRoleType
	identities []view.Identity
}

// newStateEP constructs a state-based endorsement policy from a given
// serialized EP byte array. If the byte array is empty, a new EP is created.
func newStateEP(policy []byte) (*stateEP, error) {
	s := &stateEP{
		orgs:       make(map[string]msp.MSPRole_MSPRoleType),
		identities: []view.Identity{},
	}
	if policy != nil {
		spe := &common.SignaturePolicyEnvelope{}
		if err := proto.Unmarshal(policy, spe); err != nil {
			return nil, errors.Errorf("error unmarshaling to SignaturePolicy: %s", err)
		}

		err := s.setMSPIDsFromSP(spe)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Policy returns the endorsement policy as bytes
func (s *stateEP) Policy() ([]byte, error) {
	spe, err := s.policyFromMSPIDs()
	if err != nil {
		return nil, errors.Wrapf(err, "failed building policy")
	}
	logger.Debugf("StateEP Polci [\n%s\n]", spe.String())
	spBytes, err := proto.Marshal(spe)
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshalling policy")
	}
	return spBytes, nil
}

func (s *stateEP) addOwner(owner view.Identity) {
	logger.Debugf("SBE Adding Owner [%s]", string(owner))
	s.identities = append(s.identities, owner)
}

func (s *stateEP) setMSPIDsFromSP(sp *common.SignaturePolicyEnvelope) error {
	// iterate over the identities in this envelope
	for _, identity := range sp.Identities {
		// this imlementation only supports the ROLE type
		if identity.PrincipalClassification == msp.MSPPrincipal_ROLE {
			msprole := &msp.MSPRole{}
			err := proto.Unmarshal(identity.Principal, msprole)
			if err != nil {
				return errors.Errorf("error unmarshaling msp principal: %s", err)
			}
			s.orgs[msprole.GetMspIdentifier()] = msprole.GetRole()
		}
	}
	return nil
}

// listOrgs returns an array of channel orgs that are required to endorse chnages
func (s *stateEP) listOrgs() []string {
	orgNames := make([]string, 0, len(s.orgs))
	for mspid := range s.orgs {
		orgNames = append(orgNames, mspid)
	}
	return orgNames
}

func (s *stateEP) policyFromMSPIDs() (*common.SignaturePolicyEnvelope, error) {
	mspids := s.listOrgs()
	identities := s.identities
	sort.Strings(mspids)

	principals := make([]*msp.MSPPrincipal, len(mspids)+len(identities))
	sigspolicy := make([]*common.SignaturePolicy, len(mspids)+len(identities))
	i := 0
	for _, id := range mspids {
		principal, err := proto.Marshal(
			&msp.MSPRole{
				Role:          s.orgs[id],
				MspIdentifier: id,
			},
		)
		if err != nil {
			return nil, err
		}
		principals[i] = &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               principal,
		}
		sigspolicy[i] = &common.SignaturePolicy{
			Type: &common.SignaturePolicy_SignedBy{
				SignedBy: int32(i),
			},
		}
		i++
	}

	for _, id := range identities {
		principals[i] = &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_IDENTITY,
			Principal:               id,
		}
		sigspolicy[i] = &common.SignaturePolicy{
			Type: &common.SignaturePolicy_SignedBy{
				SignedBy: int32(i),
			},
		}
		i++
	}

	// create the policy: it requires exactly 1 signature from all of the principals
	p := &common.SignaturePolicyEnvelope{
		Version: 0,
		Rule: &common.SignaturePolicy{
			Type: &common.SignaturePolicy_NOutOf_{
				NOutOf: &common.SignaturePolicy_NOutOf{
					N:     int32(len(sigspolicy)),
					Rules: sigspolicy,
				},
			},
		},
		Identities: principals,
	}
	return p, nil
}
