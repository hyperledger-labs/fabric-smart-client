/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

import (
	"fmt"
	"strconv"
)

// EnableFPC enables FPC by adding the Enclave Registry Chaincode definition.
// The ERCC is installed on all organizations and the endorsement policy is
// set to the majority of the organization on which the chaincode
// has been installed.
func (t *Topology) EnableFPC() {
	if t.FPC {
		return
	}
	t.FPC = true
	t.AddFPC("ercc", "fpc/ercc")
}

// AddFPCAtOrgs adds the Fabric Private Chaincode with the passed name, image, and organizations
// If no orgs are specified, then the Fabric Private Chaincode is installed on all organizations
// registered so far.
// The endorsement policy is set to the majority of the organization on which the chaincode
// has been installed.
func (t *Topology) AddFPCAtOrgs(name, image string, orgs []string, options ...func(*ChannelChaincode)) *ChannelChaincode {
	t.EnableFPC()

	if len(orgs) == 0 {
		orgs = t.Consortiums[0].Organizations
	}
	majority := len(orgs)/2 + 1
	policy := "OutOf(" + strconv.Itoa(majority) + ","
	for i, org := range orgs {
		if i > 0 {
			policy += ","
		}
		policy += "'" + org + "MSP.member'"
	}
	policy += ")"

	var peers []string
	for _, org := range orgs {
		for _, peer := range t.Peers {
			if peer.Organization == org {
				peers = append(peers, peer.Name)
				break
			}
		}
	}

	cc := &ChannelChaincode{
		Chaincode: Chaincode{
			Name:            name,
			Version:         "Version-1.0",
			Sequence:        "1",
			InitRequired:    false,
			Path:            name,
			Lang:            "external",
			Label:           fmt.Sprintf("%s_1.0", name),
			Ctor:            `{"Args":["init"]}`,
			Policy:          policy,
			SignaturePolicy: policy,
		},
		PrivateChaincode: PrivateChaincode{
			Image:   image,
			SGXMode: "sim",
		},
		Channel: t.Channels[0].Name,
		Private: true,
		Peers:   peers,
	}

	// apply options
	for _, o := range options {
		o(cc)
	}

	t.AddChaincode(cc)

	return cc
}

// AddFPC adds the Fabric Private Chaincode with the passed name and image.
// The Fabric Private Chaincode is installed on all organizations registered so far.
// The endorsement policy is set to the majority of the organization on which the chaincode
// has been installed.
func (t *Topology) AddFPC(name, image string, options ...func(*ChannelChaincode)) *ChannelChaincode {
	return t.AddFPCAtOrgs(name, image, nil, options...)
}

func WithSGXMode(mode string) func(*ChannelChaincode) {
	return func(cc *ChannelChaincode) {
		cc.PrivateChaincode.SGXMode = mode
	}
}

func WithSGXDevicesPaths(paths []string) func(*ChannelChaincode) {
	return func(cc *ChannelChaincode) {
		cc.PrivateChaincode.SGXDevicesPaths = paths
	}
}

func WithMREnclave(mrenclave string) func(*ChannelChaincode) {
	return func(cc *ChannelChaincode) {
		cc.Chaincode.Version = mrenclave
	}
}
