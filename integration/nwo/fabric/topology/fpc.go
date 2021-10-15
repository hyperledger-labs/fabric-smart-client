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
func (c *Topology) EnableFPC() {
	c.FPC = true
	c.AddFPC("ercc", "fpc/ercc")
}

// AddFPC adds the Fabric Private Chaincode with the passed name and image.
// If no orgs are specified, then the Fabric Private Chaincode is installed on all organizations
// registered so far.
// The endorsement policy is set to the majority of the organization on which the chaincode
// has been installed.
func (c *Topology) AddFPC(name, image string, orgs ...string) *ChannelChaincode {
	if !c.FPC {
		c.EnableFPC()
	}

	if len(orgs) == 0 {
		orgs = c.Consortiums[0].Organizations
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
		for _, peer := range c.Peers {
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
			Image: image,
		},
		Channel: c.Channels[0].Name,
		Private: true,
		Peers:   peers,
	}
	c.Chaincodes = append(c.Chaincodes, cc)

	return cc
}
