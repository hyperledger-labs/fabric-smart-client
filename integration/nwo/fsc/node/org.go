/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package node

import (
	"fmt"
)

// Organization models information about an Organization. It includes
// the information needed to populate an MSP with cryptogen.
type Organization struct {
	ID            string   `yaml:"id,omitempty"`
	MSPID         string   `yaml:"msp_id,omitempty"`
	MSPType       string   `yaml:"msp_type,omitempty"`
	Name          string   `yaml:"name,omitempty"`
	Domain        string   `yaml:"domain,omitempty"`
	EnableNodeOUs bool     `yaml:"enable_node_organizational_units"`
	Users         int      `yaml:"users,omitempty"`
	CA            *CA      `yaml:"ca,omitempty"`
	UserNames     []string `yaml:"userNames,omitempty"`
}

type CA struct {
	Hostname string `yaml:"hostname,omitempty"`
}

type PeerIdentity struct {
	ID           string
	EnrollmentID string
	MSPType      string
	MSPID        string
	Org          string
}

// Peer defines a peer instance, it's owning organization, and the list of
// channels that the peer should be joined to.
type Peer struct {
	*Node
	Name            string          `yaml:"name,omitempty"`
	Organization    string          `yaml:"organization,omitempty"`
	Bootstrap       bool            `yaml:"bootstrap,omitempty"`
	ExecutablePath  string          `yaml:"executablepath,omitempty"`
	ExtraIdentities []*PeerIdentity `yaml:"extraidentities,omitempty"`
}

// ID provides a unique identifier for a peer instance.
func (p *Peer) ID() string {
	return fmt.Sprintf("%s.%s", p.Organization, p.Name)
}
