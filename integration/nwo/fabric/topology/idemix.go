/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

import "strings"

func (c *Topology) EnableIdemix() *fscOrg {
	name := "IdemixOrg"
	o := &Organization{
		ID:            name,
		Name:          name,
		MSPID:         name + "MSP",
		MSPType:       "idemix",
		Domain:        strings.ToLower(name) + ".example.com",
		EnableNodeOUs: c.NodeOUs,
		Users:         0,
		CA:            &CA{Hostname: "ca"},
	}
	c.AppendOrganization(o)
	return &fscOrg{c: c, o: o}
}
